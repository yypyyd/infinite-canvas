package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/service"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func main() {
	if err := config.Load(); err != nil {
		exitWorker("configuration_load_failed", err)
	}
	executorURL := strings.TrimSpace(config.Cfg.BatchWorkerExecutorURL)
	var executor service.BatchProductionExecutor = service.StandardBatchProductionExecutor{}
	mode := "standard"
	if executorURL != "" {
		parsedURL, err := url.Parse(executorURL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" || parsedURL.User != nil {
			exitWorker("configuration_invalid", err, "setting", "BATCH_WORKER_EXECUTOR_URL")
		}
		if strings.TrimSpace(config.Cfg.BatchWorkerToken) == "" {
			exitWorker("configuration_invalid", nil, "setting", "BATCH_WORKER_TOKEN")
		}
		executor = service.HTTPBatchProductionExecutor{URL: executorURL, Token: config.Cfg.BatchWorkerToken}
		mode = "external"
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker_event", "component", "worker", "worker", "batch_production", "event", "executor_selected", "executor_mode", mode)
	if err := service.RunBatchProductionWorker(ctx, config.Cfg.BatchWorkerConcurrency, config.Cfg.BatchWorkerTenantConcurrency, executor); err != nil {
		exitWorker("worker_failed", err)
	}
}

func exitWorker(event string, err error, attrs ...any) {
	fields := []any{"component", "worker", "worker", "batch_production", "event", event}
	if err != nil {
		fields = append(fields, "error_type", fmt.Sprintf("%T", err))
	}
	logger.Error("worker_event", append(fields, attrs...)...)
	os.Exit(1)
}
