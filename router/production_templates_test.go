package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func TestProductionTemplateHTTPVersionsPreviewAndBatchSnapshot(t *testing.T) {
	tenant := seedRouterTestTenant(t, "production-template")
	database, err := repository.DB()
	if err != nil { t.Fatal(err) }
	brand := model.Brand{ID: "brand-router-template", OrganizationID: tenant.Organization.ID, Name: "路由品牌", Tone: "克制专业", Guidelines: "使用暖色柔光", ProhibitedTerms: []string{"夸大宣传"}, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	product := model.Product{ID: "product-router-template", OrganizationID: tenant.Organization.ID, BrandID: brand.ID, Code: "router-template-product", Name: "路由商品", SellingPoints: []string{"轻量", "耐用"}, Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	sku := model.ProductSKU{ID: "sku-router-template", OrganizationID: tenant.Organization.ID, ProductID: product.ID, Code: "router-template-sku", Name: "红色款", Attributes: map[string]string{"颜色": "红色"}, Status: model.ProductStatusActive, Version: 1, CreatedBy: tenant.User.ID, CreatedAt: "1", UpdatedAt: "1"}
	if err := database.Create(&brand).Error; err != nil { t.Fatal(err) }
	if err := database.Create(&product).Error; err != nil { t.Fatal(err) }
	if err := database.Create(&sku).Error; err != nil { t.Fatal(err) }

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	createdResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/production-templates", model.SaveProductionTemplateInput{Name: "企业棚拍", Description: "统一棚拍语言", Status: model.ProductionTemplateStatusActive, Prompt: "模板版本一"}, headers)
	var template model.ProductionTemplate
	if createdResponse.Code != 0 || json.Unmarshal(createdResponse.Data, &template) != nil || template.CurrentVersion != 1 || template.Version != 1 || template.CurrentPrompt != "模板版本一" { t.Fatalf("create template response: %#v, template=%#v", createdResponse, template) }
	updatedResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/production-templates", model.SaveProductionTemplateInput{ID: template.ID, Name: template.Name, Description: template.Description, Status: model.ProductionTemplateStatusActive, Prompt: "模板版本二", Version: template.Version}, headers)
	var updated model.ProductionTemplate
	if updatedResponse.Code != 0 || json.Unmarshal(updatedResponse.Data, &updated) != nil || updated.CurrentVersion != 2 || updated.Version != 2 || updated.CurrentPrompt != "模板版本二" { t.Fatalf("update template response: %#v, template=%#v", updatedResponse, updated) }
	if stale := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/production-templates", model.SaveProductionTemplateInput{ID: template.ID, Name: template.Name, Status: model.ProductionTemplateStatusActive, Prompt: "过期覆盖", Version: 1}, headers); stale.Code != 1 { t.Fatalf("stale template response: %#v", stale) }
	versionsResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/commerce/production-templates/"+template.ID+"/versions", nil, headers)
	var versions []model.ProductionTemplateVersion
	if versionsResponse.Code != 0 || json.Unmarshal(versionsResponse.Data, &versions) != nil || len(versions) != 2 || versions[0].Version != 2 || versions[1].Prompt != "模板版本一" { t.Fatalf("template versions response: %#v, versions=%#v", versionsResponse, versions) }

	previewResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/production-templates/preview", model.PreviewProductionPromptInput{PresetID: template.ID, PresetVersion: 2, DeliverySpecID: "douyin-product", BrandID: brand.ID, ProductID: product.ID, SKUID: sku.ID}, headers)
	var preview model.ProductionPromptPreview
	if previewResponse.Code != 0 || json.Unmarshal(previewResponse.Data, &preview) != nil { t.Fatalf("template preview response: %#v", previewResponse) }
	for _, expected := range []string{"模板版本二", "品牌语气：克制专业", "品牌规范：使用暖色柔光", "禁止在画面中出现的文字或概念：夸大宣传", "核心卖点：轻量；耐用", "SKU 属性：颜色=红色", "目标渠道：抖音 商品竖图", "交付画幅：1080×1440"} {
		if !strings.Contains(preview.Prompt, expected) { t.Fatalf("preview missing %q: %s", expected, preview.Prompt) }
	}

	batchResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/commerce/batch-jobs", model.CreateBatchProductionJobInput{RequestID: "router-template-batch", Name: "模板冻结任务", PresetID: template.ID, PresetVersion: 1, DeliverySpecID: "taobao-main", BrandID: brand.ID, ProductIDs: []string{product.ID}}, headers)
	var job model.BatchProductionJob
	if batchResponse.Code != 0 || json.Unmarshal(batchResponse.Data, &job) != nil { t.Fatalf("template batch response: %#v", batchResponse) }
	var savedJob model.BatchProductionJob
	if err := database.First(&savedJob, "id = ?", job.ID).Error; err != nil { t.Fatal(err) }
	if savedJob.PresetVersion != 1 || savedJob.PresetPrompt != "模板版本一" || savedJob.DeliverySpec.ID != "taobao-main" || savedJob.DeliverySpec.Width != 800 || savedJob.DeliverySpec.Height != 800 { t.Fatalf("batch template or delivery spec was not frozen: %#v", savedJob) }

	reviewer := model.User{ID: "user-router-template-reviewer", Username: "router-template-reviewer", Password: tenant.User.Password, OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-template-reviewer", Status: model.UserStatusActive, CreatedAt: "3", UpdatedAt: "3"}
	if err := database.Create(&reviewer).Error; err != nil { t.Fatal(err) }
	if err := database.Create(&model.OrganizationMember{ID: "member-router-template-reviewer", OrganizationID: tenant.Organization.ID, UserID: reviewer.ID, Role: model.OrganizationRoleReviewer, Version: 1, CreatedAt: "3", UpdatedAt: "3"}).Error; err != nil { t.Fatal(err) }
	reviewerClient, reviewerURL := loginRouterTestClient(t, reviewer.Username)
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, reviewerURL+"/api/commerce/production-templates/preview", model.PreviewProductionPromptInput{PresetID: template.ID, PresetVersion: 2, ProductID: product.ID}, headers); response.Code != 0 { t.Fatalf("reviewer preview response: %#v", response) }
	if response := routerTestJSON(t, reviewerClient, http.MethodPost, reviewerURL+"/api/commerce/production-templates", model.SaveProductionTemplateInput{Name: "越权模板", Status: model.ProductionTemplateStatusActive, Prompt: "越权"}, headers); response.Code != 1 { t.Fatalf("reviewer template write response: %#v", response) }
}
