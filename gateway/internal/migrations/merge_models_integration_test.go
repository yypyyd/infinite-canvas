package migrations

import (
	"context"
	"testing"
)

func migrationSQL(t *testing.T, version int64) string {
	t.Helper()
	definitions, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		if definition.Version == version {
			return definition.SQL
		}
	}
	t.Fatalf("migration %d not found", version)
	return ""
}

// TestUnifiedModelsAfterMigrations checks the end state of a fresh database:
// one public ID per model with every provider as a route, no prefixed IDs and
// route priority/weight inherited (NULL).
func TestUnifiedModelsAfterMigrations(t *testing.T) {
	db := migrationTestDB(t)
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var prefixed int64
	db.Raw(`SELECT COUNT(*) FROM logical_models WHERE id LIKE 'adobe-%' OR id LIKE 'byteplus-%' OR id LIKE 'chatgpt-%'`).Scan(&prefixed)
	if prefixed != 0 {
		t.Fatalf("prefixed logical models left: %d", prefixed)
	}
	var providers []string
	db.Raw(`SELECT provider FROM model_routes WHERE logical_model_id = 'gpt-image-2' AND enabled ORDER BY provider`).Scan(&providers)
	if len(providers) != 3 || providers[0] != "adobe" || providers[1] != "byteplus" || providers[2] != "chatgpt" {
		t.Fatalf("gpt-image-2 routes=%v, want adobe/byteplus/chatgpt", providers)
	}
	var overridden int64
	db.Raw(`SELECT COUNT(*) FROM model_routes WHERE priority IS NOT NULL OR weight IS NOT NULL`).Scan(&overridden)
	if overridden != 0 {
		t.Fatalf("routes with explicit priority/weight after 000035: %d", overridden)
	}
}

// TestMergeMigrationMovesPrefixedData replays 000034 on a database put back
// into the split (000020/000022) shape with history attached.
func TestMergeMigrationMovesPrefixedData(t *testing.T) {
	db := migrationTestDB(t)
	if err := Run(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	setup := `
INSERT INTO logical_models (id,kind,name,enabled,weight,generation_count) VALUES
 ('byteplus-gpt-image-2','image','BytePlus GPT Image 2',TRUE,0,7),
 ('adobe-gpt-image-2','image','Adobe GPT Image 2',TRUE,0,5),
 ('adobe-seedance-2.0','video','Adobe Seedance 2.0',TRUE,0,2);
UPDATE logical_models SET generation_count = 10 WHERE id = 'gpt-image-2';
UPDATE logical_models SET generation_count = 0 WHERE id = 'seedance-2.0';
UPDATE model_routes SET logical_model_id = 'byteplus-gpt-image-2' WHERE id = 'image.gpt-image-2.byteplus';
UPDATE model_routes SET logical_model_id = 'adobe-gpt-image-2', enabled = FALSE WHERE id = 'image.gpt-image-2.adobe';
UPDATE model_routes SET logical_model_id = 'adobe-seedance-2.0' WHERE id = 'video.seedance-2.0.adobe';
DELETE FROM logical_models WHERE id = 'seedance-2.0';
INSERT INTO event_logs (id, kind, status, model, provider, ts) VALUES ('merge-event','image','succeeded','adobe-gpt-image-2','adobe',NOW());
`
	if err := db.Exec(setup).Error; err != nil {
		t.Fatal("setup split state:", err)
	}
	if err := db.Exec(migrationSQL(t, 34)).Error; err != nil {
		t.Fatal("replay 000034:", err)
	}
	var count int64
	db.Raw(`SELECT generation_count FROM logical_models WHERE id = 'gpt-image-2'`).Scan(&count)
	if count != 22 {
		t.Fatalf("gpt-image-2 generation_count=%d, want 22", count)
	}
	var routes int64
	db.Raw(`SELECT COUNT(*) FROM model_routes WHERE logical_model_id = 'gpt-image-2' AND enabled`).Scan(&routes)
	if routes != 3 {
		t.Fatalf("enabled gpt-image-2 routes=%d, want 3", routes)
	}
	db.Raw(`SELECT COUNT(*) FROM model_routes WHERE logical_model_id = 'seedance-2.0'`).Scan(&routes)
	if routes != 1 {
		t.Fatalf("seedance-2.0 routes=%d, want 1 (recreated logical model)", routes)
	}
	var left int64
	db.Raw(`SELECT COUNT(*) FROM logical_models WHERE id IN ('byteplus-gpt-image-2','adobe-gpt-image-2','adobe-seedance-2.0')`).Scan(&left)
	if left != 0 {
		t.Fatalf("prefixed logical models left: %d", left)
	}
	var model string
	db.Raw(`SELECT model FROM event_logs WHERE id = 'merge-event'`).Scan(&model)
	if model != "gpt-image-2" {
		t.Fatalf("event model=%q, want gpt-image-2", model)
	}
}
