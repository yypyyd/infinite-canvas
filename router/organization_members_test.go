package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestOrganizationMemberHTTPEnforcesVersionsAndOwnership(t *testing.T) {
	tenant := seedRouterTestTenant(t, "members")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	addMember := func(suffix string, role model.OrganizationRole) (model.User, model.OrganizationMember) {
		t.Helper()
		user := model.User{
			ID: "user-router-members-" + suffix, Username: "router-members-" + suffix,
			Password: tenant.User.Password, OrganizationID: tenant.Organization.ID, Role: model.UserRoleUser,
			Group: "default", AffCode: "aff-router-members-" + suffix, Status: model.UserStatusActive,
			CreatedAt: suffix, UpdatedAt: suffix,
		}
		member := model.OrganizationMember{
			ID: "member-router-members-" + suffix, OrganizationID: tenant.Organization.ID,
			UserID: user.ID, Role: role, Version: 1, CreatedAt: suffix, UpdatedAt: suffix,
		}
		if err := database.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&member).Error; err != nil {
			t.Fatal(err)
		}
		return user, member
	}
	adminUser, adminMember := addMember("admin", model.OrganizationRoleAdmin)
	limitedUser, limitedMember := addMember("limited", model.OrganizationRoleMember)
	removedUser, removedMember := addMember("removed", model.OrganizationRoleMember)
	var ownerMember model.OrganizationMember
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, tenant.User.ID).First(&ownerMember).Error; err != nil {
		t.Fatal(err)
	}
	var secondaryMember model.OrganizationMember
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Secondary.ID, tenant.User.ID).First(&secondaryMember).Error; err != nil {
		t.Fatal(err)
	}

	ownerClient, ownerURL := loginRouterTestClient(t, tenant.User.Username)
	headers := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	updated := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+limitedMember.ID, map[string]any{"role": model.OrganizationRoleReviewer, "version": 1}, headers)
	var updatedMember model.OrganizationMember
	if updated.Code != 0 || json.Unmarshal(updated.Data, &updatedMember) != nil || updatedMember.Role != model.OrganizationRoleReviewer || updatedMember.Version != 2 {
		t.Fatalf("unexpected member update: %#v, member=%#v", updated, updatedMember)
	}
	stale := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+limitedMember.ID, map[string]any{"role": model.OrganizationRoleAdmin, "version": 1}, headers)
	if stale.Code != 1 {
		t.Fatalf("stale member update response: %#v", stale)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+secondaryMember.ID, map[string]any{"role": model.OrganizationRoleAdmin, "version": 1}, headers); response.Code != 1 {
		t.Fatalf("cross-organization member update response: %#v", response)
	}

	limitedClient, limitedURL := loginRouterTestClient(t, limitedUser.Username)
	if response := routerTestJSON(t, limitedClient, http.MethodPost, limitedURL+"/api/commerce/members/"+adminMember.ID, map[string]any{"role": model.OrganizationRoleMember, "version": 1}, headers); response.Code != 1 {
		t.Fatalf("limited member management response: %#v", response)
	}
	staleRemoval := routerTestJSON(t, ownerClient, http.MethodDelete, ownerURL+"/api/commerce/members/"+removedMember.ID+"?expectedVersion=2", nil, headers)
	if staleRemoval.Code != 1 {
		t.Fatalf("stale member removal response: %#v", staleRemoval)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodDelete, ownerURL+"/api/commerce/members/"+removedMember.ID+"?expectedVersion=1", nil, headers); response.Code != 0 {
		t.Fatalf("member removal response: %#v", response)
	}
	var memberCount int64
	if err := database.Model(&model.OrganizationMember{}).Where("id = ?", removedMember.ID).Count(&memberCount).Error; err != nil || memberCount != 0 {
		t.Fatalf("removed membership count = %d, err=%v", memberCount, err)
	}
	var savedRemovedUser model.User
	if err := database.First(&savedRemovedUser, "id = ?", removedUser.ID).Error; err != nil || savedRemovedUser.OrganizationID != "" {
		t.Fatalf("removed user organization was not cleared: %#v, err=%v", savedRemovedUser, err)
	}

	staleTransfer := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+adminMember.ID+"/transfer-owner?expectedVersion=2", nil, headers)
	if staleTransfer.Code != 1 {
		t.Fatalf("stale ownership transfer response: %#v", staleTransfer)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+adminMember.ID+"/transfer-owner?expectedVersion=1", nil, headers); response.Code != 0 {
		t.Fatalf("ownership transfer response: %#v", response)
	}
	if err := database.First(&ownerMember, "id = ?", ownerMember.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&adminMember, "id = ?", adminMember.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ownerMember.Role != model.OrganizationRoleAdmin || ownerMember.Version != 2 || adminMember.Role != model.OrganizationRoleOwner || adminMember.Version != 2 {
		t.Fatalf("unexpected ownership state: old=%#v, new=%#v", ownerMember, adminMember)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/members/"+ownerMember.ID+"/transfer-owner?expectedVersion=2", nil, headers); response.Code != 1 {
		t.Fatalf("former owner transfer response: %#v", response)
	}

	newOwnerClient, newOwnerURL := loginRouterTestClient(t, adminUser.Username)
	promoted := routerTestJSON(t, newOwnerClient, http.MethodPost, newOwnerURL+"/api/commerce/members/"+limitedMember.ID, map[string]any{"role": model.OrganizationRoleMember, "version": 2}, headers)
	if promoted.Code != 0 || json.Unmarshal(promoted.Data, &updatedMember) != nil || updatedMember.Role != model.OrganizationRoleMember || updatedMember.Version != 3 {
		t.Fatalf("new owner member update response: %#v, member=%#v", promoted, updatedMember)
	}
}
