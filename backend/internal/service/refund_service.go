package service

import (
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/dto"
	"github.com/marketpal/marketpal/internal/model"
	"github.com/marketpal/marketpal/internal/repository"
	"gorm.io/gorm"
)

// amountEpsilon 金额比较容差（退款金额不能超过实付金额）。
const amountEpsilon = 1e-9

// RefundService 售后业务服务：买家一轮申请 → 卖家同意/拒绝/一次方案 → 买家接受/撤销。
// 所有多步写操作均在同一事务内完成，并以固定顺序（订单行 → 售后单行）SELECT ... FOR UPDATE，
// 配合售后单状态条件更新，保证并发处理只能有一个结果、重复提交不改写记录。
type RefundService struct {
	refundRepo  repository.RefundRepository
	orderRepo   repository.OrderRepository
	productRepo repository.ProductRepository
	db          *gorm.DB
	logger      *slog.Logger
}

// NewRefundService 构造售后服务。
func NewRefundService(db *gorm.DB, refundRepo repository.RefundRepository, orderRepo repository.OrderRepository, productRepo repository.ProductRepository, logger *slog.Logger) *RefundService {
	return &RefundService{db: db, refundRepo: refundRepo, orderRepo: orderRepo, productRepo: productRepo, logger: logger}
}

// Apply 买家发起售后（一轮）：已付款且完成交易前的订单可申请退货退款/部分退款。
// 重复提交相同申请单幂等返回原记录，不改写任何数据；一个订单终身至多一条售后单。
func (s *RefundService) Apply(buyerID uint, req dto.RefundApplyRequest) (*model.Refund, error) {
	evidence := strings.Join(req.Evidence, ",")
	var refund *model.Refund
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// 固定加锁顺序：先订单行，再售后单行（与所有售后动作一致，避免交叉死锁）。
		o, err := s.orderRepo.GetByIDForUpdate(tx, req.OrderID)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return utilAppError(constants.CodeOrderNotFound, "发起售后失败：订单 id="+fmt.Sprint(req.OrderID)+" 不存在", err)
			}
			return fmt.Errorf("lock order %d for refund apply: %w", req.OrderID, err)
		}
		if o.BuyerID != buyerID {
			return utilAppError(constants.CodeNotOrderOwner, "发起售后失败：买家 id="+fmt.Sprint(buyerID)+" 无权为订单 "+o.OrderNo+" 申请售后", nil)
		}
		if o.PaidAt == nil || !constants.RefundableOrderStatuses[o.Status] {
			return utilAppError(constants.CodeOrderStateInvalid, "发起售后失败：订单 "+o.OrderNo+" 当前状态为 "+o.Status+"，仅已付款且完成交易前可申请售后", nil)
		}
		// 退货退款必须按实付金额全额申请（同意后订单将取消并重新上架，不能只退部分金额）；
		// 部分退款只要求 0 < 金额 ≤ 实付，两者规则在此明确区分。
		if err := validateApplyAmount(req.Type, req.Amount, o.TotalPrice, o.OrderNo); err != nil {
			return err
		}
		existing, err := s.refundRepo.GetByOrderIDForUpdate(tx, o.ID)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return fmt.Errorf("check existing refund order=%d: %w", o.ID, err)
		}
		if err == nil {
			// 终态售后单：拒绝/撤销/成功后均不能重复申请。
			if constants.RefundFinalStatuses[existing.Status] {
				return utilAppError(constants.CodeRefundExists, "发起售后失败：订单 "+o.OrderNo+" 已有完结售后单 "+existing.RefundNo+"，不能重复申请", nil)
			}
			// 处理中：仅完全相同的重复提交幂等返回原记录，任何差异都判冲突，杜绝改写。
			if existing.Type == req.Type && existing.Reason == req.Reason &&
				math.Abs(existing.ApplyAmount-req.Amount) < amountEpsilon && existing.Evidence == evidence {
				refund = existing
				return nil
			}
			return utilAppError(constants.CodeRefundExists, "发起售后失败：订单 "+o.OrderNo+" 售后单 "+existing.RefundNo+" 处理中，不能重复申请", nil)
		}

		now := time.Now()
		refund = &model.Refund{
			RefundNo:    genRefundNo(),
			OrderID:     o.ID,
			BuyerID:     o.BuyerID,
			SellerID:    o.SellerID,
			Type:        req.Type,
			Reason:      req.Reason,
			ApplyAmount: req.Amount,
			Evidence:    evidence,
			Status:      constants.RefundStatusPendingSeller,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.refundRepo.CreateWithTx(tx, refund); err != nil {
			if errors.Is(err, repository.ErrRefundExists) {
				return utilAppError(constants.CodeRefundExists, "发起售后失败：订单 "+o.OrderNo+" 已存在售后单，不能重复申请", err)
			}
			return fmt.Errorf("create refund order=%d: %w", o.ID, err)
		}
		if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
			RefundID: refund.ID, OrderID: o.ID, ActorID: buyerID, ActorRole: constants.RefundActorBuyer,
			Action: constants.RefundActionApply, Amount: req.Amount, Remark: req.Reason, Evidence: evidence, CreatedAt: now,
		}); err != nil {
			return err
		}
		// 订单进入“售后中”：暂停发货、收货、完成与评价。
		if err := s.orderRepo.SetActiveRefundForUpdate(tx, o.ID, &refund.ID); err != nil {
			return fmt.Errorf("mark order %d in refund: %w", o.ID, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	full, err := s.refundRepo.GetByID(refund.ID)
	if err != nil {
		return nil, fmt.Errorf("reload refund %d after apply: %w", refund.ID, err)
	}
	s.logger.Info(constants.LogRefundApplied, "refund_id", full.ID, "order_no", full.Order.OrderNo, "buyer_id", buyerID, "type", full.Type, "amount", full.ApplyAmount, "status", full.Status)
	return full, nil
}

// Agree 卖家同意售后（仅待卖家处理阶段）：按买家申请金额退款成功并结束售后。
func (s *RefundService) Agree(sellerID, refundID uint) (*model.Refund, error) {
	return s.sellerResolve(sellerID, refundID, constants.RefundActionAgree, 0, "")
}

// Reject 卖家拒绝售后（仅待卖家处理阶段）：售后关闭，订单恢复原状态。
func (s *RefundService) Reject(sellerID, refundID uint, req dto.RefundRejectRequest) (*model.Refund, error) {
	return s.sellerResolve(sellerID, refundID, constants.RefundActionReject, 0, req.Reason)
}

// Propose 卖家提出一次方案（仅待卖家处理阶段，每轮仅一次）：进入待买家确认。
func (s *RefundService) Propose(sellerID, refundID uint, req dto.RefundProposeRequest) (*model.Refund, error) {
	return s.sellerResolve(sellerID, refundID, constants.RefundActionPropose, req.Amount, req.Reason)
}

// sellerResolve 卖家三个动作共用同一加锁/鉴权/状态机流程。
func (s *RefundService) sellerResolve(sellerID, refundID uint, action string, amount float64, reason string) (*model.Refund, error) {
	header, err := s.refundRepo.GetByID(refundID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, utilAppError(constants.CodeRefundNotFound, "售后处理失败：售后单 id="+fmt.Sprint(refundID)+" 不存在", err)
		}
		return nil, fmt.Errorf("load refund %d: %w", refundID, err)
	}
	orderID := header.OrderID
	var refund *model.Refund
	err = s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.orderRepo.GetByIDForUpdate(tx, orderID)
		if err != nil {
			return fmt.Errorf("lock order %d for refund action: %w", orderID, err)
		}
		rf, err := s.refundRepo.GetByIDForUpdate(tx, refundID)
		if err != nil {
			return fmt.Errorf("lock refund %d: %w", refundID, err)
		}
		if o.SellerID != sellerID {
			return utilAppError(constants.CodeNotRefundParty, "售后处理失败：卖家 id="+fmt.Sprint(sellerID)+" 非售后单 "+rf.RefundNo+" 的卖家，无权操作", nil)
		}
		now := time.Now()
		switch action {
		case constants.RefundActionAgree:
			if !constants.CanRefundTransition(rf.Status, constants.RefundStatusAgreed) || rf.Status != constants.RefundStatusPendingSeller {
				return utilAppError(constants.CodeRefundStateInvalid, "同意售后失败：售后单 "+rf.RefundNo+" 当前状态 "+rf.Status+" 不允许卖家同意", nil)
			}
			// 与申请环节同一套金额规则：退货退款必须全额，部分退款不超过实付。
			if err := validateApplyAmount(rf.Type, rf.ApplyAmount, o.TotalPrice, o.OrderNo); err != nil {
				return err
			}
			finalAmount := rf.ApplyAmount
			ok, err := s.refundRepo.TransitForUpdate(tx, rf.ID, []string{constants.RefundStatusPendingSeller}, map[string]interface{}{
				"status": constants.RefundStatusAgreed, "final_amount": finalAmount, "refunded_at": &now, "closed_at": &now, "updated_at": now,
			})
			if err != nil {
				return err
			}
			if !ok {
				return utilAppError(constants.CodeRefundStateInvalid, "同意售后失败：售后单 "+rf.RefundNo+" 已被并发处理，只能有一个结果", nil)
			}
			if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
				RefundID: rf.ID, OrderID: o.ID, ActorID: sellerID, ActorRole: constants.RefundActorSeller,
				Action: constants.RefundActionAgree, Amount: finalAmount, Remark: "卖家同意退款", CreatedAt: now,
			}); err != nil {
				return err
			}
			if err := s.applyRefundResult(tx, o, rf, finalAmount, now); err != nil {
				return err
			}
			rf.Status = constants.RefundStatusAgreed
			refund = rf
		case constants.RefundActionReject:
			if rf.Status != constants.RefundStatusPendingSeller {
				return utilAppError(constants.CodeRefundStateInvalid, "拒绝售后失败：售后单 "+rf.RefundNo+" 当前状态 "+rf.Status+" 不允许卖家拒绝", nil)
			}
			ok, err := s.refundRepo.TransitForUpdate(tx, rf.ID, []string{constants.RefundStatusPendingSeller}, map[string]interface{}{
				"status": constants.RefundStatusRejected, "closed_at": &now, "updated_at": now,
			})
			if err != nil {
				return err
			}
			if !ok {
				return utilAppError(constants.CodeRefundStateInvalid, "拒绝售后失败：售后单 "+rf.RefundNo+" 已被并发处理，只能有一个结果", nil)
			}
			if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
				RefundID: rf.ID, OrderID: o.ID, ActorID: sellerID, ActorRole: constants.RefundActorSeller,
				Action: constants.RefundActionReject, Amount: 0, Remark: reason, CreatedAt: now,
			}); err != nil {
				return err
			}
			// 拒绝后解除“售后中”，订单保持原状态不变（恢复原状态）。
			if err := s.orderRepo.SetActiveRefundForUpdate(tx, o.ID, nil); err != nil {
				return fmt.Errorf("resume order %d after reject: %w", o.ID, err)
			}
			rf.Status = constants.RefundStatusRejected
			refund = rf
		case constants.RefundActionPropose:
			if rf.Status != constants.RefundStatusPendingSeller {
				return utilAppError(constants.CodeRefundStateInvalid, "提出方案失败：售后单 "+rf.RefundNo+" 当前状态 "+rf.Status+" 不允许再提方案（每轮仅一次）", nil)
			}
			// 退货退款只能全额：卖家不能用低于实付的方案替代，请直接同意或拒绝。
			if rf.Type == constants.RefundTypeReturn {
				return utilAppError(constants.CodeRefundStateInvalid, "提出方案失败：退货退款售后单 "+rf.RefundNo+" 只能按实付全额处理，不能提出部分退款方案，请直接同意或拒绝", nil)
			}
			if amount <= 0 || amount > o.TotalPrice+amountEpsilon {
				return utilAppError(constants.CodeRefundAmountExceed, fmt.Sprintf("提出方案失败：方案金额 %.2f 必须大于 0 且不超过订单 %s 实付金额 %.2f", amount, o.OrderNo, o.TotalPrice), nil)
			}
			ok, err := s.refundRepo.TransitForUpdate(tx, rf.ID, []string{constants.RefundStatusPendingSeller}, map[string]interface{}{
				"status": constants.RefundStatusProposalPending, "proposal_amount": amount, "proposal_reason": reason, "updated_at": now,
			})
			if err != nil {
				return err
			}
			if !ok {
				return utilAppError(constants.CodeRefundStateInvalid, "提出方案失败：售后单 "+rf.RefundNo+" 已被并发处理，只能有一个结果", nil)
			}
			if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
				RefundID: rf.ID, OrderID: o.ID, ActorID: sellerID, ActorRole: constants.RefundActorSeller,
				Action: constants.RefundActionPropose, Amount: amount, Remark: reason, CreatedAt: now,
			}); err != nil {
				return err
			}
			rf.Status = constants.RefundStatusProposalPending
			refund = rf
		default:
			return utilAppError(constants.CodeBadRequest, "售后处理失败：未知卖家动作 "+action, nil)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	switch action {
	case constants.RefundActionAgree:
		full, _ := s.refundRepo.GetByID(refund.ID)
		s.logger.Info(constants.LogRefundAgreed, "refund_id", refund.ID, "order_id", orderID, "seller_id", sellerID, "refund_amount", refundAmountValue(full), "status", refund.Status)
	case constants.RefundActionReject:
		s.logger.Info(constants.LogRefundRejected, "refund_id", refund.ID, "order_id", orderID, "seller_id", sellerID, "status", refund.Status)
	case constants.RefundActionPropose:
		s.logger.Info(constants.LogRefundProposed, "refund_id", refund.ID, "order_id", orderID, "seller_id", sellerID, "amount", amount, "status", refund.Status)
	}
	return s.refundRepo.GetByID(refund.ID)
}

// Accept 买家接受卖家方案：按方案金额退款成功并结束售后。
func (s *RefundService) Accept(buyerID, refundID uint) (*model.Refund, error) {
	header, err := s.refundRepo.GetByID(refundID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, utilAppError(constants.CodeRefundNotFound, "接受方案失败：售后单 id="+fmt.Sprint(refundID)+" 不存在", err)
		}
		return nil, fmt.Errorf("load refund %d: %w", refundID, err)
	}
	orderID := header.OrderID
	var refund *model.Refund
	var acceptedAmount float64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.orderRepo.GetByIDForUpdate(tx, orderID)
		if err != nil {
			return fmt.Errorf("lock order %d for refund accept: %w", orderID, err)
		}
		rf, err := s.refundRepo.GetByIDForUpdate(tx, refundID)
		if err != nil {
			return fmt.Errorf("lock refund %d: %w", refundID, err)
		}
		if o.BuyerID != buyerID {
			return utilAppError(constants.CodeNotRefundParty, "接受方案失败：买家 id="+fmt.Sprint(buyerID)+" 非售后单 "+rf.RefundNo+" 的买家，无权操作", nil)
		}
		if rf.Status != constants.RefundStatusProposalPending || rf.ProposalAmount == nil {
			return utilAppError(constants.CodeRefundStateInvalid, "接受方案失败：售后单 "+rf.RefundNo+" 当前状态 "+rf.Status+" 没有待确认的方案", nil)
		}
		finalAmount := *rf.ProposalAmount
		if finalAmount > o.TotalPrice+amountEpsilon {
			return utilAppError(constants.CodeRefundAmountExceed, fmt.Sprintf("接受方案失败：退款金额 %.2f 超过订单 %s 实付金额 %.2f", finalAmount, o.OrderNo, o.TotalPrice), nil)
		}
		now := time.Now()
		ok, err := s.refundRepo.TransitForUpdate(tx, rf.ID, []string{constants.RefundStatusProposalPending}, map[string]interface{}{
			"status": constants.RefundStatusAgreed, "final_amount": finalAmount, "refunded_at": &now, "closed_at": &now, "updated_at": now,
		})
		if err != nil {
			return err
		}
		if !ok {
			return utilAppError(constants.CodeRefundStateInvalid, "接受方案失败：售后单 "+rf.RefundNo+" 已被并发处理，只能有一个结果", nil)
		}
		if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
			RefundID: rf.ID, OrderID: o.ID, ActorID: buyerID, ActorRole: constants.RefundActorBuyer,
			Action: constants.RefundActionAccept, Amount: finalAmount, Remark: "买家接受退款方案", CreatedAt: now,
		}); err != nil {
			return err
		}
		if err := s.applyRefundResult(tx, o, rf, finalAmount, now); err != nil {
			return err
		}
		rf.Status = constants.RefundStatusAgreed
		refund = rf
		acceptedAmount = finalAmount
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.logger.Info(constants.LogRefundAccepted, "refund_id", refund.ID, "order_id", orderID, "buyer_id", buyerID, "refund_amount", acceptedAmount, "status", refund.Status)
	return s.refundRepo.GetByID(refund.ID)
}

// Cancel 买家撤销售后（卖家处理前，或方案待确认阶段）：售后关闭，订单恢复原状态。
func (s *RefundService) Cancel(buyerID, refundID uint) (*model.Refund, error) {
	header, err := s.refundRepo.GetByID(refundID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, utilAppError(constants.CodeRefundNotFound, "撤销售后失败：售后单 id="+fmt.Sprint(refundID)+" 不存在", err)
		}
		return nil, fmt.Errorf("load refund %d: %w", refundID, err)
	}
	orderID := header.OrderID
	var refund *model.Refund

	err = s.db.Transaction(func(tx *gorm.DB) error {
		o, err := s.orderRepo.GetByIDForUpdate(tx, orderID)
		if err != nil {
			return fmt.Errorf("lock order %d for refund cancel: %w", orderID, err)
		}
		rf, err := s.refundRepo.GetByIDForUpdate(tx, refundID)
		if err != nil {
			return fmt.Errorf("lock refund %d: %w", refundID, err)
		}
		if o.BuyerID != buyerID {
			return utilAppError(constants.CodeNotRefundParty, "撤销售后失败：买家 id="+fmt.Sprint(buyerID)+" 非售后单 "+rf.RefundNo+" 的买家，无权操作", nil)
		}
		if rf.Status != constants.RefundStatusPendingSeller && rf.Status != constants.RefundStatusProposalPending {
			return utilAppError(constants.CodeRefundStateInvalid, "撤销售后失败：售后单 "+rf.RefundNo+" 当前状态 "+rf.Status+" 已完结，不能撤销", nil)
		}
		fromStatus := rf.Status
		now := time.Now()
		ok, err := s.refundRepo.TransitForUpdate(tx, rf.ID, []string{constants.RefundStatusPendingSeller, constants.RefundStatusProposalPending}, map[string]interface{}{
			"status": constants.RefundStatusCancelled, "closed_at": &now, "updated_at": now,
		})
		if err != nil {
			return err
		}
		if !ok {
			return utilAppError(constants.CodeRefundStateInvalid, "撤销售后失败：售后单 "+rf.RefundNo+" 已被并发处理，只能有一个结果", nil)
		}
		if err := s.refundRepo.AddNegotiationWithTx(tx, &model.RefundNegotiation{
			RefundID: rf.ID, OrderID: o.ID, ActorID: buyerID, ActorRole: constants.RefundActorBuyer,
			Action: constants.RefundActionCancel, Amount: 0, Remark: "买家撤销售后申请", CreatedAt: now,
		}); err != nil {
			return err
		}
		// 撤销后解除“售后中”，订单保持原状态不变（恢复原状态）。
		if err := s.orderRepo.SetActiveRefundForUpdate(tx, o.ID, nil); err != nil {
			return fmt.Errorf("resume order %d after refund cancel: %w", o.ID, err)
		}
		rf.Status = constants.RefundStatusCancelled
		refund = rf
		s.logger.Info(constants.LogRefundCancelled, "refund_id", rf.ID, "order_no", o.OrderNo, "buyer_id", buyerID, "order_status", o.Status, "from_status", fromStatus, "status", rf.Status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.refundRepo.GetByID(refund.ID)
}

// applyRefundResult 协商成功后的订单侧落库（与售后状态同一事务提交）：
// 退货退款 → 订单取消并重新上架商品；部分退款 → 订单维持当前状态继续履约；两者均解除“售后中”。
func (s *RefundService) applyRefundResult(tx *gorm.DB, o *model.Order, rf *model.Refund, finalAmount float64, now time.Time) error {
	if rf.Type == constants.RefundTypeReturn {
		if err := s.orderRepo.UpdateStatusForUpdate(tx, o.ID, constants.OrderStatusCancelled, map[string]interface{}{
			"cancelled_at": &now, "active_refund_id": nil,
		}); err != nil {
			return fmt.Errorf("cancel order %s after refund: %w", o.OrderNo, err)
		}
		if err := s.productRepo.UpdateStatusForUpdate(tx, o.ProductID, constants.ProductStatusOnSale); err != nil {
			return fmt.Errorf("relist product %d after refund: %w", o.ProductID, err)
		}
		return nil
	}
	if err := s.orderRepo.SetActiveRefundForUpdate(tx, o.ID, nil); err != nil {
		return fmt.Errorf("resume order %d after partial refund: %w", o.ID, err)
	}
	return nil
}

// GetDetail 售后详情：仅买卖双方可查看（订单/退款/协商历史一致回读）。
func (s *RefundService) GetDetail(viewerID, refundID uint) (*dto.RefundVO, error) {
	rf, err := s.refundRepo.GetByID(refundID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, utilAppError(constants.CodeRefundNotFound, "售后详情查询失败：售后单 id="+fmt.Sprint(refundID)+" 不存在", err)
		}
		return nil, fmt.Errorf("get refund detail %d: %w", refundID, err)
	}
	if rf.BuyerID != viewerID && rf.SellerID != viewerID {
		return nil, utilAppError(constants.CodeNotRefundParty, "售后详情查询失败：用户 id="+fmt.Sprint(viewerID)+" 非售后单 "+rf.RefundNo+" 的买卖双方", nil)
	}
	return toRefundVO(rf), nil
}

// GetByOrder 按订单查询售后（订单详情页回读售后状态与退款结果，复用 GetDetail 的鉴权）。
func (s *RefundService) GetByOrder(viewerID, orderID uint) (*dto.RefundVO, error) {
	rf, err := s.refundRepo.GetByOrderID(orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, utilAppError(constants.CodeRefundNotFound, "售后查询失败：订单 id="+fmt.Sprint(orderID)+" 暂无售后单", err)
		}
		return nil, fmt.Errorf("get refund by order %d: %w", orderID, err)
	}
	if rf.BuyerID != viewerID && rf.SellerID != viewerID {
		return nil, utilAppError(constants.CodeNotRefundParty, "售后查询失败：用户 id="+fmt.Sprint(viewerID)+" 非订单 id="+fmt.Sprint(orderID)+" 的买卖双方", nil)
	}
	return toRefundVO(rf), nil
}

// List 售后列表（buyer/seller 双视角，复用同一 service 方法）。
func (s *RefundService) List(viewerID uint, q dto.RefundQuery) (*dto.RefundListResponse, error) {
	page, pageSize := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	var list []model.Refund
	var total int64
	var err error
	if q.Role == "seller" {
		list, total, err = s.refundRepo.ListBySeller(viewerID, q.Status, page, pageSize)
	} else {
		list, total, err = s.refundRepo.ListByBuyer(viewerID, q.Status, page, pageSize)
	}
	if err != nil {
		return nil, fmt.Errorf("list refunds viewer=%d role=%s: %w", viewerID, q.Role, err)
	}
	res := &dto.RefundListResponse{List: make([]dto.RefundVO, 0, len(list)), Total: total, Page: page, Size: pageSize}
	for i := range list {
		res.List = append(res.List, *toRefundVO(&list[i]))
	}
	return res, nil
}

// toRefundVO 组装售后视图对象；预加载了 Order 时附带订单快照（不含循环嵌套）。
func toRefundVO(rf *model.Refund) *dto.RefundVO {
	vo := dto.FromRefund(rf)
	if rf.Order != nil {
		orderVO := dto.FromOrder(rf.Order)
		vo.Order = &orderVO
	}
	return &vo
}

// genRefundNo 生成售后单号：R + yyyyMMddHHmmss + 6 位随机数。
func genRefundNo() string {
	return fmt.Sprintf("R%s%06d", time.Now().Format("20060102150405"), rand.Intn(1000000))
}

// validateApplyAmount 申请/同意环节统一的金额规则，退货退款与部分退款在此明确区分：
//   - 退货退款（return_refund）：必须按订单实付金额全额申请，少退会导致订单取消/重新上架却退款不足；
//   - 部分退款（partial_refund）：金额必须大于 0 且不超过实付。
func validateApplyAmount(refundType string, amount, paidAmount float64, orderNo string) error {
	if refundType == constants.RefundTypeReturn {
		if amount > paidAmount+amountEpsilon || math.Abs(amount-paidAmount) > amountEpsilon {
			return utilAppError(constants.CodeRefundAmountExceed, fmt.Sprintf(
				"退货退款必须按订单 %s 实付金额 %.2f 全额处理，当前金额 %.2f 不合法", orderNo, paidAmount, amount), nil)
		}
		return nil
	}
	if amount <= 0 {
		return utilAppError(constants.CodeRefundAmountExceed, fmt.Sprintf("部分退款金额必须大于 0，订单 %s 当前金额 %.2f", orderNo, amount), nil)
	}
	if amount > paidAmount+amountEpsilon {
		return utilAppError(constants.CodeRefundAmountExceed, fmt.Sprintf("退款金额 %.2f 超过订单 %s 实付金额 %.2f", amount, orderNo, paidAmount), nil)
	}
	return nil
}

// refundAmountValue 安全读取协商成功后的退款金额，记录未加载时记 0。
func refundAmountValue(rf *model.Refund) float64 {
	if rf != nil && rf.FinalAmount != nil {
		return *rf.FinalAmount
	}
	return 0
}
