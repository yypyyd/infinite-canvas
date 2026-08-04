package main

import (
	"log"

	"github.com/yypyyd/infinite-canvas/config"
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
	service.StartPromptSyncScheduler()
	service.StartOrganizationEmailOutboxWorker()
	service.StartUserFileMaintenanceWorker()
	log.Fatal(router.New().Run(":" + config.Cfg.Port))
}
