package service

import (
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/dto"
	"github.com/marketpal/marketpal/internal/model"
	"github.com/marketpal/marketpal/internal/repository"
	"github.com/marketpal/marketpal/internal/util"
	"gorm.io/gorm"
)

// seq 保证每次打开的内存库 DSN 唯一（重复运行测试也互不干扰）。
var seq atomic.Uint64

func TestCanRefundTransition(t *testing.T) {
	cases := []struct {
		from string
		to   string
		want bool
	}{
		{constants.RefundStatusPendingSeller, constants.RefundStatusProposalPending, true}, // 卖家提方案
		{constants.RefundStatusPendingSeller, constants.RefundStatusAgreed, true},          // 卖家直接同意
		{constants.RefundStatusPendingSeller, constants.RefundStatusRejected, true},        // 卖家拒绝
		{constants.RefundStatusProposalPending, constants.RefundStatusAgreed, true},        // 买家接受方案
		{constants.RefundStatusProposalPending, constants.RefundStatusCancelled, true},     // 买家撤销（方案阶段）
		{constants.RefundStatusPendingSeller, constants.RefundStatusCancelled, true},       // 卖家处理前撤销
		{constants.RefundStatusProposalPending, constants.RefundStatusRejected, false},     // 方案阶段卖家不能再拒绝
		{constants.RefundStatusAgreed, constants.RefundStatusCancelled, false},             // 终态不可撤销
		{constants.RefundStatusRejected, constants.RefundStatusAgreed, false},              // 终态不可翻转
	}
	for _, tc := range cases {
		t.Run(tc.from+"->"+tc.to, func(t *testing.T) {
			if got := constants.CanRefundTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("CanRefundTransition(%s,%s)=%v want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

// newRefundTestDB 准备独立的磁盘库与售后夹具：买家 100、卖家 200、商品、已付款订单。
// 每次调用在独立临时目录落盘（WAL + BEGIN IMMEDIATE），重复执行（-count=N）与用例之间不共享数据。
func newRefundTestDB(t *testing.T) (*gorm.DB, *RefundService, *model.Order) {
	t.Helper()
	path := filepath.Join(t.TempDir(), fmt.Sprintf("legacy_refund_%d.db", seq.Add(1)))
	dsn := filepath.ToSlash(path) +
		"?_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, derr := db.DB(); derr == nil {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(4)
	}
	models := []interface{}{
		&model.User{}, &model.Product{}, &model.Address{}, &model.Order{},
		&model.Refund{}, &model.RefundNegotiation{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now()
	paidAt := now.Add(-time.Hour)
	product := &model.Product{SellerID: 200, Title: "二手耳机", Description: "九成新", OriginalPrice: 200, Price: 100, Condition: "almost_new", Category: "digital", Status: "sold"}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	o := &model.Order{
		OrderNo: "T" + t.Name(), BuyerID: 100, SellerID: 200, ProductID: product.ID, AddressID: 1,
		Quantity: 1, TotalPrice: 100, Status: constants.OrderStatusPendingShipment, PaidAt: &paidAt,
	}
	if err := db.Create(o).Error; err != nil {
		t.Fatalf("create order: %v", err)
	}
	svc := NewRefundService(
		db,
		repository.NewRefundRepository(db),
		repository.NewOrderRepository(db),
		repository.NewProductRepository(db),
		slog.Default(),
	)
	return db, svc, o
}

func applyReq(orderID uint, amount float64) dto.RefundApplyRequest {
	return dto.RefundApplyRequest{
		OrderID: orderID, Type: constants.RefundTypePartial,
		Reason: "商品与描述不符，申请部分退款", Amount: amount, Evidence: []string{"http://x/a.jpg"},
	}
}

func asAppError(t *testing.T, err error) *util.AppError {
	t.Helper()
	var ae *util.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected AppError, got %v", err)
	}
	return ae
}

func TestRefundApplyThenAgree(t *testing.T) {
	_, svc, o := newRefundTestDB(t)

	rf, err := svc.Apply(100, applyReq(o.ID, 30))
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if rf.Status != constants.RefundStatusPendingSeller {
		t.Fatalf("status=%s", rf.Status)
	}
	// 订单进入售后中。
	var marked model.Order
	_ = svc.db.First(&marked, o.ID)
	if marked.ActiveRefundID == nil || *marked.ActiveRefundID != rf.ID {
		t.Fatalf("order should be marked in refund")
	}

	// 非卖家不能处理。
	if _, err := svc.Agree(100, rf.ID); err == nil {
		t.Fatal("buyer cannot agree")
	}
	agreed, err := svc.Agree(200, rf.ID)
	if err != nil {
		t.Fatalf("Agree: %v", err)
	}
	if agreed.Status != constants.RefundStatusAgreed || agreed.FinalAmount == nil || *agreed.FinalAmount != 30 {
		t.Fatalf("agreed refund wrong: %+v", agreed)
	}
	// 部分退款成功：订单恢复可履约（解除售后中），状态不变。
	var after model.Order
	_ = svc.db.First(&after, o.ID)
	if after.ActiveRefundID != nil {
		t.Fatal("active refund should be cleared")
	}
	if after.Status != constants.OrderStatusPendingShipment {
		t.Fatalf("partial refund should keep order status, got %s", after.Status)
	}
	// 协商历史完整：申请 → 同意，两条且不改写。
	if len(agreed.Negotiations) != 2 ||
		agreed.Negotiations[0].Action != constants.RefundActionApply ||
		agreed.Negotiations[1].Action != constants.RefundActionAgree {
		t.Fatalf("negotiations mismatch: %+v", agreed.Negotiations)
	}
}

func TestRefundReturnAgreeCancelsOrder(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	req := applyReq(o.ID, 100)
	req.Type = constants.RefundTypeReturn
	rf, err := svc.Apply(100, req)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := svc.Agree(200, rf.ID); err != nil {
		t.Fatalf("Agree: %v", err)
	}
	var after model.Order
	_ = svc.db.First(&after, o.ID)
	if after.Status != constants.OrderStatusCancelled || after.ActiveRefundID != nil {
		t.Fatalf("return refund should cancel order & clear flag, got status=%s flag=%v", after.Status, after.ActiveRefundID)
	}
}

func TestReturnRefundMustBeFullAmount(t *testing.T) {
	_, svc, o := newRefundTestDB(t)

	// 退货退款按低于实付的金额申请必须被拒绝（不能 1 元退款就让订单取消/重新上架）。
	low := applyReq(o.ID, 1)
	low.Type = constants.RefundTypeReturn
	_, err := svc.Apply(100, low)
	if asAppError(t, err).Code != constants.CodeRefundAmountExceed {
		t.Fatalf("return refund below paid amount should fail, got %v", err)
	}

	// 退货退款按全额申请成功后，卖家不能再提部分金额方案，只能同意/拒绝。
	full := applyReq(o.ID, 100)
	full.Type = constants.RefundTypeReturn
	rf, err := svc.Apply(100, full)
	if err != nil {
		t.Fatalf("full amount return refund apply: %v", err)
	}
	if _, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 50, Reason: "只退一半"}); err == nil {
		t.Fatal("propose partial amount on return refund should fail")
	}

	// 同意路径同样校验全额：篡改申请金额（模拟脏数据）时 agree 不得放行。
	if _, err := svc.Agree(200, rf.ID); err != nil {
		t.Fatalf("agree full return refund: %v", err)
	}
}

func TestPartialRefundAllowsAmountBelowPaid(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	// 部分退款允许低于实付、不超过实付，保持原行为。
	rf, err := svc.Apply(100, applyReq(o.ID, 30))
	if err != nil {
		t.Fatalf("partial refund apply: %v", err)
	}
	if _, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 20, Reason: "只同意 20"}); err != nil {
		t.Fatalf("propose on partial refund should succeed, got %v", err)
	}
}

func TestOrderReadsLatestRefundAfterClosed(t *testing.T) {
	db, svc, o := newRefundTestDB(t)
	orderRepo := repository.NewOrderRepository(db)
	rf, _ := svc.Apply(100, applyReq(o.ID, 30))
	// 拒绝后 active_refund_id 清空，但订单详情仍能读到最近一笔完结售后。
	if _, err := svc.Reject(200, rf.ID, dto.RefundRejectRequest{Reason: "凭证不足"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	got, err := orderRepo.GetByID(o.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ActiveRefundID != nil || got.ActiveRefund != nil {
		t.Fatal("active refund must be cleared after reject")
	}
	if got.LastRefund == nil || got.LastRefund.Status != constants.RefundStatusRejected {
		t.Fatalf("last refund must be readable after closed, got %+v", got.LastRefund)
	}
}

func TestRefundRejectResumesOrderAndBlocksReapply(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	rf, _ := svc.Apply(100, applyReq(o.ID, 30))
	if _, err := svc.Reject(200, rf.ID, dto.RefundRejectRequest{Reason: "凭证不足，无法处理"}); err != nil {
		t.Fatalf("Reject: %v", err)
	}
	var after model.Order
	_ = svc.db.First(&after, o.ID)
	if after.Status != constants.OrderStatusPendingShipment || after.ActiveRefundID != nil {
		t.Fatalf("reject should resume order, got status=%s flag=%v", after.Status, after.ActiveRefundID)
	}
	// 拒绝后不能重复申请。
	_, err := svc.Apply(100, applyReq(o.ID, 30))
	if asAppError(t, err).Code != constants.CodeRefundExists {
		t.Fatalf("reapply after reject should be CodeRefundExists, got %v", err)
	}
}

func TestRefundBuyerCancelResumesOrder(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	rf, _ := svc.Apply(100, applyReq(o.ID, 30))
	// 卖家处理前买家可撤销。
	if _, err := svc.Cancel(100, rf.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	var after model.Order
	_ = svc.db.First(&after, o.ID)
	if after.ActiveRefundID != nil || after.Status != constants.OrderStatusPendingShipment {
		t.Fatalf("cancel should resume order, got status=%s flag=%v", after.Status, after.ActiveRefundID)
	}
}

func TestRefundProposalAcceptFlow(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	rf, _ := svc.Apply(100, applyReq(o.ID, 80))

	// 方案金额超过实付：拒绝。
	_, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 120, Reason: "最多退这些"})
	if asAppError(t, err).Code != constants.CodeRefundAmountExceed {
		t.Fatalf("expect amount exceed, got %v", err)
	}
	if _, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 50, Reason: "只同意退 50"}); err != nil {
		t.Fatalf("Propose: %v", err)
	}
	// 方案待确认阶段，卖家不能重复提方案/同意/拒绝。
	if _, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 40, Reason: "再改"}); err == nil {
		t.Fatal("propose twice should fail")
	}
	// 买家接受方案。
	done, err := svc.Accept(100, rf.ID)
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if done.Status != constants.RefundStatusAgreed || done.FinalAmount == nil || *done.FinalAmount != 50 {
		t.Fatalf("accept wrong: %+v", done)
	}
	// 终态后重复提交任何动作都不能改写结果。
	if _, err := svc.Accept(100, rf.ID); err == nil {
		t.Fatal("duplicate accept must not rewrite record")
	}
	if _, err := svc.Cancel(100, rf.ID); err == nil {
		t.Fatal("cancel after agreed must fail")
	}
}

func TestRefundDuplicateApplyIsIdempotent(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	first, err := svc.Apply(100, applyReq(o.ID, 30))
	if err != nil {
		t.Fatalf("first apply: %v", err)
	}
	second, err := svc.Apply(100, applyReq(o.ID, 30))
	if err != nil {
		t.Fatalf("identical duplicate apply should be idempotent, got %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate apply must return same refund, got %d vs %d", first.ID, second.ID)
	}
	// 处理中用不同内容重复申请 → 冲突，不改写。
	_, err = svc.Apply(100, applyReq(o.ID, 40))
	if asAppError(t, err).Code != constants.CodeRefundExists {
		t.Fatalf("different duplicate apply should conflict, got %v", err)
	}
	var count int64
	svc.db.Model(&model.Refund{}).Where("order_id = ?", o.ID).Count(&count)
	if count != 1 {
		t.Fatalf("only one refund allowed, got %d", count)
	}
}

func TestRefundAmountExceedAndGuards(t *testing.T) {
	_, svc, o := newRefundTestDB(t)
	// 金额超过实付。
	if _, err := svc.Apply(100, applyReq(o.ID, 100.01)); asAppError(t, err).Code != constants.CodeRefundAmountExceed {
		t.Fatalf("expect amount exceed, got %v", err)
	}
	// 非买家不能申请。
	if _, err := svc.Apply(200, applyReq(o.ID, 10)); err == nil {
		t.Fatal("seller cannot apply refund")
	}
	// 未付款订单不能申请。
	unpaid := &model.Order{OrderNo: "T-unpaid", BuyerID: 100, SellerID: 200, ProductID: 1, AddressID: 1, TotalPrice: 100, Status: constants.OrderStatusPendingPayment}
	if err := svc.db.Create(unpaid).Error; err != nil {
		t.Fatalf("create unpaid order: %v", err)
	}
	if _, err := svc.Apply(100, applyReq(unpaid.ID, 10)); asAppError(t, err).Code != constants.CodeOrderStateInvalid {
		t.Fatalf("unpaid order cannot refund, got %v", err)
	}
}

func TestOrderActionsBlockedWhileRefund(t *testing.T) {
	db, refundSvc, o := newRefundTestDB(t)
	// 用订单 service 验证售后中拦截发货。
	orderSvc := NewOrderService(
		db,
		repository.NewOrderRepository(db),
		repository.NewProductRepository(db),
		repository.NewAddressRepository(db),
		repository.NewCartRepository(db),
		slog.Default(),
	)
	if _, err := refundSvc.Apply(100, applyReq(o.ID, 30)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := orderSvc.Ship(200, o.ID); asAppError(t, err).Code != constants.CodeOrderInRefund {
		t.Fatalf("ship during refund should be blocked, got %v", err)
	}

	// 撤销后恢复可发货（订单回到待发货状态机）。
	var rf model.Refund
	if err := db.Where("order_id = ?", o.ID).First(&rf).Error; err != nil {
		t.Fatalf("load refund: %v", err)
	}
	if _, err := refundSvc.Cancel(100, rf.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := orderSvc.Ship(200, o.ID); err != nil {
		t.Fatalf("ship after refund cancel should succeed, got %v", err)
	}
}
