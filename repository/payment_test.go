package repository

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestSettlePaymentOrderKeepsBalanceSeparateFromCredits(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	user := model.User{ID: "payment-user", Username: "payment-user", BalanceCents: 125, Credits: 777, Status: model.UserStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	order := model.PaymentOrder{ID: "payment-order", OrderNo: "pay-order", UserID: user.ID, AmountCents: 1000, Status: model.PaymentOrderStatusPending, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
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
	if user.BalanceCents != 1125 || user.Credits != 777 {
		t.Fatalf("unexpected balances: balanceCents=%d credits=%d", user.BalanceCents, user.Credits)
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
