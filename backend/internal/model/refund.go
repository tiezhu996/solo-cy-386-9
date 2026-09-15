package model

import "time"

// Refund 售后单实体：买家在完成交易前对已付款订单发起一轮退货退款/部分退款。
// 状态机见 constants.RefundStatusTransitions 与 service/refund_service.go；
// 一个订单终身至多一条售后单（order_id 唯一索引），杜绝重复申请。
type Refund struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	RefundNo       string     `gorm:"size:40;uniqueIndex;not null" json:"refund_no"`
	OrderID        uint       `gorm:"not null;uniqueIndex" json:"order_id"`
	BuyerID        uint       `gorm:"not null;index" json:"buyer_id"`
	SellerID       uint       `gorm:"not null;index" json:"seller_id"`
	Type           string     `gorm:"size:32;not null" json:"type"` // return_refund / partial_refund
	Reason         string     `gorm:"size:500;not null" json:"reason"`
	ApplyAmount    float64    `gorm:"type:numeric(12,2);not null" json:"apply_amount"`
	Evidence       string     `gorm:"type:text" json:"evidence"` // 逗号分隔的凭证图片 URL
	Status         string     `gorm:"size:32;not null;default:pending_seller;index" json:"status"`
	ProposalAmount *float64   `gorm:"type:numeric(12,2)" json:"proposal_amount,omitempty"` // 卖家一次方案金额
	ProposalReason string     `gorm:"size:255" json:"proposal_reason"`
	FinalAmount    *float64   `gorm:"type:numeric(12,2)" json:"final_amount,omitempty"` // 协商成功后的退款结果
	RefundedAt     *time.Time `json:"refunded_at,omitempty"`
	ClosedAt       *time.Time `json:"closed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`

	Order        *Order              `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	Negotiations []RefundNegotiation `gorm:"foreignKey:RefundID" json:"negotiations,omitempty"`
}

// RefundNegotiation 售后协商历史：只追加（INSERT）、永不修改、永不删除，
// 完整记录申请 → 同意/拒绝/方案 → 接受/撤销，保证协商历史可一致回读。
type RefundNegotiation struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	RefundID  uint      `gorm:"not null;index" json:"refund_id"`
	OrderID   uint      `gorm:"not null;index" json:"order_id"`
	ActorID   uint      `gorm:"not null" json:"actor_id"`
	ActorRole string    `gorm:"size:16;not null" json:"actor_role"` // buyer / seller
	Action    string    `gorm:"size:32;not null" json:"action"`     // apply/agree/reject/propose/accept/cancel
	Amount    float64   `gorm:"type:numeric(12,2);not null;default:0" json:"amount"`
	Remark    string    `gorm:"size:500" json:"remark"`
	Evidence  string    `gorm:"type:text" json:"evidence,omitempty"`
	CreatedAt time.Time `json:"created_at"`

	Refund *Refund `gorm:"foreignKey:RefundID" json:"-"`
}
