package service

import (
	"context"
	"errors"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	// poolLateDeliveryWindow covers the pool's own recovery window (up to 20
	// minutes after acceptance) with margin.
	poolLateDeliveryWindow   = 2 * time.Hour
	poolLateDeliveryInterval = 2 * time.Minute
	poolLateDeliveryBatch    = 20
)

// checkLateDeliveredPoolImages looks at recently failed (already refunded)
// pool image tasks. If the pool finished the image after 幻图 gave up, the
// result is delivered and the original credits are charged again (补扣费).
func checkLateDeliveredPoolImages(ctx context.Context, now time.Time) {
	if currentPoolTasks() == nil {
		return
	}
	stamp := now.Format(timestampLayout)
	tasks, err := repository.ListLateDeliveryCandidates(BuiltinPoolChannelName, "image", now.Add(-poolLateDeliveryWindow).Format(timestampLayout), stamp, poolLateDeliveryBatch)
	if err != nil {
		logWorkerError("generation_task", "late_delivery_list_failed", err)
		return
	}
	for _, candidate := range tasks {
		token := newID("late")
		task, claimed, err := repository.ClaimFailedGenerationTaskCheck(candidate.ID, token, stamp, now.Add(10*time.Minute).Format(timestampLayout))
		if err != nil || !claimed {
			continue
		}
		recoverLatePoolImage(ctx, task)
		// Still failed (nothing delivered): check again later. A delivered task
		// is success now and is never listed again.
		_ = repository.ReleaseGenerationTaskReconcile(task.ID, token, time.Now().UTC().Add(poolLateDeliveryInterval).Format(timestampLayout))
	}
}

// recoverLatePoolImage reports whether a late result was delivered and charged.
func recoverLatePoolImage(ctx context.Context, task model.GenerationTask) bool {
	backend := currentPoolTasks()
	if backend == nil || task.Status != model.GenerationTaskStatusFailed || task.Modality != "image" || !IsBuiltinPoolTask(task) {
		return false
	}
	status, err := backend.ImageTask(ctx, task.RequestID)
	if err != nil || stringField(status, "status") != "completed" {
		return false
	}
	data, _ := status["data"].([]any)
	if PoolImageArchiver == nil || len(data) == 0 {
		return false
	}
	body, keys, err := PoolImageArchiver(ctx, task, status["created"], data)
	if err != nil || len(keys) == 0 {
		logWorkerError("generation_task", "late_delivery_archive_failed", errors.Join(err, errors.New("no archived files")), "task_id", task.ID)
		return false
	}
	updatedAt := now()
	log := &model.CreditLog{ID: newID("credit"), Type: model.CreditLogTypeAILateRecharge, Remark: "结果延迟送达补扣 " + task.Model,
		Extra: generationTaskCreditExtra(task.Model, task.Path, task.PricingSnapshot), CreatedAt: updatedAt}
	recharged, err := repository.DeliverLateAndRecharge(task.ID, string(body), keys, updatedAt, log)
	if err != nil {
		logWorkerError("generation_task", "late_delivery_recharge_failed", err, "task_id", task.ID)
		return false
	}
	if recharged {
		logWorkerInfo("generation_task", "late_delivery_recharged", "task_id", task.ID, "credits", task.Credits)
	}
	return recharged
}
