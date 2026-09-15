package dto

import (
	"strings"
	"time"

	"github.com/marketpal/marketpal/internal/model"
)

// vo.go 集中 model → 视图对象 转换（handler/service 复用，避免重复代码）。

// FromUser 用户实体转视图对象。
func FromUser(u *model.User) *UserVO {
	return &UserVO{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Phone:       u.Phone,
		Avatar:      u.Avatar,
		Role:        u.Role,
		CreditScore: u.CreditScore,
		Status:      u.Status,
		CreatedAt:   formatTime(u.CreatedAt),
	}
}

// FromProduct 商品实体转视图对象。
func FromProduct(p *model.Product, isFavorite bool) ProductVO {
	var seller *UserVO
	if p.Seller != nil {
		seller = FromUser(p.Seller)
	}
	images := []string{}
	if p.Images != "" {
		images = strings.Split(p.Images, ",")
	}
	return ProductVO{
		ID:            p.ID,
		SellerID:      p.SellerID,
		Title:         p.Title,
		Description:   p.Description,
		OriginalPrice: p.OriginalPrice,
		Price:         p.Price,
		Condition:     p.Condition,
		Category:      p.Category,
		Images:        images,
		Status:        p.Status,
		ViewCount:     p.ViewCount,
		FavoriteCount: p.FavoriteCount,
		CreatedAt:     formatTime(p.CreatedAt),
		Seller:        seller,
		IsFavorite:    isFavorite,
	}
}

// FromAddress 地址实体转视图对象。
func FromAddress(a *model.Address) AddressVO {
	return AddressVO{
		ID:           a.ID,
		ReceiverName: a.ReceiverName,
		Phone:        a.Phone,
		Province:     a.Province,
		City:         a.City,
		District:     a.District,
		Detail:       a.Detail,
		IsDefault:    a.IsDefault,
	}
}

// FromOrder 订单实体转视图对象（订单列表/详情/售后嵌套共用，保证回读一致）。
func FromOrder(o *model.Order) OrderVO {
	vo := OrderVO{
		ID:         o.ID,
		OrderNo:    o.OrderNo,
		BuyerID:    o.BuyerID,
		SellerID:   o.SellerID,
		ProductID:  o.ProductID,
		AddressID:  o.AddressID,
		Quantity:   o.Quantity,
		TotalPrice: o.TotalPrice,
		Status:     o.Status,
		Remark:     o.Remark,
		CreatedAt:  formatTime(o.CreatedAt),
	}
	if o.PaidAt != nil {
		t := formatTime(*o.PaidAt)
		vo.PaidAt = &t
	}
	if o.ShippedAt != nil {
		t := formatTime(*o.ShippedAt)
		vo.ShippedAt = &t
	}
	if o.ReceivedAt != nil {
		t := formatTime(*o.ReceivedAt)
		vo.ReceivedAt = &t
	}
	if o.Product != nil {
		prod := FromProduct(o.Product, false)
		vo.Product = &prod
	}
	if o.Address != nil {
		addr := FromAddress(o.Address)
		vo.Address = &addr
	}
	if o.Buyer != nil {
		vo.Buyer = FromUser(o.Buyer)
	}
	if o.Seller != nil {
		vo.Seller = FromUser(o.Seller)
	}
	if o.ActiveRefund != nil {
		rf := FromRefund(o.ActiveRefund)
		vo.ActiveRefund = &rf
	}
	if o.LastRefund != nil {
		rf := FromRefund(o.LastRefund)
		vo.LastRefund = &rf
	}
	return vo
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// splitImages 与 Product.Images 相同的逗号分隔图片存储约定。
func splitImages(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, ",")
}

// FromRefundNegotiation 协商历史实体转视图对象（只追加记录的只读回读）。
func FromRefundNegotiation(n *model.RefundNegotiation) RefundNegotiationVO {
	return RefundNegotiationVO{
		ID:        n.ID,
		ActorID:   n.ActorID,
		ActorRole: n.ActorRole,
		Action:    n.Action,
		Amount:    n.Amount,
		Remark:    n.Remark,
		Evidence:  splitImages(n.Evidence),
		CreatedAt: formatTime(n.CreatedAt),
	}
}

// FromRefund 售后单实体转视图对象（不嵌套订单，避免循环；由 service 组合 Order 字段）。
func FromRefund(r *model.Refund) RefundVO {
	vo := RefundVO{
		ID:             r.ID,
		RefundNo:       r.RefundNo,
		OrderID:        r.OrderID,
		BuyerID:        r.BuyerID,
		SellerID:       r.SellerID,
		Type:           r.Type,
		Reason:         r.Reason,
		ApplyAmount:    r.ApplyAmount,
		Evidence:       splitImages(r.Evidence),
		Status:         r.Status,
		ProposalReason: r.ProposalReason,
		Negotiations:   make([]RefundNegotiationVO, 0, len(r.Negotiations)),
		CreatedAt:      formatTime(r.CreatedAt),
	}
	if r.Order != nil {
		vo.OrderNo = r.Order.OrderNo
		vo.OrderStatus = r.Order.Status
		vo.OrderPaidAmount = r.Order.TotalPrice
	}
	if r.ProposalAmount != nil {
		v := *r.ProposalAmount
		vo.ProposalAmount = &v
	}
	if r.FinalAmount != nil {
		v := *r.FinalAmount
		vo.FinalAmount = &v
	}
	if r.RefundedAt != nil {
		t := formatTime(*r.RefundedAt)
		vo.RefundedAt = &t
	}
	if r.ClosedAt != nil {
		t := formatTime(*r.ClosedAt)
		vo.ClosedAt = &t
	}
	for i := range r.Negotiations {
		vo.Negotiations = append(vo.Negotiations, FromRefundNegotiation(&r.Negotiations[i]))
	}
	return vo
}
