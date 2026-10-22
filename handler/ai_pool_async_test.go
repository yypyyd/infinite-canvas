package handler

import (
	"testing"
	"time"
)

func TestBuiltinPoolUsesAsyncImageTasks(t *testing.T) {
	for _, path := range []string{"/images/generations", "/images/edits"} {
		if !supportsAsyncImageTasks("http://nexa.internal/v1", path) {
			t.Fatalf("pool must use async image tasks for %s", path)
		}
	}
	if supportsAsyncImageTasks("http://nexa.internal/v1", "/videos") {
		t.Fatal("videos keep their own task flow")
	}
	if supportsAsyncImageTasks("https://example.com/v1", "/images/generations") {
		t.Fatal("third-party channels unchanged")
	}
	if got := imageTaskRecoveryWindow("http://nexa.internal/v1/images/generations"); got < 20*time.Minute {
		t.Fatalf("pool recovery window too short: %v", got)
	}
	if got := imageTaskRecoveryWindow("https://api.huantu.xyz/v1/images/generations"); got != 7*time.Minute {
		t.Fatalf("other channels unchanged, got %v", got)
	}
}
