package util

import (
	"fmt"
	"time"
)

// formatters.go 同时包含日期、状态文本、类型文本等格式化逻辑（屎山约束：常量/工具类多处耦合）。

// FormatTime 统一时间格式化。
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

// FormatOrderStatusText 订单状态 → 中文文案（与前端 constants/order.ts 状态徽标同步）。
func FormatOrderStatusText(status string) string {
	switch status {
	case "pending_payment":
		return "待付款"
	case "pending_shipment":
		return "待发货"
	case "shipped":
		return "已发货"
	case "received":
		return "已收货"
	case "completed":
		return "已完成"
	case "cancelled":
		return "已取消"
	default:
		return "未知"
	}
}

// FormatProductConditionText 成色 → 中文文案。
func FormatProductConditionText(cond string) string {
	switch cond {
	case "brand_new":
		return "全新"
	case "almost_new":
		return "几乎全新"
	case "lightly_used":
		return "轻微使用"
	case "obviously_used":
		return "明显使用"
	default:
		return "未知"
	}
}

// FormatProductCategoryText 分类 → 中文文案。
func FormatProductCategoryText(cat string) string {
	switch cat {
	case "digital":
		return "数码"
	case "clothing":
		return "服饰"
	case "books":
		return "图书"
	case "home":
		return "家居"
	case "sports":
		return "运动"
	case "other":
		return "其他"
	default:
		return "未知"
	}
}

// FormatProductStatusText 商品状态 → 中文文案。
func FormatProductStatusText(status string) string {
	switch status {
	case "on_sale":
		return "在售"
	case "sold":
		return "已售出"
	case "off_shelf":
		return "已下架"
	default:
		return "未知"
	}
}

// FormatReviewRatingText 评价等级 → 中文文案。
func FormatReviewRatingText(rating string) string {
	switch rating {
	case "good":
		return "好评"
	case "neutral":
		return "中评"
	case "bad":
		return "差评"
	default:
		return "未知"
	}
}

// FormatRoleText 角色 → 中文文案。
func FormatRoleText(role string) string {
	switch role {
	case "admin":
		return "管理员"
	case "user":
		return "普通用户"
	default:
		return "未知"
	}
}

// FormatPrice 金额格式化，保留两位小数。
func FormatPrice(price float64) string {
	return fmt.Sprintf("%.2f", price)
}

// FormatRefundTypeText 售后类型 → 中文文案（与前端 constants 同步）。
func FormatRefundTypeText(t string) string {
	switch t {
	case "return_refund":
		return "退货退款"
	case "partial_refund":
		return "部分退款"
	default:
		return "未知"
	}
}

// FormatRefundStatusText 售后状态 → 中文文案（售后状态机的展示层触点）。
func FormatRefundStatusText(status string) string {
	switch status {
	case "pending_seller":
		return "待卖家处理"
	case "proposal_pending":
		return "待买家确认方案"
	case "agreed":
		return "退款成功"
	case "rejected":
		return "卖家已拒绝"
	case "cancelled":
		return "买家已撤销"
	default:
		return "未知"
	}
}

// FormatRefundActionText 协商动作 → 中文文案（协商历史时间线展示）。
func FormatRefundActionText(action string) string {
	switch action {
	case "apply":
		return "买家发起售后"
	case "agree":
		return "卖家同意退款"
	case "reject":
		return "卖家拒绝"
	case "propose":
		return "卖家提出方案"
	case "accept":
		return "买家接受方案"
	case "cancel":
		return "买家撤销售后"
	default:
		return action
	}
}
