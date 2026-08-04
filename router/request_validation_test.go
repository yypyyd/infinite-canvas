package router

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestCommerceHTTPRejectsInvalidAndOversizedRequestsWithoutWrites(t *testing.T) {
	tenant := seedRouterTestTenant(t, "request-validation")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	product := model.Product{
		ID: "product-router-request-validation", OrganizationID: tenant.Organization.ID,
		Code: "router-request-validation", Name: "Validation Product", Status: model.ProductStatusActive,
		Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1",
	}
	if err := database.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	imageKeys := make([]string, 51)
	for index := range imageKeys {
		imageKeys[index] = fmt.Sprintf("image:validation-%d", index)
	}
	productIDs := make([]string, 201)
	for index := range productIDs {
		productIDs[index] = fmt.Sprintf("product-validation-%d", index)
	}
	requests := []struct {
		path string
		body any
	}{
		{path: "/api/commerce/brands", body: map[string]any{"name": "Unknown Field", "unexpected": true}},
		{path: "/api/commerce/brands", body: model.Brand{Name: "Oversized Brand", Guidelines: strings.Repeat("x", 70<<10)}},
		{path: "/api/commerce/products", body: model.Product{Code: "invalid-status", Name: "Invalid Status", Status: "unknown"}},
		{path: "/api/commerce/skus", body: model.ProductSKU{ProductID: product.ID, Code: "too-many-images", Name: "Too Many Images", ImageStorageKeys: imageKeys}},
		{path: "/api/commerce/batch-jobs", body: model.CreateBatchProductionJobInput{RequestID: "too-many-products", Name: "Too Many Products", PresetID: "product-main", ProductIDs: productIDs}},
		{path: "/api/commerce/brands", body: model.Brand{Name: "Request Too Large", Guidelines: strings.Repeat("x", 1<<20)}},
	}
	for _, request := range requests {
		if response := routerTestJSON(t, client, http.MethodPost, baseURL+request.path, request.body, headers); response.Code != 1 {
			t.Fatalf("invalid request %s response: %#v", request.path, response)
		}
	}
	checks := []struct {
		model any
		want  int64
	}{
		{model: &model.Brand{}, want: 0},
		{model: &model.Product{}, want: 1},
		{model: &model.ProductSKU{}, want: 0},
		{model: &model.BatchProductionJob{}, want: 0},
	}
	for _, check := range checks {
		var count int64
		if err := database.Model(check.model).Where("organization_id = ?", tenant.Organization.ID).Count(&count).Error; err != nil || count != check.want {
			t.Fatalf("unexpected persisted count for %T: count=%d want=%d err=%v", check.model, count, check.want, err)
		}
	}
}
