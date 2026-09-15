package router

import (
	"github.com/gin-gonic/gin"
	"github.com/marketpal/marketpal/internal/handler"
	"github.com/marketpal/marketpal/internal/middleware"
)

// RegisterRefundRoutes 售后模块路由（全部需要登录；买卖双方鉴权在 service 层完成）。
func RegisterRefundRoutes(api *gin.RouterGroup, h *handler.RefundHandler, secret string) {
	refunds := api.Group("/refunds", middleware.Auth(secret))
	{
		refunds.GET("", h.List)
		refunds.POST("", h.Apply)
		refunds.GET("/:id", h.Detail)
		refunds.POST("/:id/agree", h.Agree)
		refunds.POST("/:id/reject", h.Reject)
		refunds.POST("/:id/propose", h.Propose)
		refunds.POST("/:id/accept", h.Accept)
		refunds.POST("/:id/cancel", h.Cancel)
	}
	// 按订单回读售后单（订单详情页复用 RefundService.GetByOrder）。
	orders := api.Group("/orders", middleware.Auth(secret))
	{
		orders.GET("/:id/refund", h.ByOrder)
	}
}
