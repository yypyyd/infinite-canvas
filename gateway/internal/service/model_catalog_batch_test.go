package service

import (
	"errors"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

func TestRouteAccountCountIndexByRoute(t *testing.T) {
	idx := newRouteAccountCountIndex(true, []routeAccountCountRow{
		{RouteID: "r1", Pool: "chatgpt", Total: 3, Healthy: 2},
		{RouteID: "r1", Pool: "grok", Total: 5, Healthy: 1},
		{RouteID: "r2", Pool: "chatgpt", Total: 1, Healthy: 0},
	})
	if total, healthy := idx.lookup("r1", "chatgpt"); total != 3 || healthy != 2 {
		t.Fatalf("r1/chatgpt = %d/%d", total, healthy)
	}
	if total, healthy := idx.lookup("r1", "adobe"); total != 0 || healthy != 0 {
		t.Fatalf("other pool must not leak: %d/%d", total, healthy)
	}
}

func TestRouteAccountCountIndexByPool(t *testing.T) {
	idx := newRouteAccountCountIndex(false, []routeAccountCountRow{{Pool: "grok", Total: 4, Healthy: 3}})
	if total, healthy := idx.lookup("any-route", "grok"); total != 4 || healthy != 3 {
		t.Fatalf("pool-only lookup = %d/%d", total, healthy)
	}
	if total, _ := (routeAccountCountIndex{}).lookup("r", "p"); total != 0 {
		t.Fatal("empty index must report zero")
	}
}

func TestFilterRoutesWithAccountsKeepsOnlyServableRoutes(t *testing.T) {
	routes := []model.ModelRoute{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	got, err := filterRoutesWithAccounts(routes, func(r model.ModelRoute) (bool, error) { return r.ID != "b", nil })
	if err != nil || len(got) != 2 || got[0].ID != "a" || got[1].ID != "c" {
		t.Fatalf("got %+v err=%v", got, err)
	}
	none, _ := filterRoutesWithAccounts(routes, func(model.ModelRoute) (bool, error) { return false, nil })
	if len(none) != 0 {
		t.Fatal("model without usable accounts must have no routes")
	}
	if _, err := filterRoutesWithAccounts(routes, func(model.ModelRoute) (bool, error) { return false, errors.New("db down") }); err == nil {
		t.Fatal("account lookup errors must propagate")
	}
}
