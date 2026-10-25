package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

// Built-in pool tasks are settled by 幻图 only after the pool is done with
// them. While a request handler is still waiting it owns the task; after a
// restart (or once the handler stops waiting) the maintenance worker asks the
// pool in-process by request ID and settles it:
//   - completed: archive the image and mark success (the original charge stands);
//   - failed: refund once;
//   - still running past the final deadline: abandon it in the pool first,
//     so no result can arrive after the refund, then refund once.
const (
	poolImageFinalDeadline = 40 * time.Minute
	poolVideoFinalDeadline = 45 * time.Minute
	poolTaskSubmitGrace    = 5 * time.Minute
)

type poolTaskBackend interface {
	ImageTask(ctx context.Context, requestID string) (map[string]any, error)
	AbandonImageTask(ctx context.Context, requestID string) (map[string]any, error)
	AbandonVideoJob(ctx context.Context, jobID string) (map[string]any, error)
}

// currentPoolTasks is replaceable in tests.
var currentPoolTasks = func() poolTaskBackend {
	if m := gateway.Current(); m != nil {
		return m
	}
	return nil
}

var isPoolTaskNotFound = gateway.IsTaskNotFound

// PoolImageArchiver stores a completed pool image result in the task owner's
// files. It is set by the handler package, which owns image archiving.
var PoolImageArchiver func(ctx context.Context, task model.GenerationTask, created any, data []any) ([]byte, []string, error)

var poolImageWaiters sync.Map

// TrackPoolImageWaiter marks a request handler as actively waiting on a pool
// image task, so maintenance leaves it alone. Call the returned func when the
// handler stops waiting.
func TrackPoolImageWaiter(requestID string) func() {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return func() {}
	}
	token := new(int)
	poolImageWaiters.Store(requestID, token)
	return func() { poolImageWaiters.CompareAndDelete(requestID, token) }
}

func poolImageWaiterActive(requestID string) bool {
	_, ok := poolImageWaiters.Load(strings.TrimSpace(requestID))
	return ok
}

// IsBuiltinPoolTask reports whether a task is (last) routed to the pool.
func IsBuiltinPoolTask(task model.GenerationTask) bool {
	if task.ChannelName == BuiltinPoolChannelName || task.ChannelID == BuiltinPoolChannelID {
		return true
	}
	if strings.HasPrefix(strings.TrimSpace(task.ChannelID), BuiltinProviderChannelPrefix) {
		return true
	}
	ref := strings.TrimSpace(task.ChannelID)
	if ref == "" {
		ref = task.ChannelName
	}
	channel, err := generationTaskChannel(ref)
	return err == nil && (isBuiltinPoolChannel(channel) || IsBuiltinProviderChannel(channel))
}

func generationTaskAge(task model.GenerationTask) time.Duration {
	createdAt, err := time.Parse(time.RFC3339, task.CreatedAt)
	if err != nil {
		return 0
	}
	return time.Since(createdAt)
}

// reconcilePoolImageTask reports whether the task was handled here.
func reconcilePoolImageTask(ctx context.Context, task model.GenerationTask) bool {
	if task.Modality != "image" || !IsBuiltinPoolTask(task) {
		return false
	}
	if poolImageWaiterActive(task.RequestID) {
		return true
	}
	backend := currentPoolTasks()
	if backend == nil {
		return false // pool not running: the generic timeout applies
	}
	age := generationTaskAge(task)
	status, err := backend.ImageTask(ctx, task.RequestID)
	if err != nil {
		if isPoolTaskNotFound(err) {
			if age < poolTaskSubmitGrace {
				return true
			}
			finishPoolTask(task, model.GenerationTaskStatusFailed, "上游未收到生成请求，已返还算力")
			return true
		}
		if age < poolImageFinalDeadline {
			return true
		}
		status = nil
	}
	switch stringField(status, "status") {
	case "completed":
		deliverPoolImage(ctx, task, status, age)
		return true
	case "failed":
		finishPoolTask(task, model.GenerationTaskStatusFailed, poolFailureMessage(status))
		return true
	}
	if age < poolImageFinalDeadline {
		return true
	}
	abandoned, abandonErr := backend.AbandonImageTask(ctx, task.RequestID)
	if abandonErr == nil && stringField(abandoned, "status") == "completed" {
		deliverPoolImage(ctx, task, abandoned, poolImageFinalDeadline+time.Hour)
		return true
	}
	if abandonErr != nil && !isPoolTaskNotFound(abandonErr) {
		return true // could not stop the pool; retry rather than refund early
	}
	finishPoolTask(task, model.GenerationTaskStatusFailed, fmt.Sprintf("生成任务超过 %d 分钟未完成，已结束并返还算力", int(poolImageFinalDeadline/time.Minute)))
	return true
}

func deliverPoolImage(ctx context.Context, task model.GenerationTask, status map[string]any, age time.Duration) {
	data, _ := status["data"].([]any)
	if PoolImageArchiver == nil || len(data) == 0 {
		finishPoolTask(task, model.GenerationTaskStatusFailed, "AI 接口请求失败：上游任务没有返回图片")
		return
	}
	body, keys, err := PoolImageArchiver(ctx, task, status["created"], data)
	if err != nil || len(keys) == 0 {
		if age < poolImageFinalDeadline {
			logWorkerError("generation_task", "pool_archive_failed", errors.Join(err, errors.New("no archived files")), "task_id", task.ID)
			return // retry on the next pass
		}
		finishPoolTask(task, model.GenerationTaskStatusFailed, "生成结果归档失败，已返还算力")
		return
	}
	if err := UpdateGenerationTaskRecovery(&task, "", body, keys); err != nil {
		logWorkerError("generation_task", "pool_result_persist_failed", err, "task_id", task.ID)
		return
	}
	finishPoolTask(task, model.GenerationTaskStatusSuccess, "")
}

// finalizePoolVideoTask reports whether finalization must wait. Pool video
// recovery runs for up to 20 minutes after acceptance on top of the render, so
// 幻图 waits longer than its generic 30 minutes and stops the pool job before
// refunding.
func finalizePoolVideoTask(ctx context.Context, task model.GenerationTask) bool {
	if task.Modality != "video" || task.UpstreamTaskID == "" || !IsBuiltinPoolTask(task) {
		return false
	}
	if generationTaskAge(task) < poolVideoFinalDeadline {
		return true
	}
	if backend := currentPoolTasks(); backend != nil {
		if _, err := backend.AbandonVideoJob(ctx, task.UpstreamTaskID); err != nil && !isPoolTaskNotFound(err) {
			return true
		}
		if settled, _ := reconcileRunningVideoTask(ctx, task); settled {
			return true
		}
	}
	return false
}

func finishPoolTask(task model.GenerationTask, status model.GenerationTaskStatus, message string) {
	if err := FinishGenerationTask(task, status, message); err != nil {
		logWorkerError("generation_task", "pool_finalize_failed", err, "task_id", task.ID)
	}
}

func poolFailureMessage(status map[string]any) string {
	if message := strings.TrimSpace(stringField(status, "error")); message != "" {
		return "AI 接口请求失败：" + message
	}
	return "AI 接口请求失败：上游生图任务失败"
}

func stringField(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return strings.ToLower(strings.TrimSpace(value))
}
