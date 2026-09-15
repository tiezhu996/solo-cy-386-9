package service

import (
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/model"
	"github.com/marketpal/marketpal/internal/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 本文件提供售后模块“可重复运行 + 真实持久化 + 独立连接”的回归夹具：
//   - 默认使用磁盘 SQLite 文件（WAL + busy_timeout），是落盘的真实数据库，不是 :memory: 替身；
//   - 每个用例在独立临时目录建库、独立准备数据，-count=N 重复执行不串扰；
//   - 并发用例通过 newHandle() 打开指向同一库的多个独立连接池（真实独立连接，非单连接串行化）；
//   - 设置 MARKETPAL_TEST_POSTGRES_DSN 后自动改走真实 PostgreSQL，验证 SELECT ... FOR UPDATE 行锁。

// refundTestModelsAll 售后回归需要迁移的全部模型（与生产 AutoMigrate 一致）。
var refundTestModelsAll = []interface{}{
	&model.User{}, &model.Product{}, &model.Favorite{}, &model.Address{},
	&model.CartItem{}, &model.Order{}, &model.Refund{}, &model.RefundNegotiation{},
	&model.Message{}, &model.Review{}, &model.AuditLog{},
}

// envSeq 进程内自增序号，保证临时库文件名与业务唯一键（订单号/售后单号）全局唯一。
var envSeq atomic.Uint64

// persistEnv 一套独立持久化环境。
type persistEnv struct {
	t      *testing.T
	driver string // "sqlite" 或 "postgres"
	dsn    string
	db     *gorm.DB // 主连接（建表、准备数据、最终回读）
}

// openPersistEnv 打开一套全新环境：有 PostgreSQL DSN 走 Postgres，否则用独立磁盘 SQLite 文件。
func openPersistEnv(t *testing.T) *persistEnv {
	t.Helper()
	if dsn := os.Getenv("MARKETPAL_TEST_POSTGRES_DSN"); dsn != "" {
		return openPostgresEnv(t, dsn)
	}
	return openSQLiteEnv(t)
}

func openSQLiteEnv(t *testing.T) *persistEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("refund_%d.db", envSeq.Add(1)))
	// WAL + busy_timeout 支持多连接并发；_txlock=immediate 让写事务 BEGIN IMMEDIATE，
	// 在事务开始即取保留锁：竞争事务在锁上排队进入临界区，避免 SQLite 读后写升级锁直接报 SQLITE_BUSY。
	// 竞争仍然真实（多连接同时发起），只是由数据库锁裁决先后，与 Postgres 的 FOR UPDATE 效果等价。
	dsn := filepath.ToSlash(path) +
		"?_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开磁盘 SQLite 失败: %v", err)
	}
	if err := db.AutoMigrate(refundTestModelsAll...); err != nil {
		t.Fatalf("SQLite AutoMigrate 失败: %v", err)
	}
	tunePool(db)
	t.Cleanup(func() { _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`).Error })
	return &persistEnv{t: t, driver: "sqlite", dsn: dsn, db: db}
}

func openPostgresEnv(t *testing.T, dsn string) *persistEnv {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接 PostgreSQL 失败（DSN=%s）: %v", dsn, err)
	}
	if err := db.AutoMigrate(refundTestModelsAll...); err != nil {
		t.Fatalf("PostgreSQL AutoMigrate 失败: %v", err)
	}
	tunePool(db)
	// 每个用例清空相关表，保证重复运行互不串数据。
	if err := db.Exec(`TRUNCATE refund_negotiations, refunds, reviews, messages, cart_items, addresses, favorites, products, orders, users, audit_logs RESTART IDENTITY CASCADE`).Error; err != nil {
		t.Fatalf("PostgreSQL TRUNCATE 失败: %v", err)
	}
	return &persistEnv{t: t, driver: "postgres", dsn: dsn, db: db}
}

// newHandle 打开指向同一数据库的全新独立连接池（并发用例每个 goroutine 一个，真实竞争）。
func (e *persistEnv) newHandle() *gorm.DB {
	e.t.Helper()
	var db *gorm.DB
	var err error
	if e.driver == "postgres" {
		db, err = gorm.Open(postgres.Open(e.dsn), &gorm.Config{})
	} else {
		db, err = gorm.Open(sqlite.Open(e.dsn), &gorm.Config{})
	}
	if err != nil {
		e.t.Fatalf("打开独立连接失败(%s): %v", e.driver, err)
	}
	tunePool(db)
	return db
}

func tunePool(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err != nil {
		return
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxLifetime(time.Hour)
}

// svc 在指定连接上构造售后服务（仓储各自独立，事务跑在各自连接上）。
func (e *persistEnv) svc(db *gorm.DB) *RefundService {
	return NewRefundService(
		db,
		repository.NewRefundRepository(db),
		repository.NewOrderRepository(db),
		repository.NewProductRepository(db),
		slog.Default(),
	)
}

// seed 准备全新的已售商品 + 已付款订单（每次独立，序号保证业务唯一键不冲突）。
func (e *persistEnv) seed(tag string, total float64, orderStatus string) refundFixture {
	e.t.Helper()
	e.ensureParties()
	n := envSeq.Add(1)
	paidAt := time.Now().Add(-time.Hour)
	p := &model.Product{
		SellerID: 200, Title: "回归商品-" + tag + "-" + fmt.Sprint(n), Description: "九成新",
		OriginalPrice: total * 2, Price: total, Condition: constants.ProductConditionAlmostNew,
		Category: constants.ProductCategoryDigital, Status: constants.ProductStatusSold,
	}
	if err := e.db.Create(p).Error; err != nil {
		e.t.Fatalf("夹具商品创建失败: %v", err)
	}
	o := &model.Order{
		OrderNo: fmt.Sprintf("RGN-%s-%d", tag, n), BuyerID: 100, SellerID: 200,
		ProductID: p.ID, AddressID: 1, Quantity: 1, TotalPrice: total,
		Status: orderStatus, PaidAt: &paidAt,
	}
	if err := e.db.Create(o).Error; err != nil {
		e.t.Fatalf("夹具订单创建失败: %v", err)
	}
	return refundFixture{Order: o, Product: p}
}

type refundFixture struct {
	Order   *model.Order
	Product *model.Product
}

// ensureParties 准备买家(100)/卖家(200)与默认收货地址(1)，满足外键约束。
// 每个环境独立（SQLite 临时文件全新 / Postgres 用例前 TRUNCATE），因此直接创建。
func (e *persistEnv) ensureParties() {
	e.t.Helper()
	var userCount int64
	e.db.Model(&model.User{}).Where("id IN ?", []uint{100, 200}).Count(&userCount)
	if userCount == 0 {
		users := []model.User{
			{ID: 100, Username: "buyer_rt", PasswordHash: "x", Nickname: "买家", Role: "user"},
			{ID: 200, Username: "seller_rt", PasswordHash: "x", Nickname: "卖家", Role: "user"},
		}
		for i := range users {
			if err := e.db.Create(&users[i]).Error; err != nil {
				e.t.Fatalf("创建买卖双方失败: %v", err)
			}
		}
		addr := model.Address{ID: 1, UserID: 100, ReceiverName: "买家", Phone: "13800000000", Province: "北京", City: "北京", Detail: "测试地址"}
		if err := e.db.Create(&addr).Error; err != nil {
			e.t.Fatalf("创建收货地址失败: %v", err)
		}
	}
}

// snapshot 一次从指定连接回读订单、售后单、协商历史、商品四层的真实落库状态。
type snapshot struct {
	order        model.Order
	refund       *model.Refund
	negotiations []model.RefundNegotiation
	product      model.Product
}

func (e *persistEnv) readSnapshot(db *gorm.DB, orderID uint) snapshot {
	e.t.Helper()
	s := snapshot{}
	if err := db.First(&s.order, orderID).Error; err != nil {
		e.t.Fatalf("回读订单失败: %v", err)
	}
	var rf model.Refund
	if err := db.Where("order_id = ?", orderID).First(&rf).Error; err != nil {
		e.t.Fatalf("回读售后单失败: %v", err)
	}
	s.refund = &rf
	if err := db.Where("refund_id = ?", rf.ID).Order("id ASC").Find(&s.negotiations).Error; err != nil {
		e.t.Fatalf("回读协商历史失败: %v", err)
	}
	if err := db.First(&s.product, s.order.ProductID).Error; err != nil {
		e.t.Fatalf("回读商品失败: %v", err)
	}
	return s
}

// expectState 终态一致性期望：跨订单/售后/协商历史/商品四层。
type expectState struct {
	refundStatus    string   // 售后单终态
	finalAmount     *float64 // 期望退款结果；nil 表示 final_amount 必须为空
	orderStatus     string   // 订单状态
	activeRefundNil bool     // 订单 active_refund_id 是否必须为空
	productStatus   string   // 商品状态
	actions         []string // 协商历史动作的严格有序期望
}

// verifyConsistency 逐层核对，失败时明确指出是订单/售后/协商历史/商品哪一层不一致。
func (e *persistEnv) verifyConsistency(s snapshot, want expectState) {
	e.t.Helper()
	var problems []string

	// 售后单层。
	if s.refund == nil {
		e.t.Fatalf("售后单缺失：订单 id=%d 查不到售后记录", s.order.ID)
	}
	if s.refund.Status != want.refundStatus {
		problems = append(problems, fmt.Sprintf("售后单状态不一致：期望 %s，实际 %s（售后单 id=%d）", want.refundStatus, s.refund.Status, s.refund.ID))
	}
	switch {
	case want.finalAmount == nil && s.refund.FinalAmount != nil:
		problems = append(problems, fmt.Sprintf("售后单退款结果不一致：期望 final_amount 为空，实际 %.2f", *s.refund.FinalAmount))
	case want.finalAmount != nil && s.refund.FinalAmount == nil:
		problems = append(problems, fmt.Sprintf("售后单退款结果不一致：期望 %.2f，实际为空", *want.finalAmount))
	case want.finalAmount != nil && math.Abs(*s.refund.FinalAmount-*want.finalAmount) > 1e-9:
		problems = append(problems, fmt.Sprintf("售后单退款结果不一致：期望 %.2f，实际 %.2f", *want.finalAmount, *s.refund.FinalAmount))
	}

	// 订单层。
	if s.order.Status != want.orderStatus {
		problems = append(problems, fmt.Sprintf("订单状态不一致：期望 %s，实际 %s（订单 %s）", want.orderStatus, s.order.Status, s.order.OrderNo))
	}
	if want.activeRefundNil && s.order.ActiveRefundID != nil {
		problems = append(problems, fmt.Sprintf("订单售后标记不一致：期望 active_refund_id 为空，实际指向 %d（订单 %s）", *s.order.ActiveRefundID, s.order.OrderNo))
	}
	if !want.activeRefundNil && (s.order.ActiveRefundID == nil || *s.order.ActiveRefundID != s.refund.ID) {
		problems = append(problems, fmt.Sprintf("订单售后标记不一致：期望指向售后单 %d，实际 %v（订单 %s）", s.refund.ID, s.order.ActiveRefundID, s.order.OrderNo))
	}

	// 商品层。
	if s.product.Status != want.productStatus {
		problems = append(problems, fmt.Sprintf("商品状态不一致：期望 %s，实际 %s（商品 id=%d）", want.productStatus, s.product.Status, s.product.ID))
	}

	// 协商历史层：动作必须严格有序、数量一致（只追加，竞争败方动作不得落库）。
	gotActions := make([]string, 0, len(s.negotiations))
	for _, n := range s.negotiations {
		gotActions = append(gotActions, n.Action)
	}
	if len(gotActions) != len(want.actions) {
		problems = append(problems, fmt.Sprintf("协商历史条数不一致：期望 %d 条 %v，实际 %d 条 %v（售后单 id=%d）", len(want.actions), want.actions, len(gotActions), gotActions, s.refund.ID))
	} else {
		for i := range want.actions {
			if gotActions[i] != want.actions[i] {
				problems = append(problems, fmt.Sprintf("协商历史顺序不一致：期望 %v，实际 %v（售后单 id=%d）", want.actions, gotActions, s.refund.ID))
				break
			}
		}
	}

	if len(problems) > 0 {
		e.t.Fatalf("售后终态跨层一致性校验失败（驱动=%s）：\n  - %s", e.driver, strings.Join(problems, "\n  - "))
	}
}

func ptrFloat(v float64) *float64 { return &v }
