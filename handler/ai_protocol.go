package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func proxyCustomMediaGeneration(w http.ResponseWriter, r *http.Request, user model.AuthUser, selection service.ModelChannelSelection, task *model.GenerationTask, input service.MediaGenerationInput, responseFormat string, finish func(model.GenerationTaskStatus, string)) {
	if err := service.ValidateManagedMediaRequest(selection, service.PricingRequest{Modality: task.Modality, Operation: task.Operation, Quantity: task.Quantity, GenerateAudio: input.GenerateAudio}); err != nil {
		finish(model.GenerationTaskStatusFailed, err.Error())
		Fail(w, err.Error())
		return
	}
	// Let submission persist the upstream receipt even if the browser disconnects.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 15*time.Minute)
	err := service.SubmitManagedMediaTask(ctx, user, task, selection, input)
	cancel()
	if err != nil && !errors.Is(err, service.ErrProtocolSubmissionUncertain) && task.UpstreamTaskID == "" {
		finish(model.GenerationTaskStatusFailed, err.Error())
		Fail(w, err.Error())
		return
	}
	if task.Status == model.GenerationTaskStatusFailed {
		Fail(w, task.ErrorMessage)
		return
	}
	if task.Modality == "video" && task.UpstreamTaskID != "" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(task.ResultJSON))
		return
	}
	if task.UpstreamTaskID == "" && task.Status != model.GenerationTaskStatusSuccess {
		writeOpenAIError(w, http.StatusGatewayTimeout, "任务已记录，上游受理结果待确认，请通过原请求编号查询，勿重新提交", "api_error", "submission_pending", nil)
		return
	}
	deadline := service.ManagedMediaDeadline(*task)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if task.Status == model.GenerationTaskStatusSuccess {
			body, err := service.ManagedMediaResult(r.Context(), user, *task, responseFormat)
			if err != nil {
				Fail(w, "生成结果已归档，请从生成记录读取")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
			return
		}
		if time.Now().After(deadline.Add(time.Minute)) {
			writeOpenAIError(w, http.StatusGatewayTimeout, "任务仍由服务端处理，请通过原请求编号查询", "api_error", "generation_pending", nil)
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		saved, err := service.UserGenerationTaskByRequest(user.OrganizationID, user.ID, task.RequestID)
		if err != nil {
			continue
		}
		*task = saved
		if task.Status == model.GenerationTaskStatusFailed {
			Fail(w, task.ErrorMessage)
			return
		}
	}
}
