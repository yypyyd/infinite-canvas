package gateway

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yypyyd/infinite-canvas/gateway/internal/bootstrap"
)

func TestCheckHealthTimesOutOnHangingRedis(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close() // accept but never answer
		}
	}()
	rdb := redis.NewClient(&redis.Options{Addr: ln.Addr().String(), MaxRetries: -1})
	defer rdb.Close()
	m := &Module{app: &bootstrap.App{Redis: rdb}}
	start := time.Now()
	h, err := m.CheckHealth(context.Background(), 300*time.Millisecond)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("health check took %s", elapsed)
	}
	if err == nil || h.Status != "unavailable" || h.Redis != "unavailable" || h.Database != "unavailable" {
		t.Fatalf("health = %#v, err=%v", h, err)
	}
}
