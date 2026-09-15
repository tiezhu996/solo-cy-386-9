package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/marketpal/marketpal/internal/model"
	"gorm.io/gorm"
)

// ErrRefundExists 售后单已存在（唯一索引兜底，正常由 service 行锁 + 预检查拦截）。
var ErrRefundExists = errors.New("refund already exists")

// RefundRepository 售后仓储接口（售后单 + 只追加的协商历史）。
type RefundRepository interface {
	CreateWithTx(tx *gorm.DB, refund *model.Refund) error
	AddNegotiationWithTx(tx *gorm.DB, n *model.RefundNegotiation) error
	GetByIDForUpdate(tx *gorm.DB, id uint) (*model.Refund, error)
	GetByOrderIDForUpdate(tx *gorm.DB, orderID uint) (*model.Refund, error)
	GetByID(id uint) (*model.Refund, error)
	GetByOrderID(orderID uint) (*model.Refund, error)
	ListByBuyer(buyerID uint, status string, page, pageSize int) ([]model.Refund, int64, error)
	ListBySeller(sellerID uint, status string, page, pageSize int) ([]model.Refund, int64, error)
	// TransitForUpdate 条件状态流转：仅当当前状态 ∈ fromStatuses 时执行 updates（须含 status）。
	// 返回 false 表示状态已被并发操作改写（并发处理只能有一个结果）。
	TransitForUpdate(tx *gorm.DB, id uint, fromStatuses []string, updates map[string]interface{}) (bool, error)
}

type refundRepo struct {
	db *gorm.DB
}

// NewRefundRepository 构造售后仓储。
func NewRefundRepository(db *gorm.DB) RefundRepository {
	return &refundRepo{db: db}
}

func (r *refundRepo) CreateWithTx(tx *gorm.DB, refund *model.Refund) error {
	if err := tx.Create(refund).Error; err != nil {
		if isDuplicateKeyErr(err) {
			return ErrRefundExists
		}
		return fmt.Errorf("create refund order=%d: %w", refund.OrderID, err)
	}
	return nil
}

func (r *refundRepo) AddNegotiationWithTx(tx *gorm.DB, n *model.RefundNegotiation) error {
	if err := tx.Create(n).Error; err != nil {
		return fmt.Errorf("add refund negotiation refund=%d: %w", n.RefundID, err)
	}
	return nil
}

func (r *refundRepo) GetByIDForUpdate(tx *gorm.DB, id uint) (*model.Refund, error) {
	var rf model.Refund
	err := tx.Clauses(clauseLocking()).First(&rf, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get refund by id %d for update: %w", id, err)
	}
	return &rf, nil
}

func (r *refundRepo) GetByOrderIDForUpdate(tx *gorm.DB, orderID uint) (*model.Refund, error) {
	var rf model.Refund
	err := tx.Clauses(clauseLocking()).Where("order_id = ?", orderID).First(&rf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get refund by order %d for update: %w", orderID, err)
	}
	return &rf, nil
}

func (r *refundRepo) GetByID(id uint) (*model.Refund, error) {
	var rf model.Refund
	err := r.db.
		Preload("Order").Preload("Order.Product").Preload("Order.Buyer").Preload("Order.Seller").Preload("Order.Address").
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		First(&rf, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get refund by id %d: %w", id, err)
	}
	return &rf, nil
}

func (r *refundRepo) GetByOrderID(orderID uint) (*model.Refund, error) {
	var rf model.Refund
	err := r.db.
		Preload("Order").Preload("Order.Product").
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("order_id = ?", orderID).First(&rf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get refund by order %d: %w", orderID, err)
	}
	return &rf, nil
}

func (r *refundRepo) ListByBuyer(buyerID uint, status string, page, pageSize int) ([]model.Refund, int64, error) {
	var list []model.Refund
	var total int64
	q := r.db.Model(&model.Refund{}).Where("buyer_id = ?", buyerID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count buyer refunds: %w", err)
	}
	if err := q.Preload("Order").Preload("Order.Product").
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list buyer refunds: %w", err)
	}
	return list, total, nil
}

func (r *refundRepo) ListBySeller(sellerID uint, status string, page, pageSize int) ([]model.Refund, int64, error) {
	var list []model.Refund
	var total int64
	q := r.db.Model(&model.Refund{}).Where("seller_id = ?", sellerID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count seller refunds: %w", err)
	}
	if err := q.Preload("Order").Preload("Order.Product").
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, 0, fmt.Errorf("list seller refunds: %w", err)
	}
	return list, total, nil
}

func (r *refundRepo) TransitForUpdate(tx *gorm.DB, id uint, fromStatuses []string, updates map[string]interface{}) (bool, error) {
	if len(fromStatuses) == 0 {
		return false, fmt.Errorf("transit refund %d: empty expected statuses", id)
	}
	res := tx.Model(&model.Refund{}).Where("id = ? AND status IN ?", id, fromStatuses).Updates(updates)
	if res.Error != nil {
		return false, fmt.Errorf("transit refund %d: %w", id, res.Error)
	}
	return res.RowsAffected == 1, nil
}

// isDuplicateKeyErr 兼容 PostgreSQL（unique constraint）与 SQLite（UNIQUE constraint）唯一索引报错。
func isDuplicateKeyErr(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "unique constraint") || strings.Contains(s, "UNIQUE constraint") || strings.Contains(s, "Duplicate entry")
}
