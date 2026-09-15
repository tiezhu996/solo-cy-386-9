// Package constants centralizes business enums, error codes, messages and log templates.
package constants

// OrderStatus 订单状态枚举（前端 constants/order.ts 同步维护）。
const (
	OrderStatusPendingPayment  string = "pending_payment"  // 待付款
	OrderStatusPendingShipment string = "pending_shipment" // 待发货
	OrderStatusShipped         string = "shipped"          // 已发货
	OrderStatusReceived        string = "received"         // 已收货
	OrderStatusCompleted       string = "completed"        // 已完成
	OrderStatusCancelled       string = "cancelled"        // 已取消
)

// ProductCondition 商品成色枚举。
const (
	ProductConditionBrandNew      string = "brand_new"      // 全新
	ProductConditionAlmostNew     string = "almost_new"     // 几乎全新
	ProductConditionLightlyUsed   string = "lightly_used"   // 轻微使用
	ProductConditionObviouslyUsed string = "obviously_used" // 明显使用
)

// ProductCategory 商品分类枚举。
const (
	ProductCategoryDigital  string = "digital"  // 数码
	ProductCategoryClothing string = "clothing" // 服饰
	ProductCategoryBooks    string = "books"    // 图书
	ProductCategoryHome     string = "home"     // 家居
	ProductCategorySports   string = "sports"   // 运动
	ProductCategoryOther    string = "other"    // 其他
)

// ProductStatus 商品上下架状态枚举。
const (
	ProductStatusOnSale   string = "on_sale"   // 在售
	ProductStatusSold     string = "sold"      // 已售出
	ProductStatusOffShelf string = "off_shelf" // 已下架
)

// UserRole 用户角色枚举。
const (
	UserRoleUser  string = "user"  // 普通用户
	UserRoleAdmin string = "admin" // 管理员
)

// ReviewRating 评价等级枚举。
const (
	ReviewRatingGood    string = "good"    // 好评
	ReviewRatingNeutral string = "neutral" // 中评
	ReviewRatingBad     string = "bad"     // 差评
)

// RefundType 售后类型枚举（买家完成交易前可发起一轮）。
const (
	RefundTypeReturn  string = "return_refund"  // 退货退款（全额）
	RefundTypePartial string = "partial_refund" // 部分退款（仅退款不退货）
)

// RefundStatus 售后单状态枚举（售后状态机，与订单状态机正交）。
const (
	RefundStatusPendingSeller   string = "pending_seller"   // 待卖家处理（买家可撤销）
	RefundStatusProposalPending string = "proposal_pending" // 卖家已提方案，待买家确认（买家可接受/撤销）
	RefundStatusAgreed          string = "agreed"           // 协商成功（终态，已记录退款）
	RefundStatusRejected        string = "rejected"         // 卖家拒绝（终态，订单恢复原状态）
	RefundStatusCancelled       string = "cancelled"        // 买家撤销（终态，订单恢复原状态）
)

// RefundAction 协商动作枚举（refund_negotiations 历史只追加，不改写）。
const (
	RefundActionApply   string = "apply"   // 买家发起售后
	RefundActionAgree   string = "agree"   // 卖家同意
	RefundActionReject  string = "reject"  // 卖家拒绝
	RefundActionPropose string = "propose" // 卖家提出方案（仅一次）
	RefundActionAccept  string = "accept"  // 买家接受方案
	RefundActionCancel  string = "cancel"  // 买家撤销
)

// RefundActorRole 协商历史操作者角色。
const (
	RefundActorBuyer  string = "buyer"
	RefundActorSeller string = "seller"
)

// OrderStatusTransitions 订单状态机：允许的流转映射（新状态 → 允许的前置状态集合）。
var OrderStatusTransitions = map[string][]string{
	OrderStatusPendingPayment:  {OrderStatusPendingPayment},
	OrderStatusPendingShipment: {OrderStatusPendingPayment},
	OrderStatusShipped:         {OrderStatusPendingShipment},
	OrderStatusReceived:        {OrderStatusShipped},
	OrderStatusCompleted:       {OrderStatusReceived},
	OrderStatusCancelled:       {OrderStatusPendingPayment, OrderStatusPendingShipment},
}

// ValidOrderStatus 校验订单状态值是否合法。
func ValidOrderStatus(status string) bool {
	switch status {
	case OrderStatusPendingPayment, OrderStatusPendingShipment, OrderStatusShipped,
		OrderStatusReceived, OrderStatusCompleted, OrderStatusCancelled:
		return true
	}
	return false
}

// ValidProductCondition 校验成色值是否合法。
func ValidProductCondition(cond string) bool {
	switch cond {
	case ProductConditionBrandNew, ProductConditionAlmostNew, ProductConditionLightlyUsed, ProductConditionObviouslyUsed:
		return true
	}
	return false
}

// ValidProductCategory 校验分类值是否合法。
func ValidProductCategory(cat string) bool {
	switch cat {
	case ProductCategoryDigital, ProductCategoryClothing, ProductCategoryBooks,
		ProductCategoryHome, ProductCategorySports, ProductCategoryOther:
		return true
	}
	return false
}

// ValidProductStatus 校验商品状态值是否合法。
func ValidProductStatus(status string) bool {
	switch status {
	case ProductStatusOnSale, ProductStatusSold, ProductStatusOffShelf:
		return true
	}
	return false
}

// ValidUserRole 校验角色值是否合法。
func ValidUserRole(role string) bool {
	return role == UserRoleUser || role == UserRoleAdmin
}

// ValidReviewRating 校验评价等级是否合法。
func ValidReviewRating(rating string) bool {
	switch rating {
	case ReviewRatingGood, ReviewRatingNeutral, ReviewRatingBad:
		return true
	}
	return false
}

// RefundStatusTransitions 售后状态机：新状态 → 允许的前置状态集合。
// pending_seller 为申请后的初始态；卖家提方案进入 proposal_pending；
// agreed/rejected/cancelled 均为终态，不允许再流转（并发处理只能有一个结果）。
var RefundStatusTransitions = map[string][]string{
	RefundStatusPendingSeller:   {RefundStatusPendingSeller},
	RefundStatusProposalPending: {RefundStatusPendingSeller},
	RefundStatusAgreed:          {RefundStatusPendingSeller, RefundStatusProposalPending},
	RefundStatusRejected:        {RefundStatusPendingSeller},
	RefundStatusCancelled:       {RefundStatusPendingSeller, RefundStatusProposalPending},
}

// RefundFinalStatuses 售后终态集合（终态后不能重复申请以外的任何操作）。
var RefundFinalStatuses = map[string]bool{
	RefundStatusAgreed:    true,
	RefundStatusRejected:  true,
	RefundStatusCancelled: true,
}

// CanRefundTransition 售后状态机校验（与 RefundStatusTransitions 对应）。
func CanRefundTransition(from, to string) bool {
	for _, s := range RefundStatusTransitions[to] {
		if s == from {
			return true
		}
	}
	return false
}

// ValidRefundType 校验售后类型是否合法。
func ValidRefundType(t string) bool {
	return t == RefundTypeReturn || t == RefundTypePartial
}

// ValidRefundStatus 校验售后状态是否合法。
func ValidRefundStatus(status string) bool {
	switch status {
	case RefundStatusPendingSeller, RefundStatusProposalPending,
		RefundStatusAgreed, RefundStatusRejected, RefundStatusCancelled:
		return true
	}
	return false
}

// ValidRefundAction 校验协商动作是否合法。
func ValidRefundAction(action string) bool {
	switch action {
	case RefundActionApply, RefundActionAgree, RefundActionReject,
		RefundActionPropose, RefundActionAccept, RefundActionCancel:
		return true
	}
	return false
}

// RefundableOrderStatuses 完成交易前允许发起售后的订单状态（已付款且未完成/未取消）。
var RefundableOrderStatuses = map[string]bool{
	OrderStatusPendingShipment: true,
	OrderStatusShipped:         true,
	OrderStatusReceived:        true,
}
