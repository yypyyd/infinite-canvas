package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

type batchWorkerRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip batchWorkerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestHTTPBatchProductionExecutorUsesRunScopedIdempotencyKey(t *testing.T) {
	var idempotencyKey string
	client := &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		idempotencyKey = request.Header.Get("Idempotency-Key")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"resultUrl":"https://cdn.example/result.png","mimeType":"image/png","size":10}`)),
			Request:    request,
		}, nil
	})}
	executor := HTTPBatchProductionExecutor{URL: "https://executor.example/run", Client: client}
	input := BatchProductionExecution{Item: model.BatchProductionItem{ID: "item-a", RunNumber: 3}}

	result, err := executor.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("execute batch production request: %v", err)
	}
	if idempotencyKey != "item-a:3" {
		t.Fatalf("idempotency key = %q, want %q", idempotencyKey, "item-a:3")
	}
	if result.ResultURL != "https://cdn.example/result.png" || result.MimeType != "image/png" || result.Size != 10 {
		t.Fatalf("unexpected executor result: %#v", result)
	}
}
