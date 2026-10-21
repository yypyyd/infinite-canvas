package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestRequestAgentUpstreamRetriesWhenProxyDropsConnection(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			// 模拟 Cloudflare 代理在出响应头前直接断开。
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好的\"},\"finish_reason\":null}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call-1\",\"type\":\"function\",\"function\":{\"name\":\"canvas.apply_ops\",\"arguments\":\"{\\\"ops\\\":[]}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	messages := []agentChatMessage{{Role: "user", Content: "hi"}}
	bodyValue := map[string]any{"model": "grok-4.6", "messages": messages, "tools": []any{map[string]any{"type": "function"}}, "stream": true}
	message, err := requestAgentUpstreamMessage(context.Background(), model.ModelChannel{BaseURL: server.URL, APIKey: "test"}, "req-retry", bodyValue, &messages, false, newAgentRunStreamPublisher("run-retry"))
	if err != nil {
		t.Fatalf("expected retries to recover, got %v", err)
	}
	if atomic.LoadInt32(&calls) != 3 || len(message.ToolCalls) != 1 {
		t.Fatalf("calls=%d message=%+v", calls, message)
	}
}

func TestIsAgentTransportError(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, fmt.Errorf("read: %w", io.ErrUnexpectedEOF), errors.New("read tcp: connection reset by peer")} {
		if !isAgentTransportError(err) {
			t.Fatalf("%v should be transport error", err)
		}
	}
	if isAgentTransportError(errors.New("agent upstream returned status 400")) || isAgentTransportError(context.DeadlineExceeded) {
		t.Fatal("non-transport errors must not be retried as transport errors")
	}
}
