package repository

import (
	"errors"
	"fmt"

	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/model"
	"gorm.io/gorm"
)

// OrderRepository 订单仓储接口。
type OrderRepository interface {
	Create(order *model.Order) error
	CreateWithTx(tx *gorm.DB, order *model.Order) error
	GetByID(id uint) (*model.Order, error)
	GetByIDForUpdate(tx *gorm.DB, id uint) (*model.Order, error)
	ListByBuyer(buyerID uint, status string, page, pageSize int) ([]model.Order, int64, error)
	ListBySeller(sellerID uint, status string, page, pageSize int) ([]model.Order, int64, error)
	UpdateStatusForUpdate(tx *gorm.DB, id uint, status string, updates map[string]interface{}) error
	// SetActiveRefundForUpdate 标记/解除订单的进行中售后单（refundID 为 nil 时解除，拒绝/撤销/完成后调用）。
	SetActiveRefundForUpdate(tx *gorm.DB, orderID uint, refundID *uint) error
	Update(order *model.Order) error
}

type orderRepo struct {
	db *gorm.DB
}

// NewOrderRepository 构造订单仓储。
func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepo{db: db}
}

func (r *orderRepo) Create(order *model.Order) error {
	if err := r.db.Create(order).Error; err != nil {
		return fmt.Errorf("create order: %w", err)
	}
	return nil
}

// CreateWithTx 在指定事务中创建订单（订单创建必须与商品锁定同事务，避免跨连接死锁）。
func (r *orderRepo) CreateWithTx(tx *gorm.DB, order *model.Order) error {
	if err := tx.Create(order).Error; err != nil {
		return fmt.Errorf("create order with tx: %w", err)
	}
	return nil
}

func (r *orderRepo) GetByID(id uint) (*model.Order, error) {
	var o model.Order
	err := r.db.Preload("Product").Preload("Product.Seller").Preload("Buyer").Preload("Seller").Preload("Address").First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order by id %d: %w", id, err)
	}
	if err := r.loadRefund(&o); err != nil {
		return nil, err
	}
	return &o, nil
}

// loadRefund 加载订单关联的售后单：一个订单至多一条（order_id 唯一）。
// 无论进行中还是已完结都赋值到 LastRefund；进行中额外赋值到 ActiveRefund。
func (r *orderRepo) loadRefund(o *model.Order) error {
	var rf model.Refund
	err := r.db.
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("order_id = ?", o.ID).First(&rf).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil // 无售后单属正常情况
	}
	if err != nil {
		return fmt.Errorf("load refund of order %d: %w", o.ID, err)
	}
	o.LastRefund = &rf
	if o.ActiveRefundID != nil && rf.ID == *o.ActiveRefundID && !isFinalRefundStatus(rf.Status) {
		o.ActiveRefund = &rf
	}
	return nil
}

// loadRefundBatch 批量加载各订单关联的售后单（含完结），避免 N+1。
func (r *orderRepo) loadRefundBatch(orders []model.Order) error {
	if len(orders) == 0 {
		return nil
	}
	orderIDs := make([]uint, 0, len(orders))
	for i := range orders {
		orderIDs = append(orderIDs, orders[i].ID)
	}
	var refunds []model.Refund
	if err := r.db.
		Preload("Negotiations", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("order_id IN ?", orderIDs).Find(&refunds).Error; err != nil {
		return fmt.Errorf("load refunds batch: %w", err)
	}
	byOrder := map[uint]*model.Refund{}
	for i := range refunds {
		byOrder[refunds[i].OrderID] = &refunds[i]
	}
	for i := range orders {
		rf := byOrder[orders[i].ID]
		if rf == nil {
			continue
		}
		orders[i].LastRefund = rf
		if orders[i].ActiveRefundID != nil && rf.ID == *orders[i].ActiveRefundID && !isFinalRefundStatus(rf.Status) {
			orders[i].ActiveRefund = rf
		}
	}
	return nil
}

// isFinalRefundStatus 判断售后状态是否已完结（agreed/rejected/cancelled）。
func isFinalRefundStatus(status string) bool {
	return status == constants.RefundStatusAgreed ||
		status == constants.RefundStatusRejected ||
		status == constants.RefundStatusCancelled
}

func (r *orderRepo) GetByIDForUpdate(tx *gorm.DB, id uint) (*model.Order, error) {
	var o model.Order
	err := tx.Clauses(clauseLocking()).First(&o, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get order by id %d for update: %w", id, err)
	}
	return &o, nil
}

func (r *orderRepo) ListByBuyer(buyerID uint, status string, page, pageSize int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64
	q := r.db.Model(&model.Order{}).Where("buyer_id = ?", buyerID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count buyer orders: %w", err)
	}
	if err := q.Preload("Product").Preload("Product.Seller").Preload("Address").
		Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("list buyer orders: %w", err)
	}
	if err := r.loadRefundBatch(orders); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (r *orderRepo) ListBySeller(sellerID uint, status string, page, pageSize int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64
	q := r.db.Model(&model.Order{}).Where("seller_id = ?", sellerID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count seller orders: %w", err)
	}
	if err := q.Preload("Product").Preload("Buyer").Preload("Address").
		Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&orders).Error; err != nil {
		return nil, 0, fmt.Errorf("list seller orders: %w", err)
	}
	if err := r.loadRefundBatch(orders); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

func (r *orderRepo) UpdateStatusForUpdate(tx *gorm.DB, id uint, status string, updates map[string]interface{}) error {
	updates["status"] = status
	res := tx.Model(&model.Order{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return fmt.Errorf("update order %d status to %s: %w", id, status, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetActiveRefundForUpdate 标记/解除订单“售后中”（同一事务内随售后状态一起提交）。
func (r *orderRepo) SetActiveRefundForUpdate(tx *gorm.DB, orderID uint, refundID *uint) error {
	res := tx.Model(&model.Order{}).Where("id = ?", orderID).Update("active_refund_id", refundID)
	if res.Error != nil {
		return fmt.Errorf("set order %d active refund %v: %w", orderID, refundID, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *orderRepo) Update(order *model.Order) error {
	if err := r.db.Save(order).Error; err != nil {
		return fmt.Errorf("update order %d: %w", order.ID, err)
	}
	return nil
}
