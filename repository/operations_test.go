package repository

import (
	"context"
	"sort"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestOperationsHealthChecksDatabaseAndQueueBacklogs(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	if err := CheckDatabase(context.Background()); err != nil {
		t.Fatalf("database readiness: %v", err)
	}
	batchItems := []model.BatchProductionItem{
		{ID: "batch-queued", Status: model.BatchProductionStatusQueued},
		{ID: "batch-running-active", Status: model.BatchProductionStatusRunning, LeaseExpiresAt: workspaceTestFuture},
		{ID: "batch-running-expired", Status: model.BatchProductionStatusRunning, LeaseExpiresAt: workspaceTestOld},
	}
	if err := testDB.Create(&batchItems).Error; err != nil {
		t.Fatalf("create batch items: %v", err)
	}
	emails := []model.OrganizationEmailOutbox{
		{ID: "email-pending", InvitationID: "invitation-pending", Status: "pending"},
		{ID: "email-failed", InvitationID: "invitation-failed", Status: "failed"},
		{ID: "email-processing-active", InvitationID: "invitation-active", Status: "processing", LeaseExpiresAt: workspaceTestFuture},
		{ID: "email-processing-expired", InvitationID: "invitation-expired", Status: "processing", LeaseExpiresAt: workspaceTestOld},
	}
	if err := testDB.Create(&emails).Error; err != nil {
		t.Fatalf("create email outbox items: %v", err)
	}
	deletions := []model.UserObjectDeletion{
		{ID: "delete-pending", ObjectKey: "delete-pending", Status: "pending"},
		{ID: "delete-failed", ObjectKey: "delete-failed", Status: "failed"},
		{ID: "delete-processing-active", ObjectKey: "delete-processing-active", Status: "processing", LeaseExpiresAt: workspaceTestFuture},
		{ID: "delete-processing-expired", ObjectKey: "delete-processing-expired", Status: "processing", LeaseExpiresAt: workspaceTestOld},
	}
	if err := testDB.Create(&deletions).Error; err != nil {
		t.Fatalf("create object deletion items: %v", err)
	}

	health, err := GetOperationsQueueHealth(workspaceTestNow)
	if err != nil {
		t.Fatalf("get operations queue health: %v", err)
	}
	if health.BatchQueued != 1 || health.BatchRunning != 2 || health.BatchExpiredLeases != 1 {
		t.Fatalf("unexpected batch queue health: %#v", health)
	}
	if health.EmailPending != 1 || health.EmailFailed != 1 || health.EmailExpiredLeases != 1 {
		t.Fatalf("unexpected email outbox health: %#v", health)
	}
	if health.ObjectDeletionPending != 1 || health.ObjectDeletionFailed != 1 || health.ObjectDeletionExpiredLeases != 1 {
		t.Fatalf("unexpected object deletion health: %#v", health)
	}
}

func TestOperationsGenerationMetricsOnlyUseRequestedWindow(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	tasks := []model.GenerationTask{
		{ID: "generation-success-a", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-a", Status: model.GenerationTaskStatusSuccess, DurationMs: 100, CreatedAt: workspaceTestNow},
		{ID: "generation-success-b", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-b", Status: model.GenerationTaskStatusSuccess, DurationMs: 300, CreatedAt: workspaceTestNow},
		{ID: "generation-failed", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-c", Status: model.GenerationTaskStatusFailed, DurationMs: 500, CreatedAt: workspaceTestNow},
		{ID: "generation-running", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-d", Status: model.GenerationTaskStatusRunning, CreatedAt: workspaceTestNow},
		{ID: "generation-old", OrganizationID: "organization-a", UserID: "user-a", RequestID: "request-e", Status: model.GenerationTaskStatusFailed, DurationMs: 900, CreatedAt: workspaceTestOld},
	}
	if err := testDB.Create(&tasks).Error; err != nil {
		t.Fatalf("create generation tasks: %v", err)
	}

	metrics, durations, err := GetOperationsGenerationMetrics("2026-07-25T00:00:00Z")
	if err != nil {
		t.Fatalf("get generation metrics: %v", err)
	}
	if metrics.Total != 4 || metrics.Success != 2 || metrics.Failed != 1 || metrics.Running != 1 {
		t.Fatalf("unexpected generation metrics: %#v", metrics)
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	if len(durations) != 3 || durations[0] != 100 || durations[1] != 300 || durations[2] != 500 {
		t.Fatalf("unexpected terminal durations: %#v", durations)
	}
}
