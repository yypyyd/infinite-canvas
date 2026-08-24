package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Port                         string `env:"PORT" envDefault:"8080"`
	AdminUsername                string `env:"ADMIN_USERNAME" envDefault:"admin"`
	AdminPassword                string `env:"ADMIN_PASSWORD"`
	JWTSecret                    string `env:"JWT_SECRET"`
	JWTExpireHours               int    `env:"JWT_EXPIRE_HOURS" envDefault:"168"`
	StorageDriver                string `env:"STORAGE_DRIVER" envDefault:"sqlite"`
	DatabaseDSN                  string `env:"DATABASE_DSN" envDefault:"data/infinite-canvas.db"`
	PublicBaseURL                string `env:"PUBLIC_BASE_URL"`
	UserStorageQuotaMB           int64  `env:"USER_STORAGE_QUOTA_MB" envDefault:"5120"`
	LocalStoragePath             string `env:"LOCAL_STORAGE_PATH" envDefault:"data/user-files"`
	QiniuAccessKey               string `env:"QINIU_ACCESS_KEY"`
	QiniuSecretKey               string `env:"QINIU_SECRET_KEY"`
	QiniuBucket                  string `env:"QINIU_BUCKET"`
	QiniuRegion                  string `env:"QINIU_REGION" envDefault:"as0"`
	QiniuDownloadDomain          string `env:"QINIU_DOWNLOAD_DOMAIN"`
	BatchWorkerExecutorURL       string `env:"BATCH_WORKER_EXECUTOR_URL"`
	BatchWorkerToken             string `env:"BATCH_WORKER_TOKEN"`
	BatchWorkerConcurrency       int    `env:"BATCH_WORKER_CONCURRENCY" envDefault:"4"`
	BatchWorkerTenantConcurrency int    `env:"BATCH_WORKER_TENANT_CONCURRENCY" envDefault:"2"`
}

var Cfg Config

func Load() error {
	_ = godotenv.Load()
	if err := env.Parse(&Cfg); err != nil {
		return err
	}
	normalizeDockerSQLiteDSN("/app/data")
	normalizeSQLiteBusyTimeout()
	if len(strings.TrimSpace(Cfg.JWTSecret)) < 32 || Cfg.JWTSecret == "infinite-canvas" {
		return errors.New("JWT_SECRET must be set to a persistent secret of at least 32 characters")
	}
	return nil
}

func normalizeSQLiteBusyTimeout() {
	driver := strings.ToLower(strings.TrimSpace(Cfg.StorageDriver))
	dsn := strings.TrimSpace(Cfg.DatabaseDSN)
	if (driver != "" && driver != "sqlite") || dsn == "" || strings.Contains(dsn, ":memory:") || strings.Contains(strings.ToLower(dsn), "_pragma=busy_timeout(") {
		return
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	Cfg.DatabaseDSN = dsn + separator + "_pragma=busy_timeout(5000)"
}

func normalizeDockerSQLiteDSN(appDataDir string) {
	driver := strings.ToLower(strings.TrimSpace(Cfg.StorageDriver))
	if driver != "" && driver != "sqlite" {
		return
	}
	dsn := strings.TrimSpace(Cfg.DatabaseDSN)
	if dsn == "" || dsn == ":memory:" || strings.HasPrefix(dsn, "file:") {
		return
	}
	pathPart, suffix := dsn, ""
	if index := strings.Index(dsn, "?"); index >= 0 {
		pathPart = dsn[:index]
		suffix = dsn[index:]
	}
	if filepath.IsAbs(pathPart) {
		return
	}
	slashPath := filepath.ToSlash(pathPart)
	if slashPath != "data" && !strings.HasPrefix(slashPath, "data/") {
		return
	}
	if _, err := os.Stat(appDataDir); err != nil {
		return
	}
	Cfg.DatabaseDSN = filepath.Join(filepath.Dir(appDataDir), filepath.FromSlash(slashPath)) + suffix
}
