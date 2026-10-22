package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

func TestFilterPreferredProviderRoutes(t *testing.T) {
	routes := []model.ModelRoute{
		{ID: "1", Provider: "chatgpt"},
		{ID: "2", Provider: "grok"},
		{ID: "3", Provider: "byteplus"},
	}
	got := filterPreferredProviderRoutes(routes, "grok")
	if len(got) != 1 || got[0].Provider != "grok" {
		t.Fatalf("got %#v", got)
	}
	if len(filterPreferredProviderRoutes(routes, "")) != 3 {
		t.Fatal("empty preferred should keep all")
	}
}

func TestWithPreferredProviderRoundTrip(t *testing.T) {
	ctx := WithPreferredProvider(context.Background(), "BytePlus")
	if PreferredProviderFromContext(ctx) != "byteplus" {
		t.Fatalf("got %q", PreferredProviderFromContext(ctx))
	}
}

func TestRunMediaRouteFailoverSkipsBusyWithoutWait(t *testing.T) {
	routes := []model.ModelRoute{{ID: "a", Provider: "chatgpt"}, {ID: "b", Provider: "grok"}}
	calls := 0
	started := time.Now()
	_, _, err := runMediaRouteFailover(context.Background(), routes, "", func(ctx context.Context, route model.ModelRoute) ([]byte, string, error) {
		calls++
		return nil, "", ErrConcurrencyFull
	})
	elapsed := time.Since(started)
	if !errors.Is(err, ErrConcurrencyFull) {
		t.Fatalf("err=%v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("queued too long: %s", elapsed)
	}
}

func TestDefaultProviderPriorityPhase1(t *testing.T) {
	if defaultProviderPriority["chatgpt"] != 200 || defaultProviderPriority["adobe"] != 180 {
		t.Fatalf("%#v", defaultProviderPriority)
	}
}
