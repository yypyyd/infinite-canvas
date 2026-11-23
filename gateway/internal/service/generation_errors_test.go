package service

import (
	"context"
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/internal/errtext"
)

func TestImageFailureSurvivesRecoveryAndReplay(t *testing.T) {
	for _, detail := range []*errtext.Failure{{Status: 403, Detail: "Access Denied for Image"}, {Status: 401}, {Status: 429, Detail: "Rate Limit Reached"}} {
		stored := detail.Stored()
		message := safeStoredGenerationError(stored)
		if message != detail.Error() {
			t.Fatal(message)
		}
		_, err := new(V1Service).waitForImageResult(context.Background(), nil, "id", "", map[string]any{"status": "failed", "error": message, "error_status": detail.Status})
		var reported *errtext.Failure
		if !errors.As(err, &reported) || reported.Status != detail.Status || reported.Error() != message {
			t.Fatalf("replay error=%v", err)
		}
		job := newAsyncImageJob("fixture")
		job.complete(nil, err)
		result, pending := asyncImageJobResponse(job, "id", "/tasks")
		if pending || result["error"] != message || result["error_status"] != detail.Status {
			t.Fatal(result)
		}
	}
}
