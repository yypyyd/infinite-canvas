package router

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestOrganizationCreditsHTTPSettingsAndTransfer(t *testing.T) {
	tenant := seedRouterTestTenant(t, "organization-credits")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	setRouterTestCredits(t, tenant.User.ID, 100)
	ownerClient, baseURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}

	settingsResponse := routerTestJSON(t, ownerClient, http.MethodPost, baseURL+"/api/commerce/organization-credit-settings", map[string]any{"mode": "shared", "monthlyBudget": 60, "alertThreshold": 50, "version": 1}, headers)
	var settings model.OrganizationCreditSummary
	if settingsResponse.Code != 0 || json.Unmarshal(settingsResponse.Data, &settings) != nil || settings.Mode != model.OrganizationCreditModeShared || settings.MonthlyBudget != 60 || settings.AlertThreshold != 50 {
		t.Fatalf("credit settings response: %#v summary=%#v", settingsResponse, settings)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, baseURL+"/api/commerce/organization-credit-settings", map[string]any{"mode": "personal", "monthlyBudget": 0, "alertThreshold": 80, "version": 1}, headers); response.Code != 1 {
		t.Fatalf("stale credit settings response: %#v", response)
	}

	transferResponse := routerTestJSON(t, ownerClient, http.MethodPost, baseURL+"/api/commerce/organization-credits/transfer", map[string]int{"amount": 30}, headers)
	var transferred model.OrganizationCreditSummary
	if transferResponse.Code != 0 || json.Unmarshal(transferResponse.Data, &transferred) != nil || transferred.Balance != 30 || transferred.PersonalBalance != 70 {
		t.Fatalf("credit transfer response: %#v summary=%#v", transferResponse, transferred)
	}
	var organization model.Organization
	var user model.User
	if err := database.First(&organization, "id = ?", tenant.Organization.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&user, "id = ?", tenant.User.ID).Error; err != nil {
		t.Fatal(err)
	}
	if organization.Credits != 30 || user.Credits != 70 {
		t.Fatalf("unexpected transfer balances: organization=%d user=%d", organization.Credits, user.Credits)
	}
	if err := database.Model(&model.Organization{}).Where("id = ?", tenant.Organization.ID).Updates(map[string]any{"monthly_credits_used": 30, "credit_budget_month": time.Now().UTC().Format("2006-01")}).Error; err != nil {
		t.Fatal(err)
	}
	workspaceResponse := routerTestJSON(t, ownerClient, http.MethodGet, baseURL+"/api/commerce/workspace", nil, headers)
	var workspace model.OrganizationWorkspace
	if workspaceResponse.Code != 0 || json.Unmarshal(workspaceResponse.Data, &workspace) != nil || !workspace.CreditSummary.Warning {
		t.Fatalf("credit warning response: %#v workspace=%#v", workspaceResponse, workspace)
	}
	var logs []model.CreditLog
	if err := database.Where("organization_id = ?", tenant.Organization.ID).Order("created_at asc, id asc").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].CreditSource == logs[1].CreditSource || logs[0].Balance == logs[1].Balance {
		t.Fatalf("unexpected transfer ledgers: %#v", logs)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, baseURL+"/api/commerce/organization-credits/transfer", map[string]int{"amount": 1000}, headers); response.Code != 1 {
		t.Fatalf("insufficient transfer response: %#v", response)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, baseURL+"/api/commerce/organization-credit-settings", map[string]any{"mode": "personal", "monthlyBudget": 0, "alertThreshold": 80, "version": 1}, map[string]string{"X-Organization-ID": tenant.Foreign.ID}); response.Code != 1 {
		t.Fatalf("foreign credit settings response: %#v", response)
	}

	memberUser := model.User{ID: "user-router-organization-credits-member", Username: "router-organization-credits-member", Password: tenant.User.Password, OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-organization-credits-member", Status: model.UserStatusActive, CreatedAt: "2", UpdatedAt: "2"}
	member := model.OrganizationMember{ID: "member-router-organization-credits-member", OrganizationID: tenant.Organization.ID, UserID: memberUser.ID, Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2"}
	if err := database.Create(&memberUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	memberClient, memberURL := loginRouterTestClient(t, memberUser.Username)
	if response := routerTestJSON(t, memberClient, http.MethodPost, memberURL+"/api/commerce/organization-credit-settings", map[string]any{"mode": "personal", "monthlyBudget": 0, "alertThreshold": 80, "version": 2}, headers); response.Code != 1 {
		t.Fatalf("member credit settings response: %#v", response)
	}
}
