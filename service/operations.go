package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type RuntimeHealth struct {
	Status   string          `json:"status"`
	Database string          `json:"database"`
	Gateway  *gateway.Health `json:"gateway,omitempty"`
}

// gatewayHealthCheck is replaceable in tests. It returns nil health when the
// gateway is disabled.
var gatewayHealthCheck = func(ctx context.Context, timeout time.Duration) (*gateway.Health, error) {
	if !gateway.Enabled() {
		return nil, nil
	}
	m := gateway.Current()
	if m == nil {
		return &gateway.Health{Status: "unavailable", Database: "unavailable", Redis: "unavailable"}, errors.New("gateway enabled but not running")
	}
	h, err := m.CheckHealth(ctx, timeout)
	return &h, err
}

const readinessTimeout = 2 * time.Second

type OperationsHealth struct {
	RuntimeHealth
	Queues     model.OperationsQueueHealth       `json:"queues"`
	Generation model.OperationsGenerationMetrics `json:"generation"`
	Alerts     []model.OperationsAlert           `json:"alerts"`
	CheckedAt  string                            `json:"checkedAt"`
}

func CheckReadiness(ctx context.Context) (RuntimeHealth, error) {
	ctx, cancel := context.WithTimeout(ctx, readinessTimeout)
	defer cancel()
	var dbErr, gwErr error
	var gw *gateway.Health
	done := make(chan struct{})
	go func() {
		defer close(done)
		gw, gwErr = gatewayHealthCheck(ctx, readinessTimeout)
	}()
	dbErr = repository.CheckDatabase(ctx)
	<-done
	result := RuntimeHealth{Status: "ok", Database: "ok", Gateway: gw}
	if dbErr != nil {
		result.Database = "unavailable"
	}
	if dbErr != nil || gwErr != nil {
		result.Status = "unavailable"
	}
	if dbErr != nil {
		return result, dbErr
	}
	return result, gwErr
}

func GetOperationsHealth(ctx context.Context) (OperationsHealth, error) {
	runtime, err := CheckReadiness(ctx)
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime}, err
	}
	timestamp := now()
	queues, err := repository.GetOperationsQueueHealth(timestamp)
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, CheckedAt: timestamp}, err
	}
	generation, durations, err := repository.GetOperationsGenerationMetrics(time.Now().UTC().Add(-24 * time.Hour).Format(timestampLayout))
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, Queues: queues, CheckedAt: timestamp}, err
	}
	generation = finalizeOperationsGenerationMetrics(generation, durations)
	settings, err := repository.GetSettings()
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, Queues: queues, Generation: generation, CheckedAt: timestamp}, err
	}
	alerts := operationsAlerts(queues, normalizePrivateSetting(settings.Private).OperationsAlerts)
	status := "ok"
	if len(alerts) > 0 {
		status = "degraded"
	}
	return OperationsHealth{RuntimeHealth: RuntimeHealth{Status: status, Database: runtime.Database}, Queues: queues, Generation: generation, Alerts: alerts, CheckedAt: timestamp}, nil
}

func finalizeOperationsGenerationMetrics(metrics model.OperationsGenerationMetrics, durations []int64) model.OperationsGenerationMetrics {
	metrics.WindowHours = 24
	terminal := metrics.Success + metrics.Failed
	if terminal > 0 {
		metrics.SuccessRate = math.Round(float64(metrics.Success)*10000/float64(terminal)) / 100
	}
	if len(durations) == 0 {
		return metrics
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	var total int64
	for _, duration := range durations {
		total += duration
	}
	metrics.AverageDurationMs = int64(math.Round(float64(total) / float64(len(durations))))
	metrics.P95DurationMs = durations[int(math.Ceil(float64(len(durations))*0.95))-1]
	return metrics
}

func operationsAlerts(queues model.OperationsQueueHealth, setting model.OperationsAlertSetting) []model.OperationsAlert {
	if setting.Enabled != nil && !*setting.Enabled {
		return []model.OperationsAlert{}
	}
	checks := []struct {
		key       string
		value     int64
		threshold *int64
	}{
		{"batch_queue_backlog", queues.BatchQueued, setting.BatchQueuedThreshold},
		{"batch_expired_leases", queues.BatchExpiredLeases, setting.BatchExpiredLeasesThreshold},
		{"email_outbox_pending", queues.EmailPending, setting.EmailPendingThreshold},
		{"email_outbox_failed", queues.EmailFailed, setting.EmailFailedThreshold},
		{"email_outbox_expired_leases", queues.EmailExpiredLeases, setting.EmailExpiredLeasesThreshold},
		{"object_deletion_outbox_pending", queues.ObjectDeletionPending, setting.ObjectDeletionPendingThreshold},
		{"object_deletion_outbox_failed", queues.ObjectDeletionFailed, setting.ObjectDeletionFailedThreshold},
		{"object_deletion_outbox_expired_leases", queues.ObjectDeletionExpiredLeases, setting.ObjectDeletionExpiredLeasesThreshold},
	}
	alerts := make([]model.OperationsAlert, 0)
	for _, check := range checks {
		if check.threshold != nil && *check.threshold > 0 && check.value >= *check.threshold {
			alerts = append(alerts, model.OperationsAlert{Key: check.key, Value: check.value, Threshold: *check.threshold})
		}
	}
	return alerts
}
