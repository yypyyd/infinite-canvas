package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	generationTaskReconcileDelay       = time.Minute
	generationTaskTimeout              = 30 * time.Minute
	generationTaskReconcileTimeout     = 15 * time.Minute
	generationTaskReconcileConcurrency = 4
	maxGenerationTaskStatusSize        = 1 << 20
)

var (
	generationTaskStatusClient  = &http.Client{Timeout: 30 * time.Second}
	generationTaskArchiveClient = &http.Client{Timeout: 15 * time.Minute}
)

func StartGenerationTaskMaintenanceWorker() {
	logWorkerInfo("generation_task", "worker_started", "reconcile_minutes", int(generationTaskReconcileDelay/time.Minute), "timeout_minutes", int(generationTaskTimeout/time.Minute))
	go func() {
		slots := make(chan struct{}, generationTaskReconcileConcurrency)
		for {
			dispatchGenerationTaskReconciliations(time.Now().UTC(), slots)
			lateCtx, cancelLate := context.WithTimeout(context.Background(), 2*time.Minute)
			checkLateDeliveredPoolImages(lateCtx, time.Now().UTC())
			cancelLate()
			time.Sleep(5 * time.Second)
		}
	}()
}

func dispatchGenerationTaskReconciliations(timestamp time.Time, slots chan struct{}) {
	available := cap(slots) - len(slots)
	if available == 0 {
		return
	}
	tasks, err := repository.ListStaleRunningGenerationTasks(timestamp.Add(-generationTaskReconcileDelay).Format(timestampLayout), timestamp.Format(timestampLayout), available)
	if err != nil {
		logWorkerError("generation_task", "stale_list_failed", err)
		return
	}
	for _, candidate := range tasks {
		claimedAt := time.Now().UTC()
		task, claimed, claimErr := repository.ClaimGenerationTaskReconcile(candidate.ID, newID("reconcile"), claimedAt.Format(timestampLayout), claimedAt.Add(generationTaskReconcileTimeout+time.Minute).Format(timestampLayout))
		if claimErr != nil {
			logWorkerError("generation_task", "reconcile_claim_failed", claimErr, "task_id", candidate.ID)
		}
		if !claimed {
			continue
		}
		slots <- struct{}{}
		go func() {
			defer func() { <-slots }()
			defer func() {
				delay := generationTaskReconcileDelay
				if task.ProtocolSnapshot != nil && task.UpstreamTaskID != "" {
					delay = time.Duration(task.ProtocolSnapshot.Template.Operations[task.ProtocolSnapshot.Operation].PollSeconds) * time.Second
				}
				if err := repository.ReleaseGenerationTaskReconcile(task.ID, task.ReconcileLeaseToken, time.Now().UTC().Add(delay).Format(timestampLayout)); err != nil {
					logWorkerError("generation_task", "reconcile_release_failed", err, "task_id", task.ID)
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), generationTaskReconcileTimeout)
			defer cancel()
			reconcileStaleGenerationTask(ctx, task)
		}()
	}
}

func reconcileStaleGenerationTask(ctx context.Context, task model.GenerationTask) {
	if task.ProtocolSnapshot != nil {
		settled, err := ReconcileManagedMediaTask(ctx, &task, model.ModelChannel{})
		if settled {
			return
		}
		if err != nil && !errors.Is(err, ErrProtocolSubmissionUncertain) {
			logWorkerError("generation_task", "protocol_reconcile_failed", err, "task_id", task.ID)
		}
		if time.Now().Before(ManagedMediaDeadline(task)) {
			return
		}
		if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "生成任务超过协议等待期限且结果未能归档，已结束并返还算力"); err != nil {
			logWorkerError("generation_task", "protocol_finalize_failed", err, "task_id", task.ID)
		}
		return
	}
	autoDLTask := false
	if task.UpstreamTaskID != "" {
		channel, channelErr := generationTaskChannel(task.ChannelName)
		if channelErr == nil && IsAutoDLComfyUIChannel(channel) {
			autoDLTask = true
			settled, reconcileErr := ReconcileAutoDLGenerationTask(ctx, &task, channel)
			if reconcileErr != nil {
				logWorkerError("generation_task", "autodl_reconcile_failed", reconcileErr, "task_id", task.ID)
			}
			if settled {
				return
			}
		}
	}
	if !autoDLTask && task.Modality == "video" && task.UpstreamTaskID != "" {
		settled, reconcileErr := reconcileRunningVideoTask(ctx, task)
		if reconcileErr != nil {
			logWorkerError("generation_task", "video_reconcile_failed", reconcileErr, "task_id", task.ID)
		}
		if settled {
			return
		}
	}
	if reconcilePoolImageTask(ctx, task) || finalizePoolVideoTask(ctx, task) {
		return
	}
	createdAt, parseErr := time.Parse(time.RFC3339, task.CreatedAt)
	if parseErr == nil && time.Since(createdAt) < generationTaskTimeout {
		return
	}
	if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "生成任务超过 30 分钟未完成，已自动结束"); err != nil {
		logWorkerError("generation_task", "stale_finalize_failed", err, "task_id", task.ID)
		return
	}
	logWorkerInfo("generation_task", "stale_finalized", "task_id", task.ID, "modality", task.Modality)
}

func reconcileRunningVideoTask(ctx context.Context, task model.GenerationTask) (bool, error) {
	if len(task.StorageKeys) > 0 {
		return true, FinishGenerationTask(task, model.GenerationTaskStatusSuccess, "")
	}
	channel, err := generationTaskChannel(task.ChannelName)
	if err != nil {
		return false, err
	}
	statusURL := BuildModelChannelURL(channel, "/videos/"+url.PathEscape(task.UpstreamTaskID))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	response, err := generationTaskStatusClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxGenerationTaskStatusSize+1))
	if err != nil {
		return false, err
	}
	if len(body) > maxGenerationTaskStatusSize {
		return false, errors.New("video task status response is too large")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("video task status returned HTTP %d", response.StatusCode)
	}
	switch generationTaskVideoStatus(body) {
	case "completed":
		return archiveReconciledVideoTask(ctx, task, channel, body)
	case "failed", "cancelled", "canceled":
		if err := UpdateGenerationTaskRecovery(&task, task.UpstreamTaskID, body, nil); err != nil {
			return false, err
		}
		message := generationTaskVideoError(body)
		if message == "" {
			message = "视频生成失败"
		}
		return true, FinishGenerationTask(task, model.GenerationTaskStatusFailed, message)
	case "running":
		return false, nil
	default:
		return false, errors.New("video task returned an unknown status")
	}
}

func archiveReconciledVideoTask(ctx context.Context, task model.GenerationTask, channel model.ModelChannel, statusBody []byte) (bool, error) {
	contentURL := BuildModelChannelURL(channel, "/videos/"+url.PathEscape(task.UpstreamTaskID)+"/content")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+channel.APIKey)
	response, err := generationTaskArchiveClient.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("video task content returned HTTP %d", response.StatusCode)
	}
	mimeType := strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = "video/mp4"
	}
	if !strings.HasPrefix(mimeType, "video/") {
		return false, errors.New("video task content type is invalid")
	}
	user, exists, err := repository.GetUserByID(task.UserID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, errors.New("video task user does not exist")
	}
	authUser := model.PublicUser(user)
	authUser.OrganizationID = task.OrganizationID
	file, err := ArchiveGeneratedStream(ctx, authUser, task.ID, "video", 1, mimeType, response.ContentLength, response.Body)
	if err != nil {
		return false, err
	}
	result := attachGenerationTaskVideoStorage(statusBody, file)
	if err := UpdateGenerationTaskRecovery(&task, task.UpstreamTaskID, result, []string{file.StorageKey}); err != nil {
		return false, err
	}
	if err := FinishGenerationTask(task, model.GenerationTaskStatusSuccess, ""); err != nil {
		return false, err
	}
	logWorkerInfo("generation_task", "video_reconciled", "task_id", task.ID, "storage_key", file.StorageKey)
	return true, nil
}

func generationTaskChannel(name string) (model.ModelChannel, error) {
	channel, ok, err := runtimeChannelByName(name)
	if err != nil {
		return model.ModelChannel{}, err
	}
	if !ok {
		return model.ModelChannel{}, errors.New("generation task channel is unavailable")
	}
	return channel, nil
}

func generationTaskVideoPayload(body []byte) map[string]any {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	if data, ok := payload["data"].(map[string]any); ok {
		return data
	}
	return payload
}

func generationTaskVideoStatus(body []byte) string {
	payload := generationTaskVideoPayload(body)
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(payload["status"])))
	if done, ok := payload["done"].(bool); ok {
		if done {
			return "completed"
		}
		if status == "" || status == "<nil>" {
			return "running"
		}
	}
	switch status {
	case "succeeded", "success", "completed", "done":
		return "completed"
	case "queued", "pending", "running", "processing", "in_progress":
		return "running"
	default:
		return status
	}
}

func generationTaskVideoError(body []byte) string {
	payload := generationTaskVideoPayload(body)
	if item, ok := payload["error"].(map[string]any); ok {
		if message := strings.TrimSpace(fmt.Sprint(item["message"])); message != "" && message != "<nil>" {
			return message
		}
	}
	message := strings.TrimSpace(fmt.Sprint(payload["message"]))
	if message == "<nil>" {
		return ""
	}
	return message
}

func attachGenerationTaskVideoStorage(body []byte, file model.UserFile) []byte {
	var payload map[string]any
	if json.Unmarshal(body, &payload) != nil {
		return body
	}
	target := payload
	if data, ok := payload["data"].(map[string]any); ok {
		target = data
	}
	target["status"] = "completed"
	target["storage_key"] = file.StorageKey
	target["mime_type"] = file.MimeType
	target["bytes"] = file.Size
	result, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return result
}
