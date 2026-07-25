package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestWorkerLoggerWritesStructuredFieldsWithoutRawError(t *testing.T) {
	var output bytes.Buffer
	previousLogger := workerLogger
	workerLogger = slog.New(slog.NewJSONHandler(&output, nil))
	t.Cleanup(func() { workerLogger = previousLogger })

	logWorkerError("batch_production", "executor_failed", errors.New("secret signed URL"), "organization_id", "organization-a", "item_id", "item-a")

	var entry map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatalf("decode worker log: %v", err)
	}
	if entry["msg"] != "worker_event" || entry["component"] != "worker" || entry["worker"] != "batch_production" || entry["event"] != "executor_failed" || entry["organization_id"] != "organization-a" || entry["item_id"] != "item-a" || entry["error_type"] != "*errors.errorString" {
		t.Fatalf("unexpected worker log: %#v", entry)
	}
	if strings.Contains(output.String(), "secret signed URL") {
		t.Fatalf("worker log exposed raw error: %s", output.String())
	}
}
