package service

import (
	"context"
	"testing"
	"time"
)

func TestLocalConcurrencyWithoutRedis(t *testing.T) {
	c := NewConcurrencyService(nil)
	ctx := context.Background()
	now := time.Unix(1000, 0)
	c.local.now = func() time.Time { return now }
	if ok, err := c.Acquire(ctx, "k", 1, "a"); !ok || err != nil {
		t.Fatalf("first acquire = %v, %v", ok, err)
	}
	if ok, _ := c.Acquire(ctx, "k", 1, "b"); ok {
		t.Fatal("second acquire should be rejected at max=1")
	}
	if got := c.ActiveCount(ctx, "k"); got != 1 {
		t.Fatalf("active = %d", got)
	}
	c.Release(ctx, "k", "a")
	if ok, _ := c.Acquire(ctx, "k", 1, "b"); !ok {
		t.Fatal("acquire after release should succeed")
	}
	now = now.Add(time.Duration(c.ttl+1) * time.Second)
	if ok, _ := c.Acquire(ctx, "k", 1, "c"); !ok {
		t.Fatal("expired slot should be pruned")
	}
	for want := uint64(0); want < 3; want++ {
		if got, ok := c.NextCursor(ctx, "rr"); !ok || got != want {
			t.Fatalf("cursor = %d,%v want %d", got, ok, want)
		}
	}
}
