package service

import (
	"context"
	"errors"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
)

func (executor StandardBatchProductionExecutor) executeManagedBatch(ctx context.Context, user model.AuthUser, selection ModelChannelSelection, prompt, size string, references []string, task *model.GenerationTask) (BatchProductionResult, error) {
	pending := BatchProductionResult{GenerationTask: task}
	if task.ProtocolSnapshot == nil && task.UpstreamTaskID == "" {
		input := MediaGenerationInput{Prompt: prompt, Size: size, Ratio: size, ResolutionTier: task.ResolutionTier, Count: 1}
		client, cleanup := executor.standardResultClient()
		defer cleanup()
		for _, rawURL := range references {
			data, mimeType, err := downloadStandardBatchImage(ctx, client, rawURL, maxStandardBatchReferenceBytes)
			if err != nil {
				return pending, err
			}
			input.ReferenceImages = append(input.ReferenceImages, MediaReferenceFile{MimeType: mimeType, Data: data})
		}
		err := SubmitManagedMediaTask(ctx, user, task, selection, input)
		if err != nil && !errors.Is(err, ErrProtocolSubmissionUncertain) && task.UpstreamTaskID == "" {
			return pending, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, err)
		}
	}
	interval := 5 * time.Second
	if task.ProtocolSnapshot != nil {
		interval = time.Duration(task.ProtocolSnapshot.Template.Operations[task.ProtocolSnapshot.Operation].PollSeconds) * time.Second
	}
	for {
		if task.Status == model.GenerationTaskStatusFailed {
			return pending, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, errors.New(task.ErrorMessage))
		}
		if len(task.StorageKeys) > 0 {
			file, err := readProtocolStoredFile(ctx, user, task.StorageKeys[0], "image", maxUserFileSize)
			if err != nil {
				return pending, err
			}
			result, err := standardBatchImageResult(file.Data)
			result.GenerationTask = task
			return result, err
		}
		if !time.Now().Before(ManagedMediaDeadline(*task)) {
			return pending, permanentBatchProductionError(model.BatchProductionErrorUpstreamPermanent, errors.New("生成任务超过协议等待期限，未获得可交付结果"))
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return pending, ctx.Err()
		case <-timer.C:
		}
		_, _ = ReconcileManagedMediaTask(ctx, task, selection.Channel)
	}
}
