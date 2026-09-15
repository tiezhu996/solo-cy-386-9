package dto

// RefundApplyRequest 买家发起售后入参（一轮）：原因、金额、凭证。
type RefundApplyRequest struct {
	OrderID  uint     `json:"order_id" binding:"required"`
	Type     string   `json:"type" binding:"required,oneof=return_refund partial_refund"`
	Reason   string   `json:"reason" binding:"required,min=2,max=500"`
	Amount   float64  `json:"amount" binding:"required,gt=0"`
	Evidence []string `json:"evidence" binding:"omitempty,max=9,dive,max=512"`
}

// RefundRejectRequest 卖家拒绝售后入参。
type RefundRejectRequest struct {
	Reason string `json:"reason" binding:"required,min=2,max=255"`
}

// RefundProposeRequest 卖家提出一次方案入参（每轮售后仅可提一次）。
type RefundProposeRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Reason string  `json:"reason" binding:"required,min=2,max=255"`
}

// RefundQuery 售后列表查询入参（买/卖双视角）。
type RefundQuery struct {
	Status   string `form:"status" binding:"omitempty,oneof=pending_seller proposal_pending agreed rejected cancelled"`
	Role     string `form:"role" binding:"omitempty,oneof=buyer seller"`
	Page     int    `form:"page" binding:"omitempty,min=1"`
	PageSize int    `form:"page_size" binding:"omitempty,min=1,max=50"`
}

// RefundNegotiationVO 协商历史视图对象（只追加记录的只读回读）。
type RefundNegotiationVO struct {
	ID        uint     `json:"id"`
	ActorID   uint     `json:"actor_id"`
	ActorRole string   `json:"actor_role"`
	Action    string   `json:"action"`
	Amount    float64  `json:"amount"`
	Remark    string   `json:"remark"`
	Evidence  []string `json:"evidence,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// RefundVO 售后单视图对象（含退款结果与协商历史，订单/退款/历史一致回读）。
type RefundVO struct {
	ID              uint                  `json:"id"`
	RefundNo        string                `json:"refund_no"`
	OrderID         uint                  `json:"order_id"`
	OrderNo         string                `json:"order_no,omitempty"`
	BuyerID         uint                  `json:"buyer_id"`
	SellerID        uint                  `json:"seller_id"`
	Type            string                `json:"type"`
	Reason          string                `json:"reason"`
	ApplyAmount     float64               `json:"apply_amount"`
	Evidence        []string              `json:"evidence,omitempty"`
	Status          string                `json:"status"`
	ProposalAmount  *float64              `json:"proposal_amount,omitempty"`
	ProposalReason  string                `json:"proposal_reason,omitempty"`
	FinalAmount     *float64              `json:"final_amount,omitempty"`
	OrderStatus     string                `json:"order_status,omitempty"`
	OrderPaidAmount float64               `json:"order_paid_amount,omitempty"`
	RefundedAt      *string               `json:"refunded_at,omitempty"`
	ClosedAt        *string               `json:"closed_at,omitempty"`
	CreatedAt       string                `json:"created_at"`
	Negotiations    []RefundNegotiationVO `json:"negotiations"`
	Order           *OrderVO              `json:"order,omitempty"`
}

// RefundListResponse 售后单分页响应（买/卖双视角复用）。
type RefundListResponse struct {
	List  []RefundVO `json:"list"`
	Total int64      `json:"total"`
	Page  int        `json:"page"`
	Size  int        `json:"page_size"`
}
