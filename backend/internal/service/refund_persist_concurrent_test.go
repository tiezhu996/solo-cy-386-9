package service

import (
	"sync"
	"testing"

	"github.com/marketpal/marketpal/internal/constants"
	"github.com/marketpal/marketpal/internal/dto"
)

// 真实并发回归：
//   - 每个竞争者使用 newHandle() 打开的独立连接池（独立 *gorm.DB/底层连接），不是内存替身、mock 或单连接串行化；
//   - 通过屏障让两个事务在同一时刻发起，真实竞争订单行/售后单行锁与条件状态更新；
//   - 失败信息逐层指出订单、售后单、协商历史、商品哪一层不一致。

// raceResult 记录一个并发动作的结果。
type raceResult struct {
	name string
	err  error
}

// runConcurrent 启动两个动作并在屏障释放后同时执行，返回各自结果。
func runConcurrent(a, b func() error) (raceResult, raceResult) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	ra, rb := raceResult{name: "A"}, raceResult{name: "B"}
	run := func(res *raceResult, fn func() error) {
		defer wg.Done()
		<-start // 等屏障，保证两个事务尽量在同一时刻发起
		res.err = fn()
	}
	wg.Add(2)
	go run(&ra, a)
	go run(&rb, b)
	close(start)
	wg.Wait()
	return ra, rb
}

// 卖家同意 vs 买家撤销：同一售后单上两个终态动作并发，只能有一个成功、只能落一个结果。
func TestPersistConcurrentAgreeVsCancel(t *testing.T) {
	env := openPersistEnv(t)

	const rounds = 4
	for i := 0; i < rounds; i++ {
		fx := env.seed("race-ac", 100, constants.OrderStatusPendingShipment)
		// 申请在主连接完成；竞争双方各自使用独立连接。
		rf, err := env.svc(env.db).Apply(100, dto.RefundApplyRequest{
			OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
			Reason: "并发竞争用售后", Amount: 30,
		})
		if err != nil {
			t.Fatalf("第 %d 轮申请失败: %v", i, err)
		}

		// 两个全新独立连接池，各自构造独立 service/repository。
		sellerSvc := env.svc(env.newHandle())
		buyerSvc := env.svc(env.newHandle())

		res1, res2 := runConcurrent(
			func() error { _, e := sellerSvc.Agree(200, rf.ID); return e },
			func() error { _, e := buyerSvc.Cancel(100, rf.ID); return e },
		)

		successes, failures := 0, 0
		for _, r := range []raceResult{res1, res2} {
			if r.err == nil {
				successes++
			} else {
				failures++
			}
		}
		if successes != 1 || failures != 1 {
			t.Fatalf("第 %d 轮并发必须恰有一个结果，实际成功=%d 失败=%d（agreeErr=%v cancelErr=%v）",
				i, successes, failures, res1.err, res2.err)
		}

		// 败方必须是“售后状态不允许该操作”（条件更新未命中），而不是其它错误。
		var loserErr error
		if res1.err != nil {
			loserErr = res1.err
		} else {
			loserErr = res2.err
		}
		if asAppError(t, loserErr).Code != constants.CodeRefundStateInvalid {
			t.Fatalf("第 %d 轮竞争败方应返回 CodeRefundStateInvalid，实际 %v", i, loserErr)
		}

		// 最终状态按胜出方严格校验四层一致性。
		snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
		if snap.refund.Status == constants.RefundStatusAgreed {
			env.verifyConsistency(snap, expectState{
				refundStatus:    constants.RefundStatusAgreed,
				finalAmount:     ptrFloat(30),
				orderStatus:     constants.OrderStatusPendingShipment, // 部分退款成功后订单继续履约
				activeRefundNil: true,
				productStatus:   constants.ProductStatusSold,
				actions:         []string{constants.RefundActionApply, constants.RefundActionAgree},
			})
		} else {
			env.verifyConsistency(snap, expectState{
				refundStatus:    constants.RefundStatusCancelled,
				finalAmount:     nil,
				orderStatus:     constants.OrderStatusPendingShipment, // 撤销后恢复原状态
				activeRefundNil: true,
				productStatus:   constants.ProductStatusSold,
				actions:         []string{constants.RefundActionApply, constants.RefundActionCancel},
			})
		}
	}
}

// 卖家重复同意：两个独立连接并发调用 Agree，只能有一次生效，退款只记录一次。
func TestPersistConcurrentDuplicateAgree(t *testing.T) {
	env := openPersistEnv(t)

	const rounds = 4
	for i := 0; i < rounds; i++ {
		fx := env.seed("race-da", 100, constants.OrderStatusPendingShipment)
		rf, err := env.svc(env.db).Apply(100, dto.RefundApplyRequest{
			OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
			Reason: "重复同意竞争", Amount: 30,
		})
		if err != nil {
			t.Fatalf("第 %d 轮申请失败: %v", i, err)
		}

		svc1 := env.svc(env.newHandle())
		svc2 := env.svc(env.newHandle())
		res1, res2 := runConcurrent(
			func() error { _, e := svc1.Agree(200, rf.ID); return e },
			func() error { _, e := svc2.Agree(200, rf.ID); return e },
		)
		if (res1.err == nil) == (res2.err == nil) {
			t.Fatalf("第 %d 轮重复同意必须一成一败，实际 err1=%v err2=%v", i, res1.err, res2.err)
		}

		snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
		env.verifyConsistency(snap, expectState{
			refundStatus:    constants.RefundStatusAgreed,
			finalAmount:     ptrFloat(30), // 重复提交不能改写/重复记录退款
			orderStatus:     constants.OrderStatusPendingShipment,
			activeRefundNil: true,
			productStatus:   constants.ProductStatusSold,
			actions:         []string{constants.RefundActionApply, constants.RefundActionAgree},
		})

		// 退款结果只能有一条 agree 协商记录（防止重复落库）。
		agreeCount := 0
		for _, n := range snap.negotiations {
			if n.Action == constants.RefundActionAgree {
				agreeCount++
			}
		}
		if agreeCount != 1 {
			t.Fatalf("第 %d 轮 agree 协商记录必须恰好 1 条，实际 %d 条", i, agreeCount)
		}
	}
}

// 终态后并发再来的任何动作都不得改写结果：先撤销，再并发同意/接受，全部失败且状态不变。
func TestPersistConcurrentActionsAfterClosed(t *testing.T) {
	env := openPersistEnv(t)
	fx := env.seed("race-closed", 100, constants.OrderStatusPendingShipment)
	svc := env.svc(env.db)
	rf, err := svc.Apply(100, dto.RefundApplyRequest{
		OrderID: fx.Order.ID, Type: constants.RefundTypePartial,
		Reason: "先撤销", Amount: 30,
	})
	if err != nil {
		t.Fatalf("申请失败: %v", err)
	}
	if _, err := svc.Cancel(100, rf.ID); err != nil {
		t.Fatalf("撤销失败: %v", err)
	}

	sellerSvc := env.svc(env.newHandle())
	buyerSvc := env.svc(env.newHandle())
	res1, res2 := runConcurrent(
		func() error { _, e := sellerSvc.Agree(200, rf.ID); return e },
		func() error { _, e := buyerSvc.Cancel(100, rf.ID); return e },
	)
	if res1.err == nil || res2.err == nil {
		t.Fatalf("终态后并发动作必须全部失败，实际 agreeErr=%v cancelErr=%v", res1.err, res2.err)
	}

	snap := env.readSnapshot(env.newHandle(), fx.Order.ID)
	env.verifyConsistency(snap, expectState{
		refundStatus:    constants.RefundStatusCancelled,
		finalAmount:     nil,
		orderStatus:     constants.OrderStatusPendingShipment,
		activeRefundNil: true,
		productStatus:   constants.ProductStatusSold,
		actions:         []string{constants.RefundActionApply, constants.RefundActionCancel},
	})
}
