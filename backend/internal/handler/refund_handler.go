package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/dto"
	"github.com/marketpal/marketpal/internal/middleware"
	"github.com/marketpal/marketpal/internal/model"
	"github.com/marketpal/marketpal/internal/service"
	"github.com/marketpal/marketpal/internal/util"
)

// RefundHandler 售后 HTTP 处理器：申请、卖家同意/拒绝/提方案、买家接受/撤销、查询。
type RefundHandler struct {
	svc *service.RefundService
}

// NewRefundHandler 构造售后处理器。
func NewRefundHandler(svc *service.RefundService) *RefundHandler {
	return &RefundHandler{svc: svc}
}

// Apply POST /api/v1/refunds 买家发起售后（一轮）。
func (h *RefundHandler) Apply(c *gin.Context) {
	var req dto.RefundApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "发起售后失败：参数校验不通过 "+err.Error())
		return
	}
	rf, err := h.svc.Apply(middleware.GetUserID(c), req)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundApplied, toRefundVO(rf))
}

// Agree POST /api/v1/refunds/:id/agree 卖家同意。
func (h *RefundHandler) Agree(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	rf, err := h.svc.Agree(middleware.GetUserID(c), id)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundAgreed, toRefundVO(rf))
}

// Reject POST /api/v1/refunds/:id/reject 卖家拒绝。
func (h *RefundHandler) Reject(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	var req dto.RefundRejectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "拒绝售后失败：参数校验不通过 "+err.Error())
		return
	}
	rf, err := h.svc.Reject(middleware.GetUserID(c), id, req)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundRejected, toRefundVO(rf))
}

// Propose POST /api/v1/refunds/:id/propose 卖家提出一次方案。
func (h *RefundHandler) Propose(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	var req dto.RefundProposeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "提出方案失败：参数校验不通过 "+err.Error())
		return
	}
	rf, err := h.svc.Propose(middleware.GetUserID(c), id, req)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundProposed, toRefundVO(rf))
}

// Accept POST /api/v1/refunds/:id/accept 买家接受方案。
func (h *RefundHandler) Accept(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	rf, err := h.svc.Accept(middleware.GetUserID(c), id)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundAccepted, toRefundVO(rf))
}

// Cancel POST /api/v1/refunds/:id/cancel 买家撤销（卖家处理前/方案待确认）。
func (h *RefundHandler) Cancel(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	rf, err := h.svc.Cancel(middleware.GetUserID(c), id)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OKMessage(c, constants.MsgRefundCancelled, toRefundVO(rf))
}

// Detail GET /api/v1/refunds/:id 售后详情（仅买卖双方）。
func (h *RefundHandler) Detail(c *gin.Context) {
	id, ok := parseRefundID(c)
	if !ok {
		return
	}
	vo, err := h.svc.GetDetail(middleware.GetUserID(c), id)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OK(c, vo)
}

// ByOrder GET /api/v1/orders/:id/refund 按订单回读售后（仅买卖双方）。
func (h *RefundHandler) ByOrder(c *gin.Context) {
	orderID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "售后查询失败：订单 id 参数非法")
		return
	}
	vo, err := h.svc.GetByOrder(middleware.GetUserID(c), uint(orderID))
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OK(c, vo)
}

// List GET /api/v1/refunds 售后列表（buyer/seller 双视角）。
func (h *RefundHandler) List(c *gin.Context) {
	var q dto.RefundQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "售后列表失败：查询参数不合法 "+err.Error())
		return
	}
	res, err := h.svc.List(middleware.GetUserID(c), q)
	if err != nil {
		util.AbortWithError(c, err)
		return
	}
	util.OK(c, res)
}

// parseRefundID 解析售后单 id 参数。
func parseRefundID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		util.Fail(c, http.StatusBadRequest, constants.CodeBadRequest, "售后处理失败：售后单 id 参数非法")
		return 0, false
	}
	return uint(id), true
}

// toRefundVO model.Refund → dto.RefundVO（凭证字符串转数组，附带预加载的订单快照）。
func toRefundVO(rf *model.Refund) dto.RefundVO {
	vo := dto.FromRefund(rf)
	if rf.Order != nil {
		orderVO := dto.FromOrder(rf.Order)
		vo.Order = &orderVO
	}
	return vo
}
