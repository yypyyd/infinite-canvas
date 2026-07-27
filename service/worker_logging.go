package service

import (
	"fmt"
	"log/slog"
	"os"
)

var workerLogger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func logWorkerInfo(worker string, event string, attrs ...any) {
	workerLogger.Info("worker_event", workerLogAttrs(worker, event, attrs...)...)
}

func logWorkerError(worker string, event string, err error, attrs ...any) {
	if err != nil {
		attrs = append(attrs, "error_type", fmt.Sprintf("%T", err))
	}
	workerLogger.Error("worker_event", workerLogAttrs(worker, event, attrs...)...)
}

func workerLogAttrs(worker string, event string, attrs ...any) []any {
	return append([]any{"component", "worker", "worker", worker, "event", event}, attrs...)
}
