package service

import (
	"math"
	"testing"

	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/dto"
	"github.com/marketpal/marketpal/internal/model"
	"github.com/marketpal/marketpal/internal/repository"
)

// 顺序回归：每个用例独立磁盘库（或设置 DSN 后的独立 Postgres schema 数据），重复执行不串数据。

// 退货退款必须按实付全额：申请全额 → 卖家同意 → 订单取消、商品重新上架、退款结果=实付。
func TestPersistReturnRefundFullAmount(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("return", 100, constants.OrderStatusPendingShipment)

	req := dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypeReturn,
		Reason: "商品破损，申请退货退款", Amount: 100, Evidence: []string{"http://x/a.jpg"},
	}
	rf, err := env.svc(env.db).Apply(100, req)
	if err != nil {
		t.Fatalf("退货退款全额申请失败: %v", err)
	}
	if _, err := env.svc(env.db).Agree(200, rf.ID); err != nil {
		t.Fatalf("卖家同意退货退款失败: %v", err)
	}

	snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
	env.verifyConsistency(snap, expectState{
		refundStatus:    constants.RefundStatusAgreed,
		finalAmount:     ptrFloat(100),
		orderStatus:     constants.OrderStatusCancelled,
		activeRefundNil: true,
		productStatus:   constants.ProductStatusOnSale,
		actions:         []string{constants.RefundActionApply, constants.RefundActionAgree},
	})
}

// 退货退款低于实付必须被拒绝（修复点回归）：不会出现小额退款却取消订单。
func TestPersistReturnRefundRejectsPartialAmount(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("returnlow", 100, constants.OrderStatusPendingShipment)

	req := dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypeReturn,
		Reason: "想只退 1 元", Amount: 1,
	}
	if _, err := env.svc(env.db).Apply(100, req); err == nil {
		t.Fatal("退货退款按 1 元申请应当失败，但被接受")
	}

	// 申请未成立：订单与商品保持原状，售后表无记录。
	var o = fx.Order
	if err := env.db.First(o, fx.Order.ID).Error; err != nil {
		t.Fatalf("回读订单失败: %v", err)
	}
	if o.Status != constants.OrderStatusPendingShipment || o.ActiveRefundID != nil {
		t.Fatalf("失败申请不应改动订单，实际 status=%s active=%v", o.Status, o.ActiveRefundID)
	}
	// 失败申请不应产生任何售后记录。
	var count int64
	env.db.Model(&model.Refund{}).Where("order_id = ?", fx.Order.ID).Count(&count)
	if count != 0 {
		t.Fatalf("失败的退货退款申请不应落库售后单，实际存在 %d 条", count)
	}
}

// 部分退款走方案协商：申请 80 → 卖家提方案 50 → 买家接受 → 订单维持可履约、商品保持已售、退款结果=50。
func TestPersistPartialRefundProposal(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("partial", 100, constants.OrderStatusPendingShipment)

	svc := env.svc(env.db)
	rf, err := svc.Apply(100, dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
		Reason: "部分瑕疵，协商少退", Amount: 80,
	})
	if err != nil {
		t.Fatalf("部分退款申请失败: %v", err)
	}
	if _, err := svc.Propose(200, rf.ID, dto.RefundProposeRequest{Amount: 50, Reason: "只同意退 50 元"}); err != nil {
		t.Fatalf("卖家提出方案失败: %v", err)
	}
	if _, err := svc.Accept(100, rf.ID); err != nil {
		t.Fatalf("买家接受方案失败: %v", err)
	}

	snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
	env.verifyConsistency(snap, expectState{
		refundStatus:    constants.RefundStatusAgreed,
		finalAmount:     ptrFloat(50),
		orderStatus:     constants.OrderStatusPendingShipment, // 部分退款不关闭订单
		activeRefundNil: true,
		productStatus:   constants.ProductStatusSold,
		actions: []string{
			constants.RefundActionApply, constants.RefundActionPropose, constants.RefundActionAccept,
		},
	})
}

// 终态售后回读：拒绝后订单恢复、active 为空，但最近一笔售后仍可回读，且不能再次申请。
func TestPersistClosedRefundReadbackAndBlockReapply(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("reject", 100, constants.OrderStatusPendingShipment)

	svc := env.svc(env.db)
	rf, err := svc.Apply(100, dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
		Reason: "协商退款", Amount: 30,
	})
	if err != nil {
		t.Fatalf("申请售后失败: %v", err)
	}
	if _, err := svc.Reject(200, rf.ID, dto.RefundRejectRequest{Reason: "凭证不足"}); err != nil {
		t.Fatalf("卖家拒绝失败: %v", err)
	}

	// 通过独立连接 + 仓储完整回读（GetByID 组装 ActiveRefund/LastRefund）。
	handle := env.newHandle()
	orderRepo := repository.NewOrderRepository(handle)
	got, err := orderRepo.GetByID(fx.Order.ID)
	if err != nil {
		t.Fatalf("独立连接回读订单失败: %v", err)
	}
	if got.Status != constants.OrderStatusPendingShipment {
		t.Fatalf("拒绝后订单应恢复待发货，实际 %s", got.Status)
	}
	if got.ActiveRefundID != nil || got.ActiveRefund != nil {
		t.Fatalf("拒绝后不应有进行中售后，实际 active=%v", got.ActiveRefundID)
	}
	if got.LastRefund == nil {
		t.Fatal("拒绝后订单仍应能回读到最近一笔售后（last_refund），实际为空")
	}
	if got.LastRefund.Status != constants.RefundStatusRejected {
		t.Fatalf("最近一笔售后状态应为 rejected，实际 %s", got.LastRefund.Status)
	}

	// 不能再次申请（即使是不同内容也拒绝）。
	_, err = env.svc(env.newHandle()).Apply(100, dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
		Reason: "再试一次", Amount: 10,
	})
	if asAppError(t, err).Code != constants.CodeRefundExists {
		t.Fatalf("完结售后后重复申请应返回 CodeRefundExists，实际 %v", err)
	}

	snap := env.readSnapshot(handle, fx.Order.ID)
	env.verifyConsistency(snap, expectState{
		refundStatus:    constants.RefundStatusRejected,
		finalAmount:     nil,
		orderStatus:     constants.OrderStatusPendingShipment,
		activeRefundNil: true,
		productStatus:   constants.ProductStatusSold,
		actions:         []string{constants.RefundActionApply, constants.RefundActionReject},
	})
}

// 重复申请幂等：完全相同的申请返回同一售后单、不新增记录；处理中改内容判冲突不改写。
func TestPersistDuplicateApplyIdempotent(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("idemp", 100, constants.OrderStatusPendingShipment)

	svc := env.svc(env.db)
	req := dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
		Reason: "相同原因", Amount: 30, Evidence: []string{"http://x/a.jpg"},
	}
	first, err := svc.Apply(100, req)
	if err != nil {
		t.Fatalf("首次申请失败: %v", err)
	}
	// 通过独立连接提交完全相同的请求（模拟用户重复点击/网络重试）。
	second, err := env.svc(env.newHandle()).Apply(100, req)
	if err != nil {
		t.Fatalf("相同内容重复申请应幂等成功，实际报错 %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("重复申请必须返回同一售后单，first=%d second=%d", first.ID, second.ID)
	}
	// 处理中提交不同金额：必须冲突，不得改写原记录。
	diff := req
	diff.Amount = 40
	if _, err := env.svc(env.newHandle()).Apply(100, diff); asAppError(t, err).Code != constants.CodeRefundExists {
		t.Fatalf("不同内容重复申请应返回 CodeRefundExists，实际 %v", err)
	}

	snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
	if len(snap.negotiations) != 1 {
		t.Fatalf("重复申请不能追加协商历史，期望仅 1 条 apply，实际 %d 条", len(snap.negotiations))
	}
	if math.Abs(snap.refund.ApplyAmount-30) > 1e-9 || snap.refund.Reason != "相同原因" {
		t.Fatalf("原售后记录不得被改写，实际 amount=%.2f reason=%s", snap.refund.ApplyAmount, snap.refund.Reason)
	}
	env.verifyConsistency(snap, expectState{
		refundStatus:    constants.RefundStatusPendingSeller,
		finalAmount:     nil,
		orderStatus:     constants.OrderStatusPendingShipment,
		activeRefundNil: false,
		productStatus:   constants.ProductStatusSold,
		actions:         []string{constants.RefundActionApply},
	})
}
