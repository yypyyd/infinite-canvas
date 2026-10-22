package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

func TestImageRerouteSafeOnlyBeforeAcceptance(t *testing.T) {
	for _, err := range []error{ErrNoProviderAccount, ErrProviderAuth, ErrProviderQuota, ErrConcurrencyFull, fmt.Errorf("wrap: %w", ErrProviderUnsupported)} {
		if !imageRerouteSafe(err) {
			t.Fatalf("%v must be reroutable", err)
		}
	}
	for _, err := range []error{nil, ErrGenerationAccepted, fmt.Errorf("upstream: %w", ErrGenerationAccepted), fmt.Errorf("provider request failed")} {
		if imageRerouteSafe(err) {
			t.Fatalf("%v must not be reroutable", err)
		}
	}
	job := newAsyncImageJob("fp")
	job.err = ErrNoProviderAccount
	close(job.done)
	failed, _ := asyncImageJobResponse(job, "req", "/v1/images/tasks?request_id=req")
	if failed["reroutable"] != true {
		t.Fatalf("status must expose reroutable: %v", failed)
	}
}

func TestFailPendingSettlesOnlyOnceIntegration(t *testing.T) {
	db := optimizationTestDB(t)
	events := repo.NewEventRepository(db)
	ctx := context.Background()
	id := fmt.Sprintf("ev%d", time.Now().UnixNano())[:20]
	if err := db.Create(&model.EventLog{ID: id, RequestID: "abandon-" + id, TS: time.Now(), Kind: "image", Status: "pending"}).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Delete(&model.EventLog{}, "id = ?", id) })
	if ok, err := events.FailPending(ctx, id, ErrGenerationAbandoned.Error()); err != nil || !ok {
		t.Fatalf("first abandon: %v %v", ok, err)
	}
	if ok, _ := events.FailPending(ctx, id, "again"); ok {
		t.Fatal("a settled event must not change again")
	}
	svc := &V1Service{events: events}
	if svc.eventStillPending(ctx, id) || !svc.eventAbandoned(ctx, id) {
		t.Fatal("recovery must see the abandoned event and skip delivery")
	}
}
