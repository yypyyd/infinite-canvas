package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
)

func stubGatewayHealth(t *testing.T, fn func(context.Context, time.Duration) (*gateway.Health, error)) {
	t.Helper()
	previous := gatewayHealthCheck
	gatewayHealthCheck = fn
	t.Cleanup(func() { gatewayHealthCheck = previous })
}

func TestCheckReadinessOmitsGatewayWhenDisabled(t *testing.T) {
	stubGatewayHealth(t, func(context.Context, time.Duration) (*gateway.Health, error) { return nil, nil })
	health, err := CheckReadiness(context.Background())
	if err != nil || health.Status != "ok" || health.Pool != nil {
		t.Fatalf("readiness = %#v, err=%v", health, err)
	}
	raw, _ := json.Marshal(health)
	if strings.Contains(string(raw), "pool") {
		t.Fatalf("disabled gateway leaked into response: %s", raw)
	}
}

func TestCheckReadinessFailsWhenGatewayDependencyIsDown(t *testing.T) {
	stubGatewayHealth(t, func(context.Context, time.Duration) (*gateway.Health, error) {
		return &gateway.Health{Status: "unavailable", Database: "ok", Redis: "unavailable"}, errors.New("gateway redis: dial tcp 10.0.0.1:6379: secret-host refused")
	})
	health, err := CheckReadiness(context.Background())
	if err == nil || health.Status != "unavailable" || health.Database != "ok" || health.Pool == nil || health.Pool.Redis != "unavailable" {
		t.Fatalf("readiness = %#v, err=%v", health, err)
	}
	raw, _ := json.Marshal(health)
	if strings.Contains(string(raw), "secret-host") || strings.Contains(string(raw), "10.0.0.1") {
		t.Fatalf("error details leaked: %s", raw)
	}
}

func TestCheckReadinessIsBoundedBySlowGateway(t *testing.T) {
	stubGatewayHealth(t, func(ctx context.Context, _ time.Duration) (*gateway.Health, error) {
		<-ctx.Done()
		return &gateway.Health{Status: "unavailable", Database: "unavailable", Redis: "unavailable"}, ctx.Err()
	})
	start := time.Now()
	health, err := CheckReadiness(context.Background())
	if elapsed := time.Since(start); elapsed > readinessTimeout+time.Second {
		t.Fatalf("readiness took %s", elapsed)
	}
	if err == nil || health.Status != "unavailable" {
		t.Fatalf("readiness = %#v, err=%v", health, err)
	}
}
