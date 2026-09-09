package repository

import (
	"context"

	"github.com/yypyyd/infinite-canvas/model"
)

func CheckDatabase(ctx context.Context) error {
	db, err := DB()
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func GetOperationsQueueHealth(timestamp string) (model.OperationsQueueHealth, error) {
	db, err := DB()
	if err != nil {
		return model.OperationsQueueHealth{}, err
	}
	var result model.OperationsQueueHealth
	counts := []struct {
		target *int64
		query  any
		where  string
		args   []any
	}{
		{&result.BatchQueued, &model.BatchProductionItem{}, "status = ?", []any{model.BatchProductionStatusQueued}},
		{&result.BatchRunning, &model.BatchProductionItem{}, "status = ?", []any{model.BatchProductionStatusRunning}},
		{&result.BatchExpiredLeases, &model.BatchProductionItem{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{model.BatchProductionStatusRunning, timestamp}},
		{&result.EmailPending, &model.OrganizationEmailOutbox{}, "status = ?", []any{"pending"}},
		{&result.EmailFailed, &model.OrganizationEmailOutbox{}, "status = ?", []any{"failed"}},
		{&result.EmailExpiredLeases, &model.OrganizationEmailOutbox{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{"processing", timestamp}},
		{&result.ObjectDeletionPending, &model.UserObjectDeletion{}, "status = ?", []any{"pending"}},
		{&result.ObjectDeletionFailed, &model.UserObjectDeletion{}, "status = ?", []any{"failed"}},
		{&result.ObjectDeletionExpiredLeases, &model.UserObjectDeletion{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{"processing", timestamp}},
	}
	for _, count := range counts {
		if err := db.Model(count.query).Where(count.where, count.args...).Count(count.target).Error; err != nil {
			return model.OperationsQueueHealth{}, err
		}
	}
	return result, nil
}

func GetOperationsGenerationMetrics(since string) (model.OperationsGenerationMetrics, []int64, error) {
	db, err := DB()
	if err != nil {
		return model.OperationsGenerationMetrics{}, nil, err
	}
	var counts []struct {
		Status model.GenerationTaskStatus
		Value  int64
	}
	if err := db.Model(&model.GenerationTask{}).Where("created_at >= ?", since).Select("status, COUNT(*) AS value").Group("status").Scan(&counts).Error; err != nil {
		return model.OperationsGenerationMetrics{}, nil, err
	}
	var metrics model.OperationsGenerationMetrics
	for _, count := range counts {
		metrics.Total += count.Value
		switch count.Status {
		case model.GenerationTaskStatusRunning:
			metrics.Running = count.Value
		case model.GenerationTaskStatusSuccess:
			metrics.Success = count.Value
		case model.GenerationTaskStatusFailed:
			metrics.Failed = count.Value
		}
	}
	var durations []int64
	err = db.Model(&model.GenerationTask{}).
		Where("created_at >= ? AND status IN ?", since, []model.GenerationTaskStatus{model.GenerationTaskStatusSuccess, model.GenerationTaskStatusFailed}).
		Pluck("duration_ms", &durations).Error
	return metrics, durations, err
}
