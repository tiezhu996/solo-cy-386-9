package repository

import (
	"testing"

	"github.com/marketpal/marketpal/internal/model"
)

func TestRefundRepository(t *testing.T) {
	db := newTestDB(t)
	orderRepo := NewOrderRepository(db)
	repo := NewRefundRepository(db)

	o := &model.Order{OrderNo: "R-ORD-1", BuyerID: 10, SellerID: 20, ProductID: 1, AddressID: 1, Quantity: 1, TotalPrice: 200, Status: "pending_shipment"}
	if err := orderRepo.Create(o); err != nil {
		t.Fatalf("create order: %v", err)
	}

	rf := &model.Refund{RefundNo: "RR-1", OrderID: o.ID, BuyerID: 10, SellerID: 20, Type: "partial_refund", Reason: "有划痕", ApplyAmount: 50, Status: "pending_seller"}
	t.Run("create_and_get", func(t *testing.T) {
		if err := repo.CreateWithTx(db, rf); err != nil {
			t.Fatalf("CreateWithTx: %v", err)
		}
		got, err := repo.GetByID(rf.ID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if got.RefundNo != "RR-1" || got.Order.OrderNo != "R-ORD-1" {
			t.Fatalf("unexpected refund: %+v", got)
		}
	})

	t.Run("duplicate_order_rejected", func(t *testing.T) {
		dup := &model.Refund{RefundNo: "RR-2", OrderID: o.ID, BuyerID: 10, SellerID: 20, Type: "return_refund", Reason: "x", ApplyAmount: 200, Status: "pending_seller"}
		if err := repo.CreateWithTx(db, dup); err != ErrRefundExists {
			t.Fatalf("expected ErrRefundExists, got %v", err)
		}
	})

	t.Run("append_only_negotiation", func(t *testing.T) {
		if err := repo.AddNegotiationWithTx(db, &model.RefundNegotiation{RefundID: rf.ID, OrderID: o.ID, ActorID: 10, ActorRole: "buyer", Action: "apply", Amount: 50, Remark: "有划痕"}); err != nil {
			t.Fatalf("AddNegotiationWithTx: %v", err)
		}
		if err := repo.AddNegotiationWithTx(db, &model.RefundNegotiation{RefundID: rf.ID, OrderID: o.ID, ActorID: 20, ActorRole: "seller", Action: "agree", Amount: 50}); err != nil {
			t.Fatalf("AddNegotiationWithTx 2: %v", err)
		}
		full, _ := repo.GetByID(rf.ID)
		if len(full.Negotiations) != 2 || full.Negotiations[0].Action != "apply" || full.Negotiations[1].Action != "agree" {
			t.Fatalf("negotiation history mismatch: %+v", full.Negotiations)
		}
	})

	t.Run("conditional_transit", func(t *testing.T) {
		// 单据 A：前置状态匹配 → 条件流转成功。
		oa := &model.Order{OrderNo: "R-ORD-A", BuyerID: 10, SellerID: 20, ProductID: 1, AddressID: 1, TotalPrice: 100, Status: "pending_shipment"}
		if err := orderRepo.Create(oa); err != nil {
			t.Fatalf("create order A: %v", err)
		}
		ra := &model.Refund{RefundNo: "RR-A", OrderID: oa.ID, BuyerID: 10, SellerID: 20, Type: "partial_refund", Reason: "a", ApplyAmount: 10, Status: "pending_seller"}
		if err := repo.CreateWithTx(db, ra); err != nil {
			t.Fatalf("create refund A: %v", err)
		}
		ok, err := repo.TransitForUpdate(db, ra.ID, []string{"pending_seller"}, map[string]interface{}{"status": "agreed", "final_amount": 10})
		if err != nil || !ok {
			t.Fatalf("expected successful conditional transit, ok=%v err=%v", ok, err)
		}
		// 第二次条件更新必须失败（状态已是 agreed，并发只能有一个结果）。
		ok, err = repo.TransitForUpdate(db, ra.ID, []string{"pending_seller"}, map[string]interface{}{"status": "cancelled"})
		if err != nil || ok {
			t.Fatalf("second transit must not rewrite, ok=%v err=%v", ok, err)
		}

		// 单据 B：当前状态与前置不符（proposal_pending ≠ pending_seller）→ 流转失败。
		ob := &model.Order{OrderNo: "R-ORD-B", BuyerID: 10, SellerID: 20, ProductID: 1, AddressID: 1, TotalPrice: 100, Status: "pending_shipment"}
		_ = orderRepo.Create(ob)
		rb := &model.Refund{RefundNo: "RR-B", OrderID: ob.ID, BuyerID: 10, SellerID: 20, Type: "partial_refund", Reason: "b", ApplyAmount: 20, Status: "proposal_pending"}
		_ = repo.CreateWithTx(db, rb)
		ok, err = repo.TransitForUpdate(db, rb.ID, []string{"pending_seller"}, map[string]interface{}{"status": "rejected"})
		if err != nil {
			t.Fatalf("TransitForUpdate B: %v", err)
		}
		if ok {
			t.Fatal("transit should fail when current status differs")
		}
	})
}
