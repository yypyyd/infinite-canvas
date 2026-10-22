package main

import (
	"context"
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
	if gateway.Enabled() {
		module, err := gateway.Start(context.Background())
		if err != nil {
			log.Fatalf("start gateway: %v", err)
		}
		defer module.Close()
		gateway.InstallDefaultTransport()
		module.Mount(engine, middleware.AdminAuth)
		log.Printf("gateway enabled: public %s, admin %s, built-in channel %s", gateway.PublicV1Prefix, gateway.AdminPrefix, gateway.InternalBaseURL)
	}
	service.StartPromptSyncScheduler()
	service.StartOrganizationEmailOutboxWorker()
	service.StartUserFileMaintenanceWorker()
	service.StartGenerationTaskMaintenanceWorker()
	log.Fatal(engine.Run(":" + config.Cfg.Port))
}
