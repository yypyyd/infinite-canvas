package repository

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestOrganizationStatsCountsInOneQuery(t *testing.T) {
	testDB := setupUserWorkspaceTestDB(t)
	records := []any{
		&model.Brand{ID: "b1", OrganizationID: "org"},
		&model.Brand{ID: "b2", OrganizationID: "other"},
		&model.BatchProductionItem{ID: "i1", OrganizationID: "org", Status: model.BatchProductionStatusQueued},
		&model.BatchProductionItem{ID: "i2", OrganizationID: "org", Status: model.BatchProductionStatusRunning},
		&model.BatchProductionItem{ID: "i3", OrganizationID: "org", Status: model.BatchProductionStatusPendingReview},
	}
	for _, record := range records {
		if err := testDB.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	stats, err := OrganizationStats("org")
	if err != nil {
		t.Fatal(err)
	}
	if stats.Brands != 1 || stats.Products != 0 || stats.SKUs != 0 || stats.BatchJobs != 0 || stats.PendingItems != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
