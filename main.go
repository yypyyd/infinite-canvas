package main

import (
	"context"
	"errors"
	"log"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/middleware"
	"github.com/yypyyd/infinite-canvas/router"
	"github.com/yypyyd/infinite-canvas/service"
)

func main() {
	if err := config.Load(); err != nil {
		log.Fatal(err)
	}
	if err := service.EnsureDefaultAdmin(); err != nil {
		log.Fatal(err)
	}
	engine := router.New()
	// The built-in account pool starts whenever PostgreSQL is available
	// (幻图's own database by default). Without it 幻图 runs as before and
	// the pool admin pages explain what is missing.
	switch {
	case gateway.Disabled():
		gateway.MountUnavailable(engine, middleware.AdminAuth, "内置账号池已通过 POOL_ENABLED=false 关闭")
		log.Printf("account pool disabled by POOL_ENABLED=false")
	default:
		module, err := gateway.Start(context.Background())
		switch {
		case errors.Is(err, gateway.ErrNotConfigured):
			gateway.MountUnavailable(engine, middleware.AdminAuth, "内置账号池需要 PostgreSQL：请使用 STORAGE_DRIVER=postgres，或设置 POOL_POSTGRES_DSN")
			log.Printf("account pool not started: %v", err)
		case err != nil:
			log.Fatalf("start account pool: %v", err)
		default:
			defer module.Close()
			gateway.InstallDefaultTransport()
			module.Mount(engine, middleware.AdminAuth)
			log.Printf("account pool enabled: admin %s, in-process channel %s", gateway.AdminPrefix, gateway.InternalBaseURL)
		}
	}
	if _, err := service.MigrateLegacyPoolPricingRules(); err != nil {
		log.Printf("pricing: legacy pool model migration skipped: %v", err)
	}
	service.StartPromptSyncScheduler()
	service.StartOrganizationEmailOutboxWorker()
	service.StartUserFileMaintenanceWorker()
	service.StartGenerationTaskMaintenanceWorker()
	log.Fatal(engine.Run(":" + config.Cfg.Port))
}
