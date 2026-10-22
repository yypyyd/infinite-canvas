package service

import (
	"errors"
	"fmt"
	"testing"
)

func TestAcceptedSyncImageJobStaysPendingForPollers(t *testing.T) {
	job := newAsyncImageJob("fp")
	job.complete(map[string]any{"status": "in_progress"}, ErrGenerationAccepted)
	response, pending := asyncImageJobResponse(job, "req-1", "/v1/images/tasks?request_id=req-1")
	if !pending || response["status"] != "queued" {
		t.Fatalf("accepted job must poll as pending, got pending=%v status=%v", pending, response["status"])
	}
	if !imageJobStillPending(fmt.Errorf("wrapped: %w", ErrGenerationAccepted)) {
		t.Fatal("wrapped ErrGenerationAccepted must stay pending")
	}
	failed := newAsyncImageJob("fp")
	failed.complete(nil, ErrProviderExecution)
	response, pending = asyncImageJobResponse(failed, "req-2", "")
	if pending || response["status"] != "failed" {
		t.Fatalf("real failure must surface, got pending=%v status=%v", pending, response["status"])
	}
	if imageJobStillPending(errors.New("boom")) {
		t.Fatal("plain error is not pending")
	}
}
