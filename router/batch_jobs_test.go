package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func prepareRouterLegacyBatchBilling(t *testing.T, tenant routerTestTenant) {
	t.Helper()
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := saved
	settings.Public.ModelChannel = model.PublicModelChannelSetting{
		AvailableModels: []string{"router-batch-image"},
		Models:          []model.ModelDefinition{{ID: "router-batch-image", Name: "Router Batch Image", Modality: "image", Operations: []string{"generation", "edit"}, Enabled: true, ResolutionTiers: []string{"1k"}}},
		PricingRules: []model.PricingRule{
			{Model: "router-batch-image", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", BillingMode: "fixed", Credits: 1, Enabled: true},
			{Model: "router-batch-image", Modality: "image", Operation: "edit", Unit: "image", ResolutionTier: "1k", BillingMode: "fixed", Credits: 1, Enabled: true},
		},
		DefaultImageModel: "router-batch-image",
	}
	if _, err := repository.SaveSettings(settings, "router-batch-billing"); err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.User{}).Where("id = ?", tenant.User.ID).Update("credits", 100).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, "router-batch-billing-cleanup") })
}

func TestBatchJobHTTPCreateIsIdempotentAndTenantScoped(t *testing.T) {
	tenant := seedRouterTestTenant(t, "batch-create")
	prepareRouterLegacyBatchBilling(t, tenant)
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	brand := model.Brand{ID: "brand-router-batch-create", OrganizationID: tenant.Organization.ID, Name: "Router Brand", ProhibitedTerms: []string{"forbidden"}, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	product := model.Product{ID: "product-router-batch-create", OrganizationID: tenant.Organization.ID, Code: "router-batch-create", Name: "Router Product", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	if err := database.Create(&brand).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&product).Error; err != nil {
		t.Fatal(err)
	}

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	primary := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	secondary := map[string]string{"X-Organization-ID": tenant.Secondary.ID}
	input := model.CreateBatchProductionJobInput{RequestID: "router-batch-create", Name: "Router Batch Job", BrandID: brand.ID, PresetID: "product-main", ProductIDs: []string{product.ID}}
	created := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs", input, primary)
	if created.Code != 0 {
		t.Fatalf("create batch job response: %#v", created)
	}
	var job model.BatchProductionJob
	if err := json.Unmarshal(created.Data, &job); err != nil || job.ID == "" || job.TotalItems != 1 {
		t.Fatalf("unexpected batch job: %#v, err=%v", job, err)
	}

	replayed := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs", input, primary)
	var replayedJob model.BatchProductionJob
	if replayed.Code != 0 || json.Unmarshal(replayed.Data, &replayedJob) != nil || replayedJob.ID != job.ID {
		t.Fatalf("unexpected idempotent replay: %#v, job=%#v", replayed, replayedJob)
	}
	input.Name = "Conflicting Batch Job"
	if conflict := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs", input, primary); conflict.Code != 1 {
		t.Fatalf("request id conflict response: %#v", conflict)
	}

	primaryList := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/batch-jobs", nil, primary)
	var primaryJobs model.BatchProductionJobList
	if primaryList.Code != 0 || json.Unmarshal(primaryList.Data, &primaryJobs) != nil || primaryJobs.Total != 1 || len(primaryJobs.Items) != 1 || primaryJobs.Items[0].ID != job.ID {
		t.Fatalf("unexpected primary job list: %#v, list=%#v", primaryList, primaryJobs)
	}
	secondaryList := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/batch-jobs", nil, secondary)
	var secondaryJobs model.BatchProductionJobList
	if secondaryList.Code != 0 || json.Unmarshal(secondaryList.Data, &secondaryJobs) != nil || secondaryJobs.Total != 0 || len(secondaryJobs.Items) != 0 {
		t.Fatalf("secondary organization leaked jobs: %#v, list=%#v", secondaryList, secondaryJobs)
	}
	secondaryItems := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/batch-jobs/"+job.ID+"/items", nil, secondary)
	var itemList model.BatchProductionItemList
	if secondaryItems.Code != 0 || json.Unmarshal(secondaryItems.Data, &itemList) != nil || itemList.Total != 0 || len(itemList.Items) != 0 {
		t.Fatalf("secondary organization leaked items: %#v, list=%#v", secondaryItems, itemList)
	}
	resultFile := model.UserFile{ID: "file-router-batch-create", OrganizationID: tenant.Organization.ID, UserID: tenant.User.ID, StorageKey: "image:router-batch-create", ObjectKey: "organizations/" + tenant.Organization.ID + "/files/router-batch-create.jpg", Hash: "hash-router-batch-create", MimeType: "image/jpeg", Size: 42, CreatedAt: "2", UpdatedAt: "2"}
	if err := database.Create(&resultFile).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ?", tenant.Organization.ID, job.ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "result_storage_key": resultFile.StorageKey}).Error; err != nil {
		t.Fatal(err)
	}
	primaryItems := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/batch-jobs/"+job.ID+"/items", nil, primary)
	if primaryItems.Code != 0 || json.Unmarshal(primaryItems.Data, &itemList) != nil || len(itemList.Items) != 1 || itemList.Items[0].QualityContext == nil || itemList.Items[0].QualityContext.Product.ID != product.ID || itemList.Items[0].QualityContext.Brand == nil || itemList.Items[0].QualityContext.Brand.ID != brand.ID || itemList.Items[0].ResultMimeType != resultFile.MimeType || itemList.Items[0].ResultSize != resultFile.Size {
		t.Fatalf("batch item quality context missing: %#v, list=%#v", primaryItems, itemList)
	}

	counts := []struct {
		name  string
		model any
		query string
		want  int64
	}{
		{name: "jobs", model: &model.BatchProductionJob{}, query: "organization_id = ? AND id = ?", want: 1},
		{name: "items", model: &model.BatchProductionItem{}, query: "organization_id = ? AND job_id = ?", want: 1},
		{name: "snapshots", model: &model.BatchProductionSnapshot{}, query: "organization_id = ? AND job_id = ?", want: 2},
	}
	for _, check := range counts {
		var count int64
		if err := database.Model(check.model).Where(check.query, tenant.Organization.ID, job.ID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, count, check.want)
		}
	}
}

func TestBatchJobHTTPCancelAndRetryRespectTenantAndState(t *testing.T) {
	tenant := seedRouterTestTenant(t, "batch-state")
	prepareRouterLegacyBatchBilling(t, tenant)
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	products := []model.Product{
		{ID: "product-router-batch-state-a", OrganizationID: tenant.Organization.ID, Code: "router-batch-state-a", Name: "Router Product A", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "product-router-batch-state-b", OrganizationID: tenant.Organization.ID, Code: "router-batch-state-b", Name: "Router Product B", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "2", UpdatedAt: "2"},
	}
	if err := database.Create(&products).Error; err != nil {
		t.Fatal(err)
	}

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	primary := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	secondary := map[string]string{"X-Organization-ID": tenant.Secondary.ID}
	create := func(requestID string, productIDs []string) model.BatchProductionJob {
		t.Helper()
		response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs", model.CreateBatchProductionJobInput{RequestID: requestID, Name: requestID, PresetID: "product-main", ProductIDs: productIDs}, primary)
		var job model.BatchProductionJob
		if response.Code != 0 || json.Unmarshal(response.Data, &job) != nil {
			t.Fatalf("create %s response: %#v", requestID, response)
		}
		return job
	}

	cancelJob := create("router-batch-cancel", []string{products[0].ID})
	if err := database.Model(&model.BatchProductionJob{}).Where("id = ?", cancelJob.ID).Update("status", model.BatchProductionStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.BatchProductionItem{}).Where("job_id = ?", cancelJob.ID).Updates(map[string]any{"status": model.BatchProductionStatusRunning, "lease_token": "foreign-lease", "lease_expires_at": "9", "locked_at": "8", "started_at": "8"}).Error; err != nil {
		t.Fatal(err)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+cancelJob.ID+"/cancel", nil, secondary); response.Code != 1 {
		t.Fatalf("cross-organization cancel response: %#v", response)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+cancelJob.ID+"/cancel", nil, primary); response.Code != 0 {
		t.Fatalf("cancel response: %#v", response)
	}
	var cancelledJob model.BatchProductionJob
	var cancelledItem model.BatchProductionItem
	if err := database.First(&cancelledJob, "id = ?", cancelJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&cancelledItem, "job_id = ?", cancelJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if cancelledJob.Status != model.BatchProductionStatusCancelled || cancelledItem.Status != model.BatchProductionStatusCancelled || cancelledItem.LeaseToken != "" || cancelledItem.LeaseExpiresAt != "" || cancelledItem.LockedAt != "" || cancelledItem.FinishedAt == "" {
		t.Fatalf("unexpected cancelled state: job=%#v, item=%#v", cancelledJob, cancelledItem)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+cancelJob.ID+"/cancel", nil, primary); response.Code != 1 {
		t.Fatalf("repeated cancel response: %#v", response)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+cancelJob.ID+"/retry", nil, primary); response.Code != 1 {
		t.Fatalf("cancelled job retry response: %#v", response)
	}

	retryJob := create("router-batch-retry", []string{products[0].ID, products[1].ID})
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+retryJob.ID+"/retry", nil, secondary); response.Code != 1 {
		t.Fatalf("cross-organization retry response: %#v", response)
	}
	var retryItems []model.BatchProductionItem
	if err := database.Where("job_id = ?", retryJob.ID).Order("product_id asc").Find(&retryItems).Error; err != nil || len(retryItems) != 2 {
		t.Fatalf("retry items: %#v, err=%v", retryItems, err)
	}
	if err := database.Model(&model.BatchProductionItem{}).Where("id = ?", retryItems[0].ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "attempts": 1, "result_storage_key": "image:completed-result", "started_at": "3", "finished_at": "4"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.BatchProductionItem{}).Where("id = ?", retryItems[1].ID).Updates(map[string]any{"status": model.BatchProductionStatusFailed, "attempts": 5, "error_message": "failed", "lease_token": "expired", "lease_expires_at": "5", "locked_at": "3", "started_at": "3", "finished_at": "5"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.BatchProductionJob{}).Where("id = ?", retryJob.ID).Updates(map[string]any{"status": model.BatchProductionStatusFailed, "completed_items": 1, "failed_items": 1}).Error; err != nil {
		t.Fatal(err)
	}
	if response := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs/"+retryJob.ID+"/retry", nil, primary); response.Code != 0 {
		t.Fatalf("retry response: %#v", response)
	}
	var savedJob model.BatchProductionJob
	if err := database.First(&savedJob, "id = ?", retryJob.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Where("job_id = ?", retryJob.ID).Order("product_id asc").Find(&retryItems).Error; err != nil {
		t.Fatal(err)
	}
	if savedJob.Status != model.BatchProductionStatusRunning || savedJob.FailedItems != 0 || savedJob.CompletedItems != 1 {
		t.Fatalf("unexpected retried job: %#v", savedJob)
	}
	if retryItems[0].Status != model.BatchProductionStatusCompleted || retryItems[0].RunNumber != 1 || retryItems[0].ResultStorageKey != "image:completed-result" {
		t.Fatalf("completed item changed during retry: %#v", retryItems[0])
	}
	if retryItems[1].Status != model.BatchProductionStatusQueued || retryItems[1].RunNumber != 2 || retryItems[1].Attempts != 0 || retryItems[1].ErrorMessage != "" || retryItems[1].LeaseToken != "" || retryItems[1].LeaseExpiresAt != "" || retryItems[1].LockedAt != "" || retryItems[1].StartedAt != "" || retryItems[1].FinishedAt != "" {
		t.Fatalf("failed item was not reset for a new run: %#v", retryItems[1])
	}
}

func TestBatchItemHTTPReviewRetryAndPrimaryWorkflow(t *testing.T) {
	tenant := seedRouterTestTenant(t, "batch-review")
	prepareRouterLegacyBatchBilling(t, tenant)
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	product := model.Product{ID: "product-router-batch-review", OrganizationID: tenant.Organization.ID, Code: "router-batch-review", Name: "Router Review Product", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	skus := []model.ProductSKU{
		{ID: "sku-router-batch-review-a", OrganizationID: tenant.Organization.ID, ProductID: product.ID, Code: "router-batch-review-a", Name: "Review SKU A", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "sku-router-batch-review-b", OrganizationID: tenant.Organization.ID, ProductID: product.ID, Code: "router-batch-review-b", Name: "Review SKU B", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "2", UpdatedAt: "2"},
	}
	if err := database.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&skus).Error; err != nil {
		t.Fatal(err)
	}
	reviewer := model.User{ID: "user-router-batch-reviewer", Username: "router-batch-reviewer", Password: tenant.User.Password, OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-batch-reviewer", Status: model.UserStatusActive, CreatedAt: "3", UpdatedAt: "3"}
	if err := database.Create(&reviewer).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.OrganizationMember{ID: "member-router-batch-reviewer", OrganizationID: tenant.Organization.ID, UserID: reviewer.ID, Role: model.OrganizationRoleReviewer, Version: 1, CreatedAt: "3", UpdatedAt: "3"}).Error; err != nil {
		t.Fatal(err)
	}
	member := model.User{ID: "user-router-batch-member", Username: "router-batch-member", Password: tenant.User.Password, OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-batch-member", Status: model.UserStatusActive, CreatedAt: "3", UpdatedAt: "3"}
	if err := database.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&model.OrganizationMember{ID: "member-router-batch-member", OrganizationID: tenant.Organization.ID, UserID: member.ID, Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "3", UpdatedAt: "3"}).Error; err != nil {
		t.Fatal(err)
	}

	ownerClient, ownerURL := loginRouterTestClient(t, tenant.User.Username)
	reviewerClient, reviewerURL := loginRouterTestClient(t, reviewer.Username)
	memberClient, memberURL := loginRouterTestClient(t, member.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	created := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/batch-jobs", model.CreateBatchProductionJobInput{RequestID: "router-batch-review", Name: "Review Batch", PresetID: "product-main", ProductIDs: []string{product.ID}}, headers)
	var job model.BatchProductionJob
	if created.Code != 0 || json.Unmarshal(created.Data, &job) != nil {
		t.Fatalf("create review job response: %#v", created)
	}
	var items []model.BatchProductionItem
	if err := database.Where("job_id = ?", job.ID).Order("sku_id asc").Find(&items).Error; err != nil || len(items) != 2 {
		t.Fatalf("review items: %#v, err=%v", items, err)
	}
	for index := range items {
		if err := database.Model(&model.BatchProductionItem{}).Where("id = ?", items[index].ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "result_storage_key": "image:router-batch-review-" + items[index].ID, "review_status": model.BatchProductionReviewPending, "attempts": 1, "finished_at": "4"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := database.Model(&model.BatchProductionJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "completed_items": 2}).Error; err != nil {
		t.Fatal(err)
	}

	itemURL := func(baseURL string, item model.BatchProductionItem, action string) string {
		return baseURL + "/api/commerce/batch-jobs/" + job.ID + "/items/" + item.ID + "/" + action
	}
	if response := routerTestJSON(t, memberClient, http.MethodPost, itemURL(memberURL, items[0], "review"), map[string]any{"runNumber": 1, "status": model.BatchProductionReviewApproved, "comment": "越权审核"}, headers); response.Code != 1 {
		t.Fatalf("member review response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[1], "review"), map[string]any{"runNumber": 1, "status": model.BatchProductionReviewRejected, "comment": ""}, headers); response.Code != 1 {
		t.Fatalf("empty reject comment response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[0], "review"), map[string]any{"runNumber": 1, "status": model.BatchProductionReviewApproved, "comment": "可交付"}, headers); response.Code != 0 {
		t.Fatalf("approve response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[0], "primary"), map[string]int{"runNumber": 1}, headers); response.Code != 0 {
		t.Fatalf("primary response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[1], "review"), map[string]any{"runNumber": 1, "status": model.BatchProductionReviewRejected, "comment": "商品角度不正确"}, headers); response.Code != 0 {
		t.Fatalf("reject response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[1], "primary"), map[string]int{"runNumber": 1}, headers); response.Code != 1 {
		t.Fatalf("rejected primary response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[1], "retry"), map[string]int{"runNumber": 1}, headers); response.Code != 0 {
		t.Fatalf("rejected retry response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, items[1], "review"), map[string]any{"runNumber": 1, "status": model.BatchProductionReviewApproved, "comment": "旧页面"}, headers); response.Code != 1 {
		t.Fatalf("stale review response: %#v", response)
	}

	var retried model.BatchProductionItem
	if err := database.First(&retried, "id = ?", items[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if retried.Status != model.BatchProductionStatusQueued || retried.RunNumber != 2 || retried.ResultStorageKey != "" || retried.ReviewStatus != "" || retried.ReviewComment != "" {
		t.Fatalf("unexpected retried review item: %#v", retried)
	}
	if err := database.Model(&model.BatchProductionItem{}).Where("id = ?", retried.ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "result_storage_key": "image:router-batch-review-new", "review_status": model.BatchProductionReviewPending, "attempts": 1, "finished_at": "6"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.BatchProductionJob{}).Where("id = ?", job.ID).Updates(map[string]any{"status": model.BatchProductionStatusCompleted, "completed_items": 2}).Error; err != nil {
		t.Fatal(err)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, retried, "review"), map[string]any{"runNumber": 2, "status": model.BatchProductionReviewApproved, "comment": "重做通过"}, headers); response.Code != 0 {
		t.Fatalf("retried approve response: %#v", response)
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, itemURL(reviewerURL, retried, "primary"), map[string]int{"runNumber": 2}, headers); response.Code != 0 {
		t.Fatalf("replace primary response: %#v", response)
	}
	if err := database.Where("job_id = ?", job.ID).Order("sku_id asc").Find(&items).Error; err != nil {
		t.Fatal(err)
	}
	if !items[0].IsPrimary || !items[1].IsPrimary {
		t.Fatalf("SKU primaries were not independent: %#v", items)
	}
}
