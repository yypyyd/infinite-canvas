package config

import "testing"

func TestProductionStorageIsOptional(t *testing.T) {
	base := Config{AppEnv: "production", PostgresDSN: "postgres://huantu:pw@db:5432/huantu?sslmode=disable"}
	if err := base.validate(); err != nil {
		t.Fatalf("production without object storage: %v", err)
	}
	withStorage := base
	withStorage.RustFSEndpoint = "not a url"
	if err := withStorage.validate(); err == nil {
		t.Fatal("invalid RustFS endpoint should be rejected when configured")
	}
}

func TestResolvePostgresDSN(t *testing.T) {
	embeddedDSNFile = t.TempDir() + "/missing.dsn"
	t.Setenv("POOL_POSTGRES_DSN", "")
	t.Setenv("STORAGE_DRIVER", "sqlite")
	t.Setenv("DATABASE_DSN", "data/infinite-canvas.db")
	if got := ResolvePostgresDSN(); got != "" {
		t.Fatalf("sqlite should not yield a pool DSN, got %q", got)
	}
	t.Setenv("STORAGE_DRIVER", "postgres")
	t.Setenv("DATABASE_DSN", "postgres://a@b/c")
	if got := ResolvePostgresDSN(); got != "postgres://a@b/c" {
		t.Fatalf("postgres storage should reuse DATABASE_DSN, got %q", got)
	}
	t.Setenv("POOL_POSTGRES_DSN", "postgres://x@y/z")
	if got := ResolvePostgresDSN(); got != "postgres://x@y/z" {
		t.Fatalf("POOL_POSTGRES_DSN should win, got %q", got)
	}
}
