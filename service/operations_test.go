package service

import (
	"context"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
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

func TestInspectDataConsistencySnapshotFindsRepairableIssues(t *testing.T) {
	snapshot := repository.DataConsistencySnapshot{
		Organizations: []model.Organization{{ID: "organization-consistency"}},
		Users: []model.User{{ID: "user-consistency", Credits: 9}},
		Files: []model.UserFile{
			{ID: "file-state", OrganizationID: "organization-consistency", StorageKey: "image:state", ObjectKey: "organizations/organization-consistency/files/state.png", Hash: "hash-state", MimeType: "image/png", Size: 10, UnreferencedAt: "old"},
			{ID: "file-batch", OrganizationID: "organization-consistency", StorageKey: "image:batch", ObjectKey: "organizations/organization-consistency/batch-results/batch.png", Hash: "hash-batch", MimeType: "image/png", Size: 20, UnreferencedAt: "old"},
		},
		References: []model.UserFileReference{
			{ID: "reference-valid", OrganizationID: "organization-consistency", Domain: "asset", ObjectID: "asset-a", StorageKey: "image:state"},
			{ID: "reference-dangling", OrganizationID: "organization-consistency", Domain: "asset", ObjectID: "asset-b", StorageKey: "image:missing"},
		},
		GenerationTasks: []model.GenerationTask{{ID: "task-consistency", OrganizationID: "organization-consistency", UserID: "user-consistency", Credits: 1, Status: model.GenerationTaskStatusSuccess}},
		CreditLogs: []model.CreditLog{{ID: "log-consistency", UserID: "user-consistency", Type: model.CreditLogTypeAIConsume, Amount: -1, Balance: 9, RelatedID: "task-consistency"}},
		BatchItems: []model.BatchProductionItem{{ID: "batch-item-consistency", OrganizationID: "organization-consistency", Status: model.BatchProductionStatusCompleted, ResultStorageKey: "image:batch"}},
	}
	objects := []dataConsistencyObject{
		{Key: "organizations/organization-consistency/files/state.png", Hash: "hash-state", MimeType: "image/png", Size: 10},
		{Key: "organizations/organization-consistency/batch-results/batch.png", Hash: "hash-batch", MimeType: "image/png", Size: 20},
	}
	report := inspectDataConsistencySnapshot(snapshot, objects, "ok")
	if report.TotalIssues != 3 || report.Repairable != 3 || report.Summary["media_reference"] != 2 || report.Summary["batch_result"] != 1 { t.Fatalf("unexpected consistency report: %#v", report) }
	codes := map[string]bool{}
	for _, issue := range report.Issues { codes[issue.Code] = true }
	for _, code := range []string{"dangling_file_reference", "file_reference_state_mismatch", "batch_result_reference_mismatch"} { if !codes[code] { t.Fatalf("missing issue %q: %#v", code, report.Issues) } }
}
