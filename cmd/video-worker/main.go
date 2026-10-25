package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/service"
)

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func main() {
	if err := config.Load(); err != nil {
		exitWorker("configuration_load_failed", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger.Info("worker_event", "component", "worker", "worker", "video_render", "event", "executor_selected", "executor_mode", "ffmpeg")
	if err := service.RunVideoRenderWorker(ctx, config.Cfg.VideoWorkerConcurrency, config.Cfg.VideoWorkerTenantConcurrency); err != nil && err != context.Canceled {
		exitWorker("worker_failed", err)
	}
}

func exitWorker(event string, err error, attrs ...any) {
	fields := []any{"component", "worker", "worker", "video_render", "event", event}
	if err != nil {
		fields = append(fields, "error_type", fmt.Sprintf("%T", err))
	}
	logger.Error("worker_event", append(fields, attrs...)...)
	os.Exit(1)
}
