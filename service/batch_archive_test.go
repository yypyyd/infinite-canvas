package service

import (
	"strings"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
)

func TestBatchProductionArchiveNamesAreStableAndPathSafe(t *testing.T) {
	jobName := batchProductionArchiveName(model.BatchProductionJob{ID: "batch-a", Name: " 秋季 / 上新 "})
	if jobName != "秋季-上新.zip" { t.Fatalf("archive name = %q", jobName) }
	entry := batchProductionArchiveEntryName(model.BatchProductionArchiveItem{ID: "batch-item-1234567890", ProductID: "product-a", ProductCode: "../商品/A", SKUID: "sku-a", SKUCode: "SKU/../../红色", MimeType: "image/png", IsPrimary: true}, model.ProductionDeliverySpec{})
	if entry != "商品A/商品A_SKU红色_主图_1234567890.png" { t.Fatalf("archive entry = %q", entry) }
	if strings.HasPrefix(entry, "/") || strings.Contains(entry, "../") || strings.Count(entry, "/") != 1 { t.Fatalf("unsafe archive entry = %q", entry) }
}
