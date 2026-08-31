package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
	"github.com/yypyyd/infinite-canvas/service"
)

type pricingRouteResponse struct {
	Group      string  `json:"group"`
	GroupRatio float64 `json:"groupRatio"`
	Items      []struct {
		Model          string  `json:"model"`
		EffectiveRatio float64 `json:"effectiveRatio"`
		Source         string  `json:"source"`
	} `json:"items"`
}

func TestUserPricingRoutesExposeOnlyEffectiveExactSpecPricing(t *testing.T) {
	tenant := seedRouterTestTenant(t, "pricing")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Model(&model.User{}).Where("id = ?", tenant.User.ID).Updates(map[string]any{"group": "vip", "role": model.UserRoleAdmin}).Error; err != nil {
		t.Fatal(err)
	}
	savedSettings, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := savedSettings
	settings.Public.ModelChannel.GroupRatios = map[string]float64{"default": 1, "vip": 0.8}
	settings.Public.ModelChannel.PricingRules = []model.PricingRule{{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Credits: 10, Enabled: true}}
	if _, err := repository.SaveSettings(settings, "router-pricing-settings"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(savedSettings, "router-pricing-cleanup") })

	client, baseURL := loginRouterTestClient(t, tenant.User.Username)
	adminResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/admin/users/"+tenant.User.ID+"/pricing-discounts", map[string]any{"items": []map[string]any{{"model": "image-model", "modality": "image", "operation": "generation", "unit": "image", "resolutionTier": "1k", "ratio": 0.5, "remark": "专属优惠"}}}, nil)
	if adminResponse.Code != 0 {
		t.Fatalf("save pricing response: %#v", adminResponse)
	}
	var saved []model.UserPricingDiscount
	if err := json.Unmarshal(adminResponse.Data, &saved); err != nil || len(saved) != 1 || saved[0].ID == "" || saved[0].Ratio != 0.5 {
		t.Fatalf("unexpected saved pricing: %#v, err=%v", saved, err)
	}
	adminListResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/admin/users/"+tenant.User.ID+"/pricing-discounts", nil, nil)
	var listed []model.UserPricingDiscount
	if adminListResponse.Code != 0 || json.Unmarshal(adminListResponse.Data, &listed) != nil || len(listed) != 1 || listed[0].ID != saved[0].ID {
		t.Fatalf("unexpected pricing list response: %#v, items=%#v", adminListResponse, listed)
	}
	invalidResponse := routerTestJSON(t, client, http.MethodPost, baseURL+"/api/admin/users/"+tenant.User.ID+"/pricing-discounts", map[string]any{"items": []map[string]any{{"model": "image-model", "modality": "image", "operation": "generation", "unit": "image", "resolutionTier": "2k", "ratio": 0.4}}}, nil)
	if invalidResponse.Code != 1 {
		t.Fatalf("mismatched global pricing spec should be rejected: %#v", invalidResponse)
	}

	pricingResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/v1/pricing", nil, map[string]string{"X-Organization-ID": tenant.Organization.ID})
	if pricingResponse.Code != 0 {
		t.Fatalf("pricing response: %#v", pricingResponse)
	}
	var pricing pricingRouteResponse
	if err := json.Unmarshal(pricingResponse.Data, &pricing); err != nil {
		t.Fatal(err)
	}
	if pricing.Group != "vip" || pricing.GroupRatio != 0.8 || len(pricing.Items) != 1 || pricing.Items[0].Model != "image-model" || pricing.Items[0].EffectiveRatio != 0.5 || pricing.Items[0].Source != "user_spec" {
		t.Fatalf("unexpected effective pricing: %#v", pricing)
	}
	serialized := string(pricingResponse.Data)
	if strings.Contains(serialized, "snapshot") || strings.Contains(serialized, "credits") || strings.Contains(serialized, "userSpecRatio") {
		t.Fatalf("pricing endpoint leaked internal pricing details: %s", serialized)
	}

	authUser := model.PublicUser(tenant.User)
	authUser.OrganizationID = tenant.Organization.ID
	credential, err := service.CreateUserAPIKey(authUser, "pricing-test")
	if err != nil {
		t.Fatal(err)
	}
	apiKeyResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/v1/pricing", nil, map[string]string{"Authorization": "Bearer " + credential.Secret})
	var apiKeyPricing pricingRouteResponse
	if apiKeyResponse.Code != 0 || json.Unmarshal(apiKeyResponse.Data, &apiKeyPricing) != nil || apiKeyPricing.Group != pricing.Group || apiKeyPricing.GroupRatio != pricing.GroupRatio || len(apiKeyPricing.Items) != len(pricing.Items) || apiKeyPricing.Items[0].EffectiveRatio != pricing.Items[0].EffectiveRatio || apiKeyPricing.Items[0].Source != pricing.Items[0].Source {
		t.Fatalf("API Key pricing differs from session pricing: response=%#v pricing=%#v", apiKeyResponse, apiKeyPricing)
	}
	forgedResponse := routerTestJSON(t, client, http.MethodGet, baseURL+"/api/v1/pricing", nil, map[string]string{"Authorization": "Bearer " + credential.Secret, "X-Organization-ID": tenant.Foreign.ID})
	if forgedResponse.Code != 1 {
		t.Fatalf("API Key should not read pricing through a forged organization: %#v", forgedResponse)
	}
}
