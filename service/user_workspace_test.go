package service

import (
	"encoding/json"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestApplyUserWorkspaceChangesRejectsReviewerWrites(t *testing.T) {
	_, user, organization := seedTenant(t, "workspace-reviewer")
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.OrganizationMember{}).Where("organization_id = ? AND user_id = ?", organization.ID, user.ID).Update("role", model.OrganizationRoleReviewer).Error; err != nil {
		t.Fatal(err)
	}
	request := WorkspaceChangeRequest{Changes: []WorkspaceChangeInput{{Domain: "canvas_project", ObjectID: "project-reviewer", Data: json.RawMessage(`{"title":"Reviewer canvas"}`)}}}
	if _, err := ApplyUserWorkspaceChanges(user, request); err == nil {
		t.Fatal("expected reviewer workspace save to be rejected")
	}
	var count int64
	if err := db.Model(&model.UserProject{}).Where("organization_id = ? AND id = ?", organization.ID, "project-reviewer").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("reviewer workspace write was persisted")
	}
}

func TestApplyUserWorkspaceChangesRejectsCrossOrganizationMediaReference(t *testing.T) {
	_, userA, organizationA := seedTenant(t, "workspace-media-a")
	_, _, organizationB := seedTenant(t, "workspace-media-b")
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	foreignFile := model.UserFile{ID: "file-workspace-media-b", OrganizationID: organizationB.ID, UserID: "user-workspace-media-b", StorageKey: "image:foreign", ObjectKey: "organizations/" + organizationB.ID + "/files/foreign.png", Size: 10}
	if err := db.Create(&foreignFile).Error; err != nil {
		t.Fatal(err)
	}
	request := WorkspaceChangeRequest{Changes: []WorkspaceChangeInput{{Domain: "canvas_project", ObjectID: "project-media-a", Data: json.RawMessage(`{"title":"Canvas","image":"image:foreign"}`)}}}
	if _, err := ApplyUserWorkspaceChanges(userA, request); err == nil {
		t.Fatal("expected cross-organization media reference to be rejected")
	}
	var count int64
	if err := db.Model(&model.UserProject{}).Where("organization_id = ? AND id = ?", organizationA.ID, "project-media-a").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("workspace record with foreign media was persisted")
	}
}
