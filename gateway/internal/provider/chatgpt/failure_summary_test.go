package chatgpt

import (
	"errors"
	"testing"
)

func TestFailureSummaryDropsBodies(t *testing.T) {
	err := AcceptedFailure("conv", ensureOK(429, []byte(`{"detail":"Too many requests","token":"secret-value"}`), "conversation_get"))
	if got := FailureSummary(err); got != "rate_limited conversation_get 429" {
		t.Fatalf("got %q", got)
	}
	if got := FailureSummary(errors.New("dial tcp https://x/?access_token=secret")); got != "error" {
		t.Fatalf("got %q", got)
	}
}
