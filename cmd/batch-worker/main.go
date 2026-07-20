package main

import (
	"context"
	"log"
	"net/url"
	"os/signal"
	"strings"
	"syscall"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/service"
)

func main() {
	if err := config.Load(); err != nil { log.Fatal(err) }
	executorURL := strings.TrimSpace(config.Cfg.BatchWorkerExecutorURL)
	parsedURL, err := url.Parse(executorURL)
	if executorURL == "" || err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" { log.Fatal("BATCH_WORKER_EXECUTOR_URL must be a valid HTTP/HTTPS URL") }
	if strings.TrimSpace(config.Cfg.BatchWorkerToken) == "" { log.Fatal("BATCH_WORKER_TOKEN is required") }
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	executor := service.HTTPBatchProductionExecutor{URL: executorURL, Token: config.Cfg.BatchWorkerToken}
	if err := service.RunBatchProductionWorker(ctx, config.Cfg.BatchWorkerConcurrency, executor); err != nil { log.Fatal(err) }
}
