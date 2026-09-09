package service

import (
	"context"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestGetOperationsHealthReportsFailedOutboxAsDegraded(t *testing.T) {
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	item := model.UserObjectDeletion{ID: "operations-health-failed", ObjectKey: "operations-health-failed", Status: "failed"}
	if err := db.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	health, err := GetOperationsHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if health.Status != "degraded" || health.Database != "ok" || health.Queues.ObjectDeletionFailed < 1 {
		t.Fatalf("unexpected degraded operations health: %#v", health)
	}
}

func TestCheckReadinessReportsCancelledDatabaseProbeAsUnavailable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	health, err := CheckReadiness(ctx)
	if err == nil || health.Status != "unavailable" || health.Database != "unavailable" {
		t.Fatalf("cancelled readiness = %#v, err=%v", health, err)
	}
}

func TestFinalizeOperationsGenerationMetricsCalculatesSuccessRateAndLatency(t *testing.T) {
	metrics := finalizeOperationsGenerationMetrics(model.OperationsGenerationMetrics{Total: 4, Running: 1, Success: 2, Failed: 1}, []int64{1000, 100, 200})
	if metrics.WindowHours != 24 || metrics.SuccessRate != 66.67 || metrics.AverageDurationMs != 433 || metrics.P95DurationMs != 1000 {
		t.Fatalf("unexpected generation metrics: %#v", metrics)
	}
	empty := finalizeOperationsGenerationMetrics(model.OperationsGenerationMetrics{}, nil)
	if empty.WindowHours != 24 || empty.SuccessRate != 0 || empty.AverageDurationMs != 0 || empty.P95DurationMs != 0 {
		t.Fatalf("unexpected empty generation metrics: %#v", empty)
	}
}

func TestOperationsAlertsRespectBoundaryAndDisabledThresholds(t *testing.T) {
	enabled := true
	pendingThreshold := int64(2)
	disabledThreshold := int64(0)
	alerts := operationsAlerts(model.OperationsQueueHealth{EmailPending: 2, EmailFailed: 10}, model.OperationsAlertSetting{
		Enabled:               &enabled,
		EmailPendingThreshold: &pendingThreshold,
		EmailFailedThreshold:  &disabledThreshold,
	})
	if len(alerts) != 1 || alerts[0].Key != "email_outbox_pending" || alerts[0].Value != 2 || alerts[0].Threshold != 2 {
		t.Fatalf("unexpected alerts: %#v", alerts)
	}
	enabled = false
	if alerts := operationsAlerts(model.OperationsQueueHealth{EmailPending: 100}, model.OperationsAlertSetting{Enabled: &enabled, EmailPendingThreshold: &pendingThreshold}); len(alerts) != 0 {
		t.Fatalf("disabled alerts = %#v", alerts)
	}
}
