package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type batchWorkerRoundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip batchWorkerRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestHTTPBatchProductionExecutorUsesRunScopedIdempotencyKey(t *testing.T) {
	user, _, organization := seedTenant(t, "external-batch-pricing")
	database, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := model.PricingSnapshot{Model: "image-model", Modality: "image", Operation: "generation", Unit: "image", ResolutionTier: "1k", Quantity: 1, BillingMode: "fixed", RuleCredits: 0, EffectiveRatio: 1, Source: "default", Credits: 0}
	job := model.BatchProductionJob{ID: "job-external-batch-pricing", OrganizationID: organization.ID, RequestID: "request-external-batch-pricing", Model: "image-model", Status: model.BatchProductionStatusRunning, CreatedBy: user.ID, CreditSource: model.CreditSourcePersonal, CreatedAt: now(), UpdatedAt: now()}
	item := model.BatchProductionItem{ID: "item-a", OrganizationID: organization.ID, JobID: job.ID, Operation: "generation", ResolutionTier: "1k", EstimatedCredits: 0, PricingSnapshot: snapshot, Status: model.BatchProductionStatusRunning, RunNumber: 3, CreatedAt: now(), UpdatedAt: now()}
	if err := database.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
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
	input := BatchProductionExecution{Job: job, Item: item}

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
	if result.GenerationTask == nil || result.GenerationTask.Credits != 0 || result.GenerationTask.PricingSnapshot != snapshot {
		t.Fatalf("external executor did not freeze pricing in a generation task: %#v", result.GenerationTask)
	}
}
