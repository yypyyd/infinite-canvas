package repository

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestSettlePaymentOrderKeepsBalanceSeparateFromCredits(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "payment-user", Username: "payment-user", BalanceCents: 125, Credits: 777, Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	order := model.PaymentOrder{ID: "payment-order", OrderNo: "pay-order", UserID: user.ID, AmountCents: 9800, BalanceCents: 10000, Status: model.PaymentOrderStatusPending, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&order).Error; err != nil {
		t.Fatal(err)
	}

	settledOrder, settled, err := SettlePaymentOrder(order.OrderNo, "trade-payment", workspaceTestFuture, model.BalanceLog{ID: "balance-log-1", Type: model.BalanceLogTypePaymentRecharge})
	if err != nil || !settled || settledOrder.Status != model.PaymentOrderStatusPaid {
		t.Fatalf("settle payment: order=%#v settled=%v err=%v", settledOrder, settled, err)
	}
	if _, settled, err = SettlePaymentOrder(order.OrderNo, "trade-payment", workspaceTestFuture, model.BalanceLog{ID: "balance-log-2", Type: model.BalanceLogTypePaymentRecharge}); err != nil || settled {
		t.Fatalf("repeat settlement: settled=%v err=%v", settled, err)
	}

	if err := testDB.First(&user, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 10125 || user.Credits != 777 {
		t.Fatalf("unexpected balances: balanceCents=%d credits=%d", user.BalanceCents, user.Credits)
	}
	var balanceLog model.BalanceLog
	if err := testDB.Where("user_id = ?", user.ID).First(&balanceLog).Error; err != nil {
		t.Fatal(err)
	}
	if balanceLog.AmountCents != 10000 || balanceLog.BalanceCents != 10125 {
		t.Fatalf("unexpected balance log: %#v", balanceLog)
	}
	var balanceLogs, creditLogs int64
	if err := testDB.Model(&model.BalanceLog{}).Where("user_id = ?", user.ID).Count(&balanceLogs).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Model(&model.CreditLog{}).Where("user_id = ?", user.ID).Count(&creditLogs).Error; err != nil {
		t.Fatal(err)
	}
	if balanceLogs != 1 || creditLogs != 0 {
		t.Fatalf("unexpected ledger counts: balance=%d credit=%d", balanceLogs, creditLogs)
	}
}

func TestSettlePaymentOrderCreditsInviterOnce(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	inviter := model.User{ID: "referral-inviter", Username: "referral-inviter", BalanceCents: 100, Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	invitee := model.User{ID: "referral-invitee", Username: "referral-invitee", InviterID: inviter.ID, Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	order := model.PaymentOrder{ID: "referral-payment-order", OrderNo: "referral-pay-order", UserID: invitee.ID, AmountCents: 9800, BalanceCents: 10000, Status: model.PaymentOrderStatusPending, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	for _, item := range []any{&inviter, &invitee, &order} {
		if err := testDB.Create(item).Error; err != nil {
			t.Fatal(err)
		}
	}

	commission := model.ReferralCommission{ID: "referral-commission", InviterID: inviter.ID, InviteeID: invitee.ID, BaseAmountCents: order.AmountCents, RatePercent: 10, CommissionCents: 980}
	if _, settled, err := SettlePaymentOrder(order.OrderNo, "trade-referral", workspaceTestFuture, model.BalanceLog{ID: "referral-recharge-log", Type: model.BalanceLogTypePaymentRecharge}, commission); err != nil || !settled {
		t.Fatalf("settle referral payment: settled=%v err=%v", settled, err)
	}
	if _, settled, err := SettlePaymentOrder(order.OrderNo, "trade-referral", workspaceTestFuture, model.BalanceLog{ID: "referral-recharge-repeat", Type: model.BalanceLogTypePaymentRecharge}, commission); err != nil || settled {
		t.Fatalf("repeat referral settlement: settled=%v err=%v", settled, err)
	}
	secondOrder := model.PaymentOrder{ID: "referral-payment-order-2", OrderNo: "referral-pay-order-2", UserID: invitee.ID, AmountCents: 20000, BalanceCents: 20000, Status: model.PaymentOrderStatusPending, CreatedAt: workspaceTestFuture, UpdatedAt: workspaceTestFuture}
	if err := testDB.Create(&secondOrder).Error; err != nil {
		t.Fatal(err)
	}
	if _, settled, err := SettlePaymentOrder(secondOrder.OrderNo, "trade-referral-2", workspaceTestFuture, model.BalanceLog{ID: "referral-recharge-log-2", Type: model.BalanceLogTypePaymentRecharge}, commission); err != nil || !settled {
		t.Fatalf("settle second payment: settled=%v err=%v", settled, err)
	}

	if err := testDB.First(&inviter, "id = ?", inviter.ID).Error; err != nil {
		t.Fatal(err)
	}
	if inviter.BalanceCents != 1080 {
		t.Fatalf("unexpected inviter balance: %d", inviter.BalanceCents)
	}
	var count int64
	if err := testDB.Model(&model.ReferralCommission{}).Where("payment_order_id = ?", order.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected referral commission count: %d", count)
	}
	if err := testDB.Model(&model.BalanceLog{}).Where("user_id = ? AND type = ?", inviter.ID, model.BalanceLogTypeReferralCommission).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("unexpected referral balance log count: %d", count)
	}
}

func TestExchangeUserBalanceForCreditsUpdatesBothLedgersAtomically(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "exchange-user", Username: "exchange-user", OrganizationID: "exchange-org", BalanceCents: 2500, Credits: 50, Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&user).Error; err != nil {
		t.Fatal(err)
	}

	exchangeID := "exchange-1"
	updated, err := ExchangeUserBalanceForCredits(user.ID, 1000, 2000, workspaceTestFuture,
		model.BalanceLog{ID: "exchange-balance-log", OrganizationID: user.OrganizationID, Type: model.BalanceLogTypeCreditsExchange, RelatedID: exchangeID},
		model.CreditLog{ID: "exchange-credit-log", OrganizationID: user.OrganizationID, Type: model.CreditLogTypeBalanceExchange, RelatedID: exchangeID},
	)
	if err != nil {
		t.Fatalf("exchange balance: %v", err)
	}
	if updated.BalanceCents != 1500 || updated.Credits != 2050 {
		t.Fatalf("unexpected balances after exchange: balanceCents=%d credits=%d", updated.BalanceCents, updated.Credits)
	}

	var balanceLog model.BalanceLog
	if err := testDB.Where("user_id = ?", user.ID).First(&balanceLog).Error; err != nil {
		t.Fatal(err)
	}
	if balanceLog.Type != model.BalanceLogTypeCreditsExchange || balanceLog.AmountCents != -1000 || balanceLog.BalanceCents != 1500 || balanceLog.RelatedID != exchangeID {
		t.Fatalf("unexpected balance log: %#v", balanceLog)
	}
	var creditLog model.CreditLog
	if err := testDB.Where("user_id = ?", user.ID).First(&creditLog).Error; err != nil {
		t.Fatal(err)
	}
	if creditLog.Type != model.CreditLogTypeBalanceExchange || creditLog.CreditSource != model.CreditSourcePersonal || creditLog.Amount != 2000 || creditLog.Balance != 2050 || creditLog.RelatedID != exchangeID {
		t.Fatalf("unexpected credit log: %#v", creditLog)
	}

	if _, err := ExchangeUserBalanceForCredits(user.ID, 2000, 4000, workspaceTestFuture,
		model.BalanceLog{ID: "failed-balance-log", Type: model.BalanceLogTypeCreditsExchange, RelatedID: "exchange-2"},
		model.CreditLog{ID: "failed-credit-log", Type: model.CreditLogTypeBalanceExchange, RelatedID: "exchange-2"},
	); !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("insufficient balance exchange error = %v", err)
	}
	if err := testDB.First(&user, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if user.BalanceCents != 1500 || user.Credits != 2050 {
		t.Fatalf("failed exchange changed balances: balanceCents=%d credits=%d", user.BalanceCents, user.Credits)
	}
	var balanceLogs, creditLogs int64
	if err := testDB.Model(&model.BalanceLog{}).Where("user_id = ?", user.ID).Count(&balanceLogs).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Model(&model.CreditLog{}).Where("user_id = ?", user.ID).Count(&creditLogs).Error; err != nil {
		t.Fatal(err)
	}
	if balanceLogs != 1 || creditLogs != 1 {
		t.Fatalf("failed exchange changed ledger counts: balance=%d credit=%d", balanceLogs, creditLogs)
	}
}
