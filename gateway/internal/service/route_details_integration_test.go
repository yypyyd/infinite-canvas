package service

import (
	"context"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

func TestRouteDetailsCountsBoundAvailableAndCooldownIntegration(t *testing.T) {
	db := optimizationTestDB(t)
	ctx := context.Background()
	svc := NewAdminConsoleService(nil, db, repo.NewModelRepository(db), repo.NewTokenRepository(db), nil, nil)
	find := func(details []RouteDetail, id string) (RouteDetail, bool) {
		for _, detail := range details {
			if detail.RouteID == id {
				return detail, true
			}
		}
		return RouteDetail{}, false
	}
	before, err := svc.RouteDetails(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) == 0 {
		t.Skip("no seeded routes")
	}
	route := before[0]
	future := time.Now().Add(time.Hour)
	accounts := []struct {
		id       string
		status   string
		cooldown *time.Time
	}{{"rd-ok", "active", nil}, {"rd-cool", "active", &future}, {"rd-off", "disabled", nil}}
	for _, item := range accounts {
		if db.Create(&model.TokenAccount{ID: item.id, Pool: route.Provider, Status: item.status, Value: "test"}).Error != nil {
			t.Fatal("create account")
		}
		if db.Create(&model.AccountModelRoute{ID: "amr-" + item.id, AccountID: item.id, ModelRouteID: route.RouteID, Enabled: true, Entitled: true, CooldownUntil: item.cooldown}).Error != nil {
			t.Fatal("create binding")
		}
	}
	after, err := svc.RouteDetails(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := find(after, route.RouteID)
	if !ok {
		t.Fatal("route missing")
	}
	if got.BoundAccounts-route.BoundAccounts != 3 || got.AvailableAccounts-route.AvailableAccounts != 1 || got.CooldownAccounts-route.CooldownAccounts != 1 {
		t.Fatalf("before=%+v after=%+v", route, got)
	}
	if got.EffectivePriority <= 0 || got.EffectiveWeight <= 0 || !got.ProviderEnabled {
		t.Fatalf("schedule = %+v", got)
	}
}
