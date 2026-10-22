package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/yypyyd/infinite-canvas/gateway/internal/config"
	"github.com/yypyyd/infinite-canvas/gateway/internal/http/handler"
	"github.com/yypyyd/infinite-canvas/gateway/internal/http/router"
	"github.com/yypyyd/infinite-canvas/gateway/internal/migrations"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/adobe"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/byteplus"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/chatgpt"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/grok"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
	"github.com/yypyyd/infinite-canvas/gateway/internal/service"
	"github.com/yypyyd/infinite-canvas/gateway/internal/storage"
)

// BuiltinCredentialID owns every request 幻图 sends to the gateway
// in-process. Its key hash is random and never revealed, so it cannot be used
// over the network.
const BuiltinCredentialID = "cred-huantu-builtin"

type App struct {
	Config      *config.Config
	DB          *gorm.DB
	Redis       *redis.Client
	Handlers    router.Handlers
	Credentials *service.APICredentialService
	Builtin     *model.APICredential

	cancel context.CancelFunc
}

// OpenDB opens the gateway Postgres database with search_path pinned to the
// gateway schema (default "gateway"), creating the schema when missing. This
// keeps every gateway table out of 幻图's own tables even when both share one
// database.
func OpenDB(ctx context.Context, dsn, schema string, gormLogger logger.Interface) (*gorm.DB, error) {
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "gateway"
	}
	if !validSchemaName(schema) {
		return nil, fmt.Errorf("invalid GATEWAY_DB_SCHEMA %q", schema)
	}
	pgCfg, err := pgx.ParseConfig(strings.TrimSpace(dsn))
	if err != nil {
		return nil, fmt.Errorf("parse GATEWAY_POSTGRES_DSN: %w", err)
	}
	bootstrapConn, err := pgx.ConnectConfig(ctx, pgCfg)
	if err != nil {
		return nil, fmt.Errorf("connect gateway postgres: %w", err)
	}
	_, err = bootstrapConn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS "+pgx.Identifier{schema}.Sanitize())
	_ = bootstrapConn.Close(ctx)
	if err != nil {
		return nil, fmt.Errorf("create gateway schema: %w", err)
	}
	pgCfg.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*pgCfg)
	if gormLogger == nil {
		gormLogger = logger.Default.LogMode(logger.Warn)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{TranslateError: true, Logger: gormLogger})
	if err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func validSchemaName(name string) bool {
	if len(name) > 63 {
		return false
	}
	for i, r := range name {
		if r == '_' || (r >= 'a' && r <= 'z') || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

// Migrate brings the gateway schema to the current version: versioned SQL
// migrations, compatible AutoMigrate columns, then default settings.
func Migrate(ctx context.Context, db *gorm.DB) error {
	if err := migrations.Run(ctx, db); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	if err := db.WithContext(ctx).AutoMigrate(model.AutoMigrateModels()...); err != nil {
		return fmt.Errorf("auto migrate operational tables: %w", err)
	}
	if err := seedDefaults(ctx, db); err != nil {
		return fmt.Errorf("seed defaults: %w", err)
	}
	return nil
}

func NewApp(ctx context.Context) (*App, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.GeneratedRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create generated root %s: %w", cfg.GeneratedRoot, err)
	}

	dbLogger := logger.Default.LogMode(logger.Warn)
	if cfg.AppEnv == "production" {
		// Provider cookies, API keys and login material are query parameters. Never
		// interpolate them into production SQL logs, including slow-query output.
		dbLogger = logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold: time.Second, LogLevel: logger.Warn, ParameterizedQueries: true,
		})
	}
	db, err := OpenDB(ctx, cfg.PostgresDSN, cfg.DBSchema, dbLogger)
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sql db: %w", err)
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// Forward-only migrations own identity, routing, provider accounts, quota,
	// dispatch, and idempotency constraints. AutoMigrate runs afterwards only to
	// add compatible columns on retained operational tables.
	if err := Migrate(ctx, db); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	credentialRepo := repo.NewAPICredentialRepository(db)
	modelRepo := repo.NewModelRepository(db)
	eventRepo := repo.NewEventRepository(db)
	tokenRepo := repo.NewTokenRepository(db)
	refreshRepo := repo.NewRefreshProfileRepository(db)
	settingsRepo := repo.NewSiteSettingRepository(db, rdb)
	bannedRepo := repo.NewBannedWordRepository(db)

	concurrencySvc := service.NewConcurrencyService(rdb)
	credentialSvc := service.NewAPICredentialService(credentialRepo)
	credentialSvc.SetConcurrency(concurrencySvc)

	builtin, err := ensureBuiltinCredential(ctx, credentialRepo)
	if err != nil {
		_ = rdb.Close()
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ensure built-in credential: %w", err)
	}

	adobeClient := adobe.NewClient("clio-playground-web", "")
	bytePlusClient := byteplus.NewClient("")
	chatGPTClient := chatgpt.NewClient("")
	grokClient := grok.NewClient("")

	objectStore := storage.New(cfg.RustFSEndpoint, cfg.RustFSBucket, cfg.RustFSAccessKey, cfg.RustFSSecretKey)

	tokenSvc := service.NewTokenService(
		tokenRepo, refreshRepo, eventRepo, settingsRepo,
		adobeClient, bytePlusClient, chatGPTClient,
		grokClient, modelRepo.Quotas(),
	)
	refreshSvc := service.NewRefreshProfileService(refreshRepo, tokenRepo, adobeClient, bytePlusClient, tokenSvc)
	v1Svc := service.NewV1Service(
		cfg, modelRepo, credentialSvc, eventRepo, tokenRepo, settingsRepo, concurrencySvc,
		adobeClient, bytePlusClient, chatGPTClient,
		grokClient, objectStore,
	)
	v1Svc.SetRefresh(refreshSvc)
	v1Svc.SetBannedWords(bannedRepo)
	adminConsoleSvc := service.NewAdminConsoleService(cfg, db, modelRepo, tokenRepo, tokenSvc, settingsRepo)
	maintenanceCtx, cancelMaintenance := context.WithCancel(context.Background())
	maintenanceSvc := service.NewMaintenanceService(tokenRepo, tokenSvc, eventRepo, refreshSvc, settingsRepo, modelRepo, objectStore, v1Svc)
	go maintenanceSvc.Run(maintenanceCtx)

	handlers := router.Handlers{
		Health:         handler.NewHealthHandler(db, rdb, objectStore),
		V1:             handler.NewV1Handler(v1Svc),
		APICredentials: handler.NewUserToolsHandler(credentialSvc),
		Admin:          handler.NewAdminConsoleHandler(adminConsoleSvc),
		BannedWords:    handler.NewBannedWordsHandler(bannedRepo),
	}

	return &App{Config: cfg, DB: db, Redis: rdb, Handlers: handlers, Credentials: credentialSvc, Builtin: builtin, cancel: cancelMaintenance}, nil
}

func ensureBuiltinCredential(ctx context.Context, credentials *repo.APICredentialRepository) (*model.APICredential, error) {
	existing, err := credentials.GetByID(ctx, BuiltinCredentialID)
	if err == nil && existing != nil {
		if !existing.IsActive() {
			log.Printf("gateway: built-in credential %s is %s; 幻图 in-process calls will be rejected", BuiltinCredentialID, existing.Status)
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	now := time.Now()
	credential := &model.APICredential{
		ID:         BuiltinCredentialID,
		Name:       "幻图内置调用（进程内）",
		KeyPreview: "in-process",
		KeyHash:    service.HashAPIKey("builtin-" + hex.EncodeToString(secret)),
		Status:     model.APICredentialStatusActive,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := credentials.Create(ctx, credential); err != nil {
		return nil, err
	}
	return credential, nil
}

func (a *App) Close() error {
	var firstErr error
	if a.cancel != nil {
		a.cancel()
	}
	if a.Redis != nil {
		if err := a.Redis.Close(); err != nil {
			firstErr = err
		}
	}
	if a.DB != nil {
		sqlDB, err := a.DB.DB()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
		} else if err := sqlDB.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
