package config

import "testing"

func TestProductionStorageIsOptional(t *testing.T) {
	base := Config{AppEnv: "production", PublicBaseURL: "https://huantu.example.com", PostgresDSN: "postgres://gateway:a-long-throwaway-password@db:5432/huantu?sslmode=disable"}
	if err := base.validate(); err != nil {
		t.Fatalf("production without object storage: %v", err)
	}
	withStorage := base
	withStorage.RustFSEndpoint = "not a url"
	if err := withStorage.validate(); err == nil {
		t.Fatal("invalid RustFS endpoint should be rejected when configured")
	}
}
