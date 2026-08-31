package repository

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

type legacyBatchProductionJob struct {
	ID             string `gorm:"primaryKey"`
	OrganizationID string `gorm:"index;uniqueIndex:idx_organization_batch_request"`
	RequestID      string `gorm:"uniqueIndex:idx_organization_batch_request"`
	Kind           string `gorm:"size:24;not null;default:image_pack;index"`
}

func (legacyBatchProductionJob) TableName() string { return "batch_production_jobs" }

type legacyBatchProductionItem struct {
	ID             string `gorm:"primaryKey;index:idx_batch_ready,priority:4"`
	OrganizationID string `gorm:"not null;index;index:idx_batch_ready,priority:1"`
	JobID          string `gorm:"not null;index"`
	ProductID      string `gorm:"not null;index"`
	SKUID          string `gorm:"column:sku_id;not null;index"`
	TemplateType   string `gorm:"size:32;not null;default:custom;index"`
	VariantIndex   int    `gorm:"not null;default:1"`
	Status         string `gorm:"index;index:idx_batch_ready,priority:2"`
	CreatedAt      string `gorm:"index:idx_batch_ready,priority:3"`
}

func (legacyBatchProductionItem) TableName() string { return "batch_production_items" }

func TestBatchPricingColumnsMigrateExistingRows(t *testing.T) {
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := database.AutoMigrate(&legacyBatchProductionJob{}, &legacyBatchProductionItem{}); err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&legacyBatchProductionJob{ID: "legacy-job", OrganizationID: "organization-a", RequestID: "request-a", Kind: "image_pack"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&legacyBatchProductionItem{ID: "legacy-item", OrganizationID: "organization-a", JobID: "legacy-job", ProductID: "product-a", TemplateType: "custom", VariantIndex: 1, Status: "completed", CreatedAt: workspaceTestNow}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&model.BatchProductionJob{}, &model.BatchProductionItem{}); err != nil {
		t.Fatalf("migrate frozen pricing columns with existing rows: %v", err)
	}
	var job model.BatchProductionJob
	var item model.BatchProductionItem
	if err := database.First(&job, "id = ?", "legacy-job").Error; err != nil {
		t.Fatal(err)
	}
	if err := database.First(&item, "id = ?", "legacy-item").Error; err != nil {
		t.Fatal(err)
	}
	if job.Model != "" || item.Operation != "" || item.ResolutionTier != "" {
		t.Fatalf("legacy rows should receive safe empty frozen pricing defaults: job=%#v item=%#v", job, item)
	}
}
