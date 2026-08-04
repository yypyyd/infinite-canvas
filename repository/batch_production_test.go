package repository

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

const batchTestExpired = "2026-07-25T09:00:00.000Z"

func createBatchTestJob(t *testing.T, testDB *gorm.DB, organizationID, jobID string, status model.BatchProductionStatus, itemStatuses ...model.BatchProductionStatus) {
	t.Helper()
	job := model.BatchProductionJob{ID: jobID, OrganizationID: organizationID, RequestID: "request-" + jobID, RequestHash: "hash-" + jobID, ArchiveToken: "archive-" + jobID, Name: jobID, PresetID: "product-main", Status: status, TotalItems: len(itemStatuses), CreatedBy: "user-a", CreatedAt: workspaceTestOld, UpdatedAt: workspaceTestOld}
	if err := testDB.Create(&job).Error; err != nil {
		t.Fatalf("create batch job %s: %v", jobID, err)
	}
	for index, itemStatus := range itemStatuses {
		item := model.BatchProductionItem{ID: jobID + "-item-" + string(rune('a'+index)), OrganizationID: organizationID, JobID: jobID, ProductID: "product-" + jobID, Status: itemStatus, RunNumber: 1, CreatedAt: workspaceTestOld, UpdatedAt: workspaceTestOld}
		if itemStatus == model.BatchProductionStatusRunning {
			item.Attempts, item.LeaseToken, item.LeaseExpiresAt, item.LockedAt, item.StartedAt = 1, "lease-old", workspaceTestFuture, workspaceTestNow, workspaceTestNow
		}
		if itemStatus == model.BatchProductionStatusCompleted || itemStatus == model.BatchProductionStatusFailed || itemStatus == model.BatchProductionStatusCancelled {
			item.FinishedAt = workspaceTestOld
		}
		if err := testDB.Create(&item).Error; err != nil {
			t.Fatalf("create batch item %s: %v", item.ID, err)
		}
	}
}

func TestClaimNextBatchProductionItemDoesNotDuplicateConcurrentClaims(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusQueued, model.BatchProductionStatusQueued, model.BatchProductionStatusQueued)

	type claimResult struct {
		item    model.BatchProductionItem
		claimed bool
		err     error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	for _, token := range []string{"lease-a", "lease-b"} {
		token := token
		go func() {
			<-start
			item, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, token, 2)
			results <- claimResult{item: item, claimed: claimed, err: err}
		}()
	}
	close(start)

	claimedIDs := map[string]bool{}
	for range 2 {
		result := <-results
		if result.err != nil || !result.claimed {
			t.Fatalf("claim failed, claimed=%v err=%v", result.claimed, result.err)
		}
		if claimedIDs[result.item.ID] {
			t.Fatalf("batch item %s was claimed twice", result.item.ID)
		}
		claimedIDs[result.item.ID] = true
	}
	if len(claimedIDs) != 2 {
		t.Fatalf("claimed %d distinct items, want 2", len(claimedIDs))
	}
}

func TestClaimNextBatchProductionItemRespectsTenantLimitAndFairness(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a", "organization-b")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusQueued, model.BatchProductionStatusQueued, model.BatchProductionStatusQueued)
	createBatchTestJob(t, testDB, "organization-b", "job-b", model.BatchProductionStatusQueued, model.BatchProductionStatusQueued)

	first, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-a", 1)
	if err != nil || !claimed {
		t.Fatalf("first claim failed, claimed=%v err=%v", claimed, err)
	}
	second, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-b", 1)
	if err != nil || !claimed {
		t.Fatalf("second claim failed, claimed=%v err=%v", claimed, err)
	}
	if first.OrganizationID == second.OrganizationID {
		t.Fatalf("tenant limit did not rotate work: first=%s second=%s", first.OrganizationID, second.OrganizationID)
	}
	if _, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-c", 1); err != nil || claimed {
		t.Fatalf("third claim should be blocked by per-tenant limit, claimed=%v err=%v", claimed, err)
	}
}

func TestExpiredBatchProductionLeaseCanBeTakenOverSafely(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusRunning, model.BatchProductionStatusRunning)
	var stale model.BatchProductionItem
	if err := testDB.First(&stale, "job_id = ?", "job-a").Error; err != nil {
		t.Fatalf("load stale item: %v", err)
	}
	if err := testDB.Model(&model.BatchProductionItem{}).Where("id = ?", stale.ID).Update("lease_expires_at", batchTestExpired).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	claimedItem, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-new", 1)
	if err != nil || !claimed {
		t.Fatalf("take over expired lease, claimed=%v err=%v", claimed, err)
	}
	if claimedItem.ID != stale.ID || claimedItem.LeaseToken != "lease-new" || claimedItem.Attempts != 2 {
		t.Fatalf("unexpected claimed item: %#v", claimedItem)
	}
	if err := RenewBatchProductionItemLease(stale, workspaceTestFuture, workspaceTestNow); !errors.Is(err, ErrBatchProductionLeaseLost) {
		t.Fatalf("stale lease renewal error = %v, want lease lost", err)
	}
	if err := FinishBatchProductionItem(stale, model.BatchProductionStatusCompleted, "", "", workspaceTestNow); !errors.Is(err, ErrBatchProductionLeaseLost) {
		t.Fatalf("stale lease finish error = %v, want lease lost", err)
	}
	if err := FinishBatchProductionItem(claimedItem, model.BatchProductionStatusCompleted, "", "", workspaceTestNow); err != nil {
		t.Fatalf("finish current lease: %v", err)
	}
	var savedItem model.BatchProductionItem
	if err := testDB.First(&savedItem, "id = ?", stale.ID).Error; err != nil {
		t.Fatalf("load finished item: %v", err)
	}
	if savedItem.Status != model.BatchProductionStatusCompleted || savedItem.Attempts != 2 || savedItem.LeaseToken != "" {
		t.Fatalf("unexpected finished item: %#v", savedItem)
	}
}

func TestCancelBatchProductionJobIsTerminalForClaimedWorker(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusQueued, model.BatchProductionStatusQueued)
	item, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-a", 1)
	if err != nil || !claimed {
		t.Fatalf("claim item, claimed=%v err=%v", claimed, err)
	}
	if err := CancelBatchProductionJob("organization-a", "job-a", workspaceTestNow); err != nil {
		t.Fatalf("cancel job: %v", err)
	}
	if err := RenewBatchProductionItemLease(item, workspaceTestFuture, workspaceTestNow); !errors.Is(err, ErrBatchProductionLeaseLost) {
		t.Fatalf("cancelled lease renewal error = %v, want lease lost", err)
	}
	if err := FinishBatchProductionItem(item, model.BatchProductionStatusCompleted, "", "", workspaceTestNow); !errors.Is(err, ErrBatchProductionLeaseLost) {
		t.Fatalf("cancelled lease finish error = %v, want lease lost", err)
	}
	var job model.BatchProductionJob
	if err := testDB.First(&job, "id = ?", "job-a").Error; err != nil {
		t.Fatalf("load cancelled job: %v", err)
	}
	var savedItem model.BatchProductionItem
	if err := testDB.First(&savedItem, "id = ?", item.ID).Error; err != nil {
		t.Fatalf("load cancelled item: %v", err)
	}
	if job.Status != model.BatchProductionStatusCancelled || savedItem.Status != model.BatchProductionStatusCancelled || savedItem.LeaseToken != "" {
		t.Fatalf("unexpected cancelled state: job=%s item=%#v", job.Status, savedItem)
	}
}

func TestRetryBatchProductionJobStartsNewRunOnlyForFailedItems(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusFailed, model.BatchProductionStatusFailed, model.BatchProductionStatusCompleted)
	if err := testDB.Model(&model.Organization{}).Where("id = ?", "organization-a").Updates(map[string]any{"credit_mode": model.OrganizationCreditModeShared, "credits": 10}).Error; err != nil {
		t.Fatalf("seed organization credits: %v", err)
	}
	if err := testDB.Model(&model.BatchProductionJob{}).Where("id = ?", "job-a").Updates(map[string]any{"completed_items": 1, "failed_items": 1, "credit_source": model.CreditSourceOrganization}).Error; err != nil {
		t.Fatalf("seed job counters: %v", err)
	}
	if err := testDB.Model(&model.BatchProductionItem{}).Where("job_id = ? AND status = ?", "job-a", model.BatchProductionStatusFailed).Updates(map[string]any{"attempts": 5, "estimated_credits": 1, "error_message": "failed", "lease_token": "stale", "lease_expires_at": batchTestExpired}).Error; err != nil {
		t.Fatalf("seed failed item: %v", err)
	}

	if err := RetryBatchProductionJob("organization-a", "job-a", workspaceTestNow); err != nil {
		t.Fatalf("retry job: %v", err)
	}
	var items []model.BatchProductionItem
	if err := testDB.Where("job_id = ?", "job-a").Order("id asc").Find(&items).Error; err != nil {
		t.Fatalf("load retried items: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("retried item count = %d, want 2", len(items))
	}
	for _, item := range items {
		switch item.Status {
		case model.BatchProductionStatusQueued:
			if item.RunNumber != 2 || item.Attempts != 0 || item.LeaseToken != "" || item.ErrorMessage != "" {
				t.Fatalf("failed item was not reset for a new run: %#v", item)
			}
		case model.BatchProductionStatusCompleted:
			if item.RunNumber != 1 {
				t.Fatalf("completed item run changed: %#v", item)
			}
		default:
			t.Fatalf("unexpected item status after retry: %#v", item)
		}
	}
	var job model.BatchProductionJob
	if err := testDB.First(&job, "id = ?", "job-a").Error; err != nil {
		t.Fatalf("load retried job: %v", err)
	}
	if job.Status != model.BatchProductionStatusQueued || job.CompletedItems != 1 || job.FailedItems != 0 {
		t.Fatalf("unexpected retried job: %#v", job)
	}
}

func TestCreateBatchProductionJobIsRequestIdempotent(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	if err := testDB.Model(&model.Organization{}).Where("id = ?", "organization-a").Updates(map[string]any{"credit_mode": model.OrganizationCreditModeShared, "credits": 10}).Error; err != nil {
		t.Fatalf("seed organization credits: %v", err)
	}
	product := model.Product{ID: "product-a", OrganizationID: "organization-a", Code: "product-a", Name: "Product A", Status: model.ProductStatusActive, CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	if err := testDB.Create(&product).Error; err != nil {
		t.Fatalf("create product: %v", err)
	}
	first := model.BatchProductionJob{ID: "job-a", OrganizationID: "organization-a", RequestID: "request-a", RequestHash: "same-hash", ArchiveToken: "archive-a", Name: "Job A", PresetID: "product-main", ProductIDs: []string{"product-a"}, Status: model.BatchProductionStatusQueued, CreatedBy: "user-a", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
	estimates := map[string]int{"product-a\x00": 1}
	saved, err := CreateBatchProductionJob(first, estimates)
	if err != nil {
		t.Fatalf("create first job: %v", err)
	}
	replay := first
	replay.ID, replay.ArchiveToken = "job-replayed", "archive-replayed"
	replayed, err := CreateBatchProductionJob(replay, estimates)
	if err != nil {
		t.Fatalf("replay job: %v", err)
	}
	if saved.ID != "job-a" || replayed.ID != saved.ID {
		t.Fatalf("idempotent replay returned jobs %q and %q", saved.ID, replayed.ID)
	}
	conflict := replay
	conflict.ID, conflict.RequestHash = "job-conflict", "different-hash"
	if _, err := CreateBatchProductionJob(conflict, estimates); !errors.Is(err, ErrBatchProductionRequestConflict) {
		t.Fatalf("conflicting replay error = %v, want request conflict", err)
	}
	var jobs, items, snapshots int64
	if err := testDB.Model(&model.BatchProductionJob{}).Where("organization_id = ?", "organization-a").Count(&jobs).Error; err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if err := testDB.Model(&model.BatchProductionItem{}).Where("organization_id = ?", "organization-a").Count(&items).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if err := testDB.Model(&model.BatchProductionSnapshot{}).Where("organization_id = ?", "organization-a").Count(&snapshots).Error; err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if jobs != 1 || items != 1 || snapshots != 1 {
		t.Fatalf("idempotent create stored jobs=%d items=%d snapshots=%d", jobs, items, snapshots)
	}
	var organization model.Organization
	if err := testDB.First(&organization, "id = ?", "organization-a").Error; err != nil {
		t.Fatalf("load organization credits: %v", err)
	}
	if saved.EstimatedCredits != 1 || saved.ReservedCredits != 1 || saved.CreditSource != model.CreditSourceOrganization || organization.ReservedCredits != 1 {
		t.Fatalf("unexpected batch credit reservation: job=%#v organization=%#v", saved, organization)
	}
}

func TestFailExhaustedBatchProductionItemsStopsAfterFiveLeases(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")
	createBatchTestJob(t, testDB, "organization-a", "job-a", model.BatchProductionStatusRunning, model.BatchProductionStatusRunning)
	if err := testDB.Model(&model.BatchProductionItem{}).Where("job_id = ?", "job-a").Updates(map[string]any{"attempts": 5, "lease_expires_at": batchTestExpired}).Error; err != nil {
		t.Fatalf("exhaust item leases: %v", err)
	}
	if _, _, claimed, err := ClaimNextBatchProductionItem(workspaceTestNow, workspaceTestFuture, "lease-new", 1); err != nil || claimed {
		t.Fatalf("exhausted item claim, claimed=%v err=%v", claimed, err)
	}
	var item model.BatchProductionItem
	if err := testDB.First(&item, "job_id = ?", "job-a").Error; err != nil {
		t.Fatalf("load exhausted item: %v", err)
	}
	var job model.BatchProductionJob
	if err := testDB.First(&job, "id = ?", "job-a").Error; err != nil {
		t.Fatalf("load exhausted job: %v", err)
	}
	if item.Status != model.BatchProductionStatusFailed || job.Status != model.BatchProductionStatusFailed || job.FailedItems != 1 {
		t.Fatalf("unexpected exhausted state: job=%#v item=%#v", job, item)
	}
}
