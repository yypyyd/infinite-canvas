package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
	"gorm.io/gorm"
)

func lateRechargeCount(t *testing.T, task model.GenerationTask) int64 {
	t.Helper()
	db, _ := repository.DB()
	var count int64
	db.Model(&model.CreditLog{}).Where("related_id = ? AND type = ?", task.ID, model.CreditLogTypeAILateRecharge).Count(&count)
	return count
}

func completedPool() *fakePoolTasks {
	return &fakePoolTasks{status: map[string]any{"status": "completed", "created": 1, "data": []any{map[string]any{"b64_json": "eA=="}}}}
}

func TestLateDeliveryAfterRefundChargesOnce(t *testing.T) {
	usePoolTasks(t, completedPool())
	task, user := poolTask(t, "late-recharge", 3*time.Minute)
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "handler gave up"); err != nil {
		t.Fatal(err)
	}
	if userCredits(t, user) != 100 || refundCount(t, user) != 1 {
		t.Fatal("precondition: refunded once")
	}
	failed := reloadTask(t, task)
	if !recoverLatePoolImage(context.Background(), failed) {
		t.Fatal("late result must be delivered")
	}
	// Duplicate recovery (another pass / restart with stale data) changes nothing.
	if recoverLatePoolImage(context.Background(), failed) {
		t.Fatal("second recovery must not recharge")
	}
	checkLateDeliveredPoolImages(context.Background(), time.Now().UTC())
	saved := reloadTask(t, task)
	if saved.Status != model.GenerationTaskStatusSuccess || len(saved.StorageKeys) != 1 {
		t.Fatalf("status=%s keys=%v", saved.Status, saved.StorageKeys)
	}
	if userCredits(t, user) != 97 || lateRechargeCount(t, task) != 1 {
		t.Fatalf("net one charge expected: credits=%d recharges=%d", userCredits(t, user), lateRechargeCount(t, task))
	}
}

func TestConcurrentLateRecoveryRechargesOnce(t *testing.T) {
	usePoolTasks(t, completedPool())
	task, user := poolTask(t, "late-concurrent", 3*time.Minute)
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "x"); err != nil {
		t.Fatal(err)
	}
	failed := reloadTask(t, task)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); recoverLatePoolImage(context.Background(), failed) }()
	}
	wg.Wait()
	if userCredits(t, user) != 97 || lateRechargeCount(t, task) != 1 {
		t.Fatalf("credits=%d recharges=%d", userCredits(t, user), lateRechargeCount(t, task))
	}
}

func TestLateDeliveryWithInsufficientBalanceGoesNegative(t *testing.T) {
	usePoolTasks(t, completedPool())
	task, user := poolTask(t, "late-debt", 3*time.Minute)
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "x"); err != nil {
		t.Fatal(err)
	}
	db, _ := repository.DB()
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("credits", 1).Error; err != nil {
		t.Fatal(err)
	}
	if !recoverLatePoolImage(context.Background(), reloadTask(t, task)) {
		t.Fatal("delivery must not be blocked by balance")
	}
	if userCredits(t, user) != -2 {
		t.Fatalf("balance=%d want -2 (debt)", userCredits(t, user))
	}
	var log model.CreditLog
	if err := db.Where("related_id = ? AND type = ?", task.ID, model.CreditLogTypeAILateRecharge).First(&log).Error; err != nil || log.Amount != -3 || log.Balance != -2 {
		t.Fatalf("ledger: %+v %v", log, err)
	}
	// A later top-up repays the debt first.
	if err := db.Model(&model.User{}).Where("id = ?", user.ID).Update("credits", gorm.Expr("credits + ?", 10)).Error; err != nil {
		t.Fatal(err)
	}
	if userCredits(t, user) != 8 {
		t.Fatalf("top-up should net the debt: %d", userCredits(t, user))
	}
}

func TestLateDeliveryIgnoresPoolFailures(t *testing.T) {
	usePoolTasks(t, &fakePoolTasks{status: map[string]any{"status": "failed"}})
	task, user := poolTask(t, "late-none", 3*time.Minute)
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "x"); err != nil {
		t.Fatal(err)
	}
	checkLateDeliveredPoolImages(context.Background(), time.Now().UTC())
	if reloadTask(t, task).Status != model.GenerationTaskStatusFailed || userCredits(t, user) != 100 {
		t.Fatal("no delivery, no charge")
	}
}
