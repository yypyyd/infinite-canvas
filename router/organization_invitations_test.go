package router

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func TestOrganizationInvitationHTTPLifecycleAndOutbox(t *testing.T) {
	tenant := seedRouterTestTenant(t, "invitations")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	addIndependentUser := func(suffix string, email string) (model.User, model.Organization) {
		t.Helper()
		user := model.User{
			ID: "user-router-invite-" + suffix, Username: "router-invite-" + suffix,
			Password: tenant.User.Password, Email: email, OrganizationID: "org-router-invite-" + suffix,
			Role: model.UserRoleUser, Group: "default", AffCode: "aff-router-invite-" + suffix,
			Status: model.UserStatusActive, CreatedAt: suffix, UpdatedAt: suffix,
		}
		organization := model.Organization{
			ID: user.OrganizationID, Name: "Independent " + suffix, Slug: "router-invite-" + suffix,
			Status: "active", Version: 1, CreatedBy: user.ID, CreatedAt: suffix, UpdatedAt: suffix,
		}
		membership := model.OrganizationMember{
			ID: "member-router-invite-" + suffix, OrganizationID: organization.ID, UserID: user.ID,
			Role: model.OrganizationRoleOwner, Version: 1, CreatedAt: suffix, UpdatedAt: suffix,
		}
		if err := database.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&organization).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Create(&membership).Error; err != nil {
			t.Fatal(err)
		}
		return user, organization
	}
	invitee, inviteeOrganization := addIndependentUser("accept", "accept@example.com")
	imposter, imposterOrganization := addIndependentUser("imposter", "imposter@example.com")
	revokedUser, revokedOrganization := addIndependentUser("revoked", "revoked@example.com")
	expiredUser, expiredOrganization := addIndependentUser("expired", "expired@example.com")

	ownerClient, ownerURL := loginRouterTestClient(t, tenant.User.Username)
	ownerHeaders := map[string]string{"X-Organization-ID": tenant.Organization.ID}
	createInvitation := func(email string) model.OrganizationInvitation {
		t.Helper()
		response := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/invitations", map[string]any{"email": email, "role": model.OrganizationRoleMember}, ownerHeaders)
		var invitation model.OrganizationInvitation
		if response.Code != 0 || json.Unmarshal(response.Data, &invitation) != nil || invitation.ID == "" || invitation.Status != model.OrganizationInvitationPending {
			t.Fatalf("create invitation response: %#v, invitation=%#v", response, invitation)
		}
		return invitation
	}

	acceptedInvitation := createInvitation(invitee.Email)
	var acceptedOutbox model.OrganizationEmailOutbox
	if err := database.First(&acceptedOutbox, "invitation_id = ?", acceptedInvitation.ID).Error; err != nil || acceptedOutbox.Status != "pending" || acceptedOutbox.Receiver != invitee.Email {
		t.Fatalf("unexpected invitation outbox: %#v, err=%v", acceptedOutbox, err)
	}
	duplicate := routerTestJSON(t, ownerClient, http.MethodPost, ownerURL+"/api/commerce/invitations", map[string]any{"email": invitee.Email, "role": model.OrganizationRoleAdmin}, ownerHeaders)
	if duplicate.Code != 1 {
		t.Fatalf("duplicate invitation response: %#v", duplicate)
	}

	imposterClient, imposterURL := loginRouterTestClient(t, imposter.Username)
	if response := routerTestJSON(t, imposterClient, http.MethodPost, imposterURL+"/api/commerce/invitations/"+acceptedInvitation.ID+"/accept", nil, map[string]string{"X-Organization-ID": imposterOrganization.ID}); response.Code != 1 {
		t.Fatalf("imposter invitation acceptance response: %#v", response)
	}
	inviteeClient, inviteeURL := loginRouterTestClient(t, invitee.Username)
	pending := routerTestJSON(t, inviteeClient, http.MethodGet, inviteeURL+"/api/commerce/invitations", nil, map[string]string{"X-Organization-ID": inviteeOrganization.ID})
	var pendingInvitations []model.OrganizationInvitation
	if pending.Code != 0 || json.Unmarshal(pending.Data, &pendingInvitations) != nil || len(pendingInvitations) != 1 || pendingInvitations[0].ID != acceptedInvitation.ID {
		t.Fatalf("invitee pending invitations: %#v, items=%#v", pending, pendingInvitations)
	}
	if response := routerTestJSON(t, inviteeClient, http.MethodPost, inviteeURL+"/api/commerce/invitations/"+acceptedInvitation.ID+"/accept", nil, map[string]string{"X-Organization-ID": inviteeOrganization.ID}); response.Code != 0 {
		t.Fatalf("accept invitation response: %#v", response)
	}
	var acceptedMembership model.OrganizationMember
	if err := database.Where("organization_id = ? AND user_id = ?", tenant.Organization.ID, invitee.ID).First(&acceptedMembership).Error; err != nil || acceptedMembership.Role != model.OrganizationRoleMember || acceptedMembership.Version != 1 {
		t.Fatalf("accepted membership: %#v, err=%v", acceptedMembership, err)
	}
	if err := database.First(&acceptedOutbox, "id = ?", acceptedOutbox.ID).Error; err != nil || acceptedOutbox.Status != "cancelled" {
		t.Fatalf("accepted invitation outbox: %#v, err=%v", acceptedOutbox, err)
	}
	if response := routerTestJSON(t, inviteeClient, http.MethodPost, inviteeURL+"/api/commerce/invitations/"+acceptedInvitation.ID+"/accept", nil, ownerHeaders); response.Code != 1 {
		t.Fatalf("repeated invitation acceptance response: %#v", response)
	}
	if response := routerTestJSON(t, inviteeClient, http.MethodPost, inviteeURL+"/api/commerce/invitations", map[string]any{"email": "forbidden@example.com", "role": model.OrganizationRoleMember}, ownerHeaders); response.Code != 1 {
		t.Fatalf("member invitation response: %#v", response)
	}

	revokedInvitation := createInvitation(revokedUser.Email)
	if response := routerTestJSON(t, ownerClient, http.MethodDelete, ownerURL+"/api/commerce/invitations/"+revokedInvitation.ID, nil, ownerHeaders); response.Code != 0 {
		t.Fatalf("revoke invitation response: %#v", response)
	}
	var revokedOutbox model.OrganizationEmailOutbox
	if err := database.First(&revokedOutbox, "invitation_id = ?", revokedInvitation.ID).Error; err != nil || revokedOutbox.Status != "cancelled" {
		t.Fatalf("revoked invitation outbox: %#v, err=%v", revokedOutbox, err)
	}
	if response := routerTestJSON(t, ownerClient, http.MethodDelete, ownerURL+"/api/commerce/invitations/"+revokedInvitation.ID, nil, ownerHeaders); response.Code != 1 {
		t.Fatalf("repeated invitation revocation response: %#v", response)
	}
	revokedClient, revokedURL := loginRouterTestClient(t, revokedUser.Username)
	if response := routerTestJSON(t, revokedClient, http.MethodPost, revokedURL+"/api/commerce/invitations/"+revokedInvitation.ID+"/accept", nil, map[string]string{"X-Organization-ID": revokedOrganization.ID}); response.Code != 1 {
		t.Fatalf("revoked invitation acceptance response: %#v", response)
	}

	expiredInvitation := createInvitation(expiredUser.Email)
	if err := database.Model(&model.OrganizationInvitation{}).Where("id = ?", expiredInvitation.ID).Update("expires_at", "2000-01-01T00:00:00Z").Error; err != nil {
		t.Fatal(err)
	}
	expiredClient, expiredURL := loginRouterTestClient(t, expiredUser.Username)
	expiredList := routerTestJSON(t, expiredClient, http.MethodGet, expiredURL+"/api/commerce/invitations", nil, map[string]string{"X-Organization-ID": expiredOrganization.ID})
	if expiredList.Code != 0 || json.Unmarshal(expiredList.Data, &pendingInvitations) != nil || len(pendingInvitations) != 0 {
		t.Fatalf("expired invitation list response: %#v, items=%#v", expiredList, pendingInvitations)
	}
	if claimed, ok, err := repository.ClaimOrganizationEmailOutbox("2100-01-01T00:00:00Z", "2100-01-01T00:05:00Z", "expired-lease"); err != nil || ok {
		t.Fatalf("expired invitation outbox claim: %#v, ok=%v, err=%v", claimed, ok, err)
	}
	var expiredOutbox model.OrganizationEmailOutbox
	if err := database.First(&expiredOutbox, "invitation_id = ?", expiredInvitation.ID).Error; err != nil || expiredOutbox.Status != "cancelled" {
		t.Fatalf("expired invitation outbox: %#v, err=%v", expiredOutbox, err)
	}
}
