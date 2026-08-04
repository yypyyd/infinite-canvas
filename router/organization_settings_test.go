package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestOrganizationHTTPCreateUpdateAndAuditAreTenantScoped(t *testing.T) {
	tenant := seedRouterTestTenant(t, "organization-settings")
	ownerClient, ownerURL := loginRouterTestClient(t, tenant.User.Username)
	primaryHeaders := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	createdResponse := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/organizations", map[string]string{"name": "Created Organization"}, primaryHeaders)
	var created model.Organization
	if createdResponse.Code != 0 || json.Unmarshal(createdResponse.Data, &created) != nil || created.ID == "" || created.Version != 1 || created.Name != "Created Organization" {
		t.Fatalf("create organization response: %#v, organization=%#v", createdResponse, created)
	}
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	var ownerMembership model.OrganizationMember
	if err := database.Where("organization_id = ? AND user_id = ?", created.ID, tenant.User.ID).First(&ownerMembership).Error; err != nil || ownerMembership.Role != model.OrganizationRoleOwner || ownerMembership.Version != 1 {
		t.Fatalf("created owner membership: %#v, err=%v", ownerMembership, err)
	}
	var savedUser model.User
	if err := database.First(&savedUser, "id = ?", tenant.User.ID).Error; err != nil || savedUser.OrganizationID != created.ID {
		t.Fatalf("created organization was not selected: %#v, err=%v", savedUser, err)
	}

	createdHeaders := map[string]string{"X-Organization-ID": created.ID}
	updatedResponse := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/organizations/current", map[string]any{"name": "Updated Organization", "version": 1}, createdHeaders)
	var updated model.Organization
	if updatedResponse.Code != 0 || json.Unmarshal(updatedResponse.Data, &updated) != nil || updated.Name != "Updated Organization" || updated.Version != 2 {
		t.Fatalf("update organization response: %#v, organization=%#v", updatedResponse, updated)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/organizations/current", map[string]any{"name": "Stale Organization", "version": 1}, createdHeaders); response.Code != 1 {
		t.Fatalf("stale organization update response: %#v", response)
	}

	memberUser := model.User{
		ID: "user-router-organization-settings-member", Username: "router-organization-settings-member",
		Password: tenant.User.Password, OrganizationID: created.ID, Role: model.UserRoleUser, Group: "default",
		AffCode: "aff-router-organization-settings-member", Status: model.UserStatusActive, CreatedAt: "2", UpdatedAt: "2",
	}
	member := model.OrganizationMember{
		ID: "member-router-organization-settings-member", OrganizationID: created.ID, UserID: memberUser.ID,
		Role: model.OrganizationRoleMember, Version: 1, CreatedAt: "2", UpdatedAt: "2",
	}
	if err := database.Create(&memberUser).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&member).Error; err != nil {
		t.Fatal(err)
	}
	memberClient, memberURL := loginRouterTestClient(t, memberUser.Username)
	if response := routerTestJSON(t, memberClient, http.MethodPost, memberURL+"/api/commerce/organizations/current", map[string]any{"name": "Forbidden Organization", "version": 2}, createdHeaders); response.Code != 1 {
		t.Fatalf("member organization update response: %#v", response)
	}

	type auditList struct {
		Items []model.OrganizationAuditLog `json:"items"`
		Total int                          `json:"total"`
	}
	auditResponse := routerTestJSON(t, ownerClient, http.MethodGet, ownerURL+"/api/commerce/audit-logs", nil, createdHeaders)
	var audits auditList
	if auditResponse.Code != 0 || json.Unmarshal(auditResponse.Data, &audits) != nil || audits.Total != 2 || len(audits.Items) != 2 {
		t.Fatalf("created organization audit logs: %#v, list=%#v", auditResponse, audits)
	}
	actions := map[string]bool{}
	for _, audit := range audits.Items {
		if audit.OrganizationID != created.ID {
			t.Fatalf("audit leaked another organization: %#v", audit)
		}
		actions[audit.Action] = true
	}
	if !actions["organization.create"] || !actions["organization.update"] {
		t.Fatalf("missing organization audit actions: %#v", actions)
	}
	primaryAuditResponse := routerTestJSON(t, ownerClient, http.MethodGet, ownerURL+"/api/commerce/audit-logs", nil, primaryHeaders)
	if primaryAuditResponse.Code != 0 || json.Unmarshal(primaryAuditResponse.Data, &audits) != nil || audits.Total != 0 || len(audits.Items) != 0 {
		t.Fatalf("primary organization audit leakage: %#v, list=%#v", primaryAuditResponse, audits)
	}
}
