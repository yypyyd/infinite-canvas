package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestCommerceCatalogHTTPIsTenantScopedAndReviewerReadOnly(t *testing.T) {
	tenant := seedRouterTestTenant(t, "catalog")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	brands := []model.Brand{
		{ID: "brand-router-catalog-a", OrganizationID: tenant.Organization.ID, Name: "Primary Brand", Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "brand-router-catalog-b", OrganizationID: tenant.Secondary.ID, Name: "Foreign Secret Brand", Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "2", UpdatedAt: "2"},
	}
	products := []model.Product{
		{ID: "product-router-catalog-a", OrganizationID: tenant.Organization.ID, BrandID: brands[0].ID, Code: "router-catalog-a", Name: "Primary Product", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "product-router-catalog-b", OrganizationID: tenant.Secondary.ID, BrandID: brands[1].ID, Code: "router-catalog-b", Name: "Foreign Secret Product", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "2", UpdatedAt: "2"},
	}
	skus := []model.ProductSKU{
		{ID: "sku-router-catalog-a", OrganizationID: tenant.Organization.ID, ProductID: products[0].ID, Code: "router-sku-a", Name: "Primary SKU", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"},
		{ID: "sku-router-catalog-b", OrganizationID: tenant.Secondary.ID, ProductID: products[1].ID, Code: "router-sku-b", Name: "Foreign Secret SKU", Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "2", UpdatedAt: "2"},
	}
	if err := database.Create(&brands).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&products).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&skus).Error; err != nil {
		t.Fatal(err)
	}

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	primary := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	secondary := map[string]string{"X-Organization-ID": tenant.Secondary.ID}
	brandResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/brands", nil, primary)
	var brandList model.BrandList
	if brandResponse.Code != 0 || json.Unmarshal(brandResponse.Data, &brandList) != nil || brandList.Total != 1 || len(brandList.Items) != 1 || brandList.Items[0].ID != brands[0].ID {
		t.Fatalf("unexpected primary brands: %#v, list=%#v", brandResponse, brandList)
	}
	brandResponse = routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/brands", nil, secondary)
	if brandResponse.Code != 0 || json.Unmarshal(brandResponse.Data, &brandList) != nil || brandList.Total != 1 || len(brandList.Items) != 1 || brandList.Items[0].ID != brands[1].ID {
		t.Fatalf("unexpected secondary brands: %#v, list=%#v", brandResponse, brandList)
	}

	productResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/products", nil, primary)
	var productList model.ProductList
	if productResponse.Code != 0 || json.Unmarshal(productResponse.Data, &productList) != nil || productList.Total != 1 || len(productList.Items) != 1 || productList.Items[0].ID != products[0].ID {
		t.Fatalf("unexpected primary products: %#v, list=%#v", productResponse, productList)
	}
	filtered := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/products?keyword=Foreign%20Secret", nil, primary)
	if filtered.Code != 0 || json.Unmarshal(filtered.Data, &productList) != nil || productList.Total != 0 || len(productList.Items) != 0 {
		t.Fatalf("product keyword bypassed tenant scope: %#v, list=%#v", filtered, productList)
	}

	skuResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/products/"+products[0].ID+"/skus", nil, primary)
	var skuList model.ProductSKUList
	if skuResponse.Code != 0 || json.Unmarshal(skuResponse.Data, &skuList) != nil || skuList.Total != 1 || len(skuList.Items) != 1 || skuList.Items[0].ID != skus[0].ID {
		t.Fatalf("unexpected primary skus: %#v, list=%#v", skuResponse, skuList)
	}
	if response := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/products/"+products[0].ID+"/skus", nil, secondary); response.Code != 1 {
		t.Fatalf("cross-organization sku response: %#v", response)
	}
	foreignWrites := []struct {
		path string
		body any
	}{
		{path: "/api/commerce/brands", body: brands[1]},
		{path: "/api/commerce/products", body: products[1]},
		{path: "/api/commerce/skus", body: skus[1]},
	}
	for _, request := range foreignWrites {
		if response := routerTestJSON(t, client, http.MethodPost, baseURL+request.path, request.body, primary); response.Code != 1 {
			t.Fatalf("cross-organization write %s response: %#v", request.path, response)
		}
	}

	reviewer := model.User{
		ID: "user-router-catalog-reviewer", Username: "router-catalog-reviewer", Password: tenant.User.Password,
		OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default",
		AffCode: "aff-router-catalog-reviewer", Status: model.UserStatusActive, CreatedAt: "3", UpdatedAt: "3",
	}
	if err := database.Create(&reviewer).Error; err != nil {
		t.Fatal(err)
	}
	membership := model.OrganizationMember{
		ID: "member-router-catalog-reviewer", OrganizationID: tenant.Organization.ID, UserID: reviewer.ID,
		Role: model.OrganizationRoleReviewer, Version: 1, CreatedAt: "3", UpdatedAt: "3",
	}
	if err := database.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	reviewerClient, reviewerURL := loginRouterTestClient(t, reviewer.Username)
	if response := routerTestJSON(t, reviewerClient, http.MethodGet, reviewerURL+"/api/commerce/products", nil, primary); response.Code != 0 {
		t.Fatalf("reviewer product read response: %#v", response)
	}
	reviewerWrites := []struct {
		path string
		body any
	}{
		{path: "/api/commerce/brands", body: model.Brand{Name: "Reviewer Brand"}},
		{path: "/api/commerce/products", body: model.Product{Code: "reviewer-product", Name: "Reviewer Product"}},
		{path: "/api/commerce/skus", body: model.ProductSKU{ProductID: products[0].ID, Code: "reviewer-sku", Name: "Reviewer SKU"}},
		{path: "/api/commerce/batch-jobs", body: model.CreateBatchProductionJobInput{RequestID: "reviewer-batch", Name: "Reviewer Batch", PresetID: "product-main", ProductIDs: []string{products[0].ID}}},
	}
	for _, request := range reviewerWrites {
		if response := routerTestJSON(t, reviewerClient, http.MethodPost, reviewerURL+request.path, request.body, primary); response.Code != 1 {
			t.Fatalf("reviewer write %s response: %#v", request.path, response)
		}
	}
	if response := routerTestJSON(t, reviewerClient, http.MethodGet, reviewerURL+"/api/commerce/audit-logs", nil, primary); response.Code != 1 {
		t.Fatalf("reviewer audit log response: %#v", response)
	}
}
