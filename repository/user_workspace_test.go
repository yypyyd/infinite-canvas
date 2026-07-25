package repository

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

const (
	workspaceTestNow    = "2026-07-25T10:00:00.000Z"
	workspaceTestFuture = "2026-07-25T11:00:00.000Z"
	workspaceTestOld    = "2026-07-23T10:00:00.000Z"
)

func setupUserWorkspaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousConfig := config.Cfg
	db, dbErr, dbOnce = nil, nil, sync.Once{}
	config.Cfg.StorageDriver = "sqlite"
	config.Cfg.DatabaseDSN = filepath.Join(t.TempDir(), "workspace.db")

	testDB, err := DB()
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatalf("get test database connection: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		config.Cfg = previousConfig
		db, dbErr, dbOnce = nil, nil, sync.Once{}
	})
	return testDB
}

func createWorkspaceTestOrganizations(t *testing.T, testDB *gorm.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		organization := model.Organization{ID: id, Name: id, Slug: id, Status: "active", CreatedAt: workspaceTestNow, UpdatedAt: workspaceTestNow}
		if err := testDB.Create(&organization).Error; err != nil {
			t.Fatalf("create organization %s: %v", id, err)
		}
	}
}

func workspaceUpload(id, organizationID, userID, storageKey string, size int64) model.UserFileUploadReservation {
	return model.UserFileUploadReservation{
		ID:             id,
		OrganizationID: organizationID,
		UserID:         userID,
		StorageKey:     storageKey,
		ObjectKey:      "organizations/" + organizationID + "/uploads/" + id + ".png",
		MimeType:       "image/png",
		Size:           size,
		ExpiresAt:      workspaceTestFuture,
		CleanupAfter:   workspaceTestFuture,
		CreatedAt:      workspaceTestNow,
	}
}

func TestReserveUserFileUploadSerializesConcurrentQuotaChecks(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a")

	start := make(chan struct{})
	results := make(chan error, 2)
	for index, userID := range []string{"user-a", "user-b"} {
		item := workspaceUpload("upload-"+userID, "organization-a", userID, "image:quota-"+string(rune('a'+index)), 60)
		go func() {
			<-start
			_, err := ReserveUserFileUpload(item, 100, workspaceTestNow)
			results <- err
		}()
	}
	close(start)

	succeeded, rejected := 0, 0
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrWorkspaceStorageQuotaExceeded):
			rejected++
		default:
			t.Fatalf("unexpected reservation result: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("reservation results = %d succeeded, %d rejected", succeeded, rejected)
	}

	var count, reserved int64
	if err := testDB.Model(&model.UserFileUploadReservation{}).Where("organization_id = ?", "organization-a").Count(&count).Error; err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if err := testDB.Model(&model.UserFileUploadReservation{}).Where("organization_id = ?", "organization-a").Select("COALESCE(SUM(reserved_bytes), 0)").Scan(&reserved).Error; err != nil {
		t.Fatalf("sum reservations: %v", err)
	}
	if count != 1 || reserved != 60 {
		t.Fatalf("stored reservations = %d files and %d bytes, want 1 file and 60 bytes", count, reserved)
	}
}

func TestCancelUserFileUploadReservationIsScopedAndReleasesReservation(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a", "organization-b")
	item := workspaceUpload("upload-a", "organization-a", "user-a", "image:cancel", 80)
	if _, err := ReserveUserFileUpload(item, 100, workspaceTestNow); err != nil {
		t.Fatalf("reserve upload: %v", err)
	}

	if err := CancelUserFileUploadReservation("organization-b", "user-a", item.ID, workspaceTestNow); err != nil {
		t.Fatalf("cross-organization cancellation: %v", err)
	}
	if err := CancelUserFileUploadReservation("organization-a", "user-b", item.ID, workspaceTestNow); err != nil {
		t.Fatalf("cross-user cancellation: %v", err)
	}
	if _, found, err := GetUserFileUploadReservation("organization-a", "user-a", item.ID); err != nil || !found {
		t.Fatalf("reservation should remain after unauthorized cancellation, found=%v err=%v", found, err)
	}

	if err := CancelUserFileUploadReservation("organization-a", "user-a", item.ID, workspaceTestNow); err != nil {
		t.Fatalf("cancel upload: %v", err)
	}
	if _, found, err := GetUserFileUploadReservation("organization-a", "user-a", item.ID); err != nil || found {
		t.Fatalf("reservation should be released, found=%v err=%v", found, err)
	}
	var deletion model.UserObjectDeletion
	if err := testDB.First(&deletion, "organization_id = ? AND object_key = ?", "organization-a", item.ObjectKey).Error; err != nil {
		t.Fatalf("load queued object deletion: %v", err)
	}
	if deletion.UserID != "user-a" || deletion.Size != item.Size || deletion.Status != "pending" {
		t.Fatalf("unexpected object deletion: %#v", deletion)
	}
}

func TestUserFileQueriesEnforceOrganizationAndUserBoundaries(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a", "organization-b")
	files := []model.UserFile{
		{ID: "file-a", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:shared", ObjectKey: "organizations/organization-a/files/shared.png", Size: 10},
		{ID: "file-b", OrganizationID: "organization-b", UserID: "user-b", StorageKey: "image:shared", ObjectKey: "organizations/organization-b/files/shared.png", Size: 20},
	}
	if err := testDB.Create(&files).Error; err != nil {
		t.Fatalf("create files: %v", err)
	}
	reservation := workspaceUpload("upload-a", "organization-a", "user-a", "image:private", 10)
	if _, err := ReserveUserFileUpload(reservation, 100, workspaceTestNow); err != nil {
		t.Fatalf("reserve upload: %v", err)
	}

	fileA, found, err := GetUserFile("organization-a", "image:shared")
	if err != nil || !found || fileA.ID != "file-a" {
		t.Fatalf("organization A file = %#v, found=%v err=%v", fileA, found, err)
	}
	fileB, found, err := GetUserFile("organization-b", "image:shared")
	if err != nil || !found || fileB.ID != "file-b" {
		t.Fatalf("organization B file = %#v, found=%v err=%v", fileB, found, err)
	}
	if _, found, err := GetUserFileUploadReservation("organization-b", "user-a", reservation.ID); err != nil || found {
		t.Fatalf("cross-organization reservation lookup found=%v err=%v", found, err)
	}
	if _, found, err := GetUserFileUploadReservation("organization-a", "user-b", reservation.ID); err != nil || found {
		t.Fatalf("cross-user reservation lookup found=%v err=%v", found, err)
	}
}

func TestCollectUserFileGarbageProtectsReferencesAndActiveReservations(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	createWorkspaceTestOrganizations(t, testDB, "organization-a", "organization-b")
	files := []model.UserFile{
		{ID: "file-referenced", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:referenced", ObjectKey: "organizations/organization-a/files/referenced.png", Size: 10, UnreferencedAt: workspaceTestOld},
		{ID: "file-reserved", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:reserved", ObjectKey: "organizations/organization-a/files/reserved.png", Size: 10, UnreferencedAt: workspaceTestOld},
		{ID: "file-orphan", OrganizationID: "organization-a", UserID: "user-a", StorageKey: "image:orphan", ObjectKey: "organizations/organization-a/files/orphan.png", Size: 10, UnreferencedAt: workspaceTestOld},
		{ID: "file-other", OrganizationID: "organization-b", UserID: "user-b", StorageKey: "image:orphan", ObjectKey: "organizations/organization-b/files/orphan.png", Size: 10, UnreferencedAt: workspaceTestOld},
	}
	if err := testDB.Create(&files).Error; err != nil {
		t.Fatalf("create files: %v", err)
	}
	reference := model.UserFileReference{ID: "reference-a", OrganizationID: "organization-a", Domain: "canvas_project", ObjectID: "project-a", StorageKey: "image:referenced", CreatedAt: workspaceTestNow}
	if err := testDB.Create(&reference).Error; err != nil {
		t.Fatalf("create file reference: %v", err)
	}
	reservation := workspaceUpload("upload-a", "organization-a", "user-a", "image:reserved", 10)
	if _, err := ReserveUserFileUpload(reservation, 1000, workspaceTestNow); err != nil {
		t.Fatalf("reserve upload: %v", err)
	}

	if err := CollectUserFileGarbage("organization-a", workspaceTestNow, workspaceTestOld, 1000); err != nil {
		t.Fatalf("collect file garbage: %v", err)
	}
	for _, id := range []string{"file-referenced", "file-reserved", "file-other"} {
		var count int64
		if err := testDB.Model(&model.UserFile{}).Where("id = ?", id).Count(&count).Error; err != nil {
			t.Fatalf("count protected file %s: %v", id, err)
		}
		if count != 1 {
			t.Fatalf("protected file %s was deleted", id)
		}
	}
	var orphanCount int64
	if err := testDB.Model(&model.UserFile{}).Where("organization_id = ? AND id = ?", "organization-a", "file-orphan").Count(&orphanCount).Error; err != nil {
		t.Fatalf("count orphan file: %v", err)
	}
	if orphanCount != 0 {
		t.Fatal("unreferenced file was not collected")
	}
	var deletion model.UserObjectDeletion
	if err := testDB.First(&deletion, "object_key = ?", "organizations/organization-a/files/orphan.png").Error; err != nil {
		t.Fatalf("load orphan deletion: %v", err)
	}
	if deletion.OrganizationID != "organization-a" || deletion.Status != "pending" {
		t.Fatalf("unexpected orphan deletion: %#v", deletion)
	}
}
