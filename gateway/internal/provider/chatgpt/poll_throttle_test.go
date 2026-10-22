package chatgpt

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
)

type sequenceTestClient struct {
	tlsclient.HttpClient
	responses []func() *http.Response
	calls     int
}

func (c *sequenceTestClient) Do(*http.Request) (*http.Response, error) {
	index := min(c.calls, len(c.responses)-1)
	c.calls++
	return c.responses[index](), nil
}

func TestPollForImageRetriesThrottledConversationReads(t *testing.T) {
	throttled := func() *http.Response {
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"detail":"Too many requests"}`))}
	}
	ready := func() *http.Response {
		body := `{"mapping":{"n":{"message":{"author":{"role":"tool"},"content":{"parts":["file-service://file-abc123"]}}}}}`
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
	}
	session := &sequenceTestClient{responses: []func() *http.Response{throttled, ready}}
	started := time.Now()
	files, _, err := NewClient("").pollForImage(context.Background(), session, "test-token", "conv-1", nil, nil, nil, time.Minute)
	if err != nil {
		t.Fatalf("a 429 on an accepted task's poll must be retried, got %v", err)
	}
	if len(files) != 1 || files[0] != "file-abc123" || session.calls < 2 {
		t.Fatalf("files=%v calls=%d", files, session.calls)
	}
	if time.Since(started) > 40*time.Second {
		t.Fatal("backoff too long")
	}
}
