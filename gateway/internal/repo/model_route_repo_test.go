package repo

import (
	"context"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

func TestModelRouteRepositoryFallsBackWithoutDatabase(t *testing.T) {
	r := NewModelRouteRepository(nil)
	if r.hasRoutingTables() {
		t.Fatal("nil database must not report routing tables")
	}
	if r.tablesReady.Load() {
		t.Fatal("negative probe must not be cached as ready")
	}
	items, err := r.ListLogical(context.Background(), false)
	if err != nil || len(items) == 0 {
		t.Fatalf("fallback catalog: items=%d err=%v", len(items), err)
	}
	routes, err := r.ListRoutes(context.Background(), items[0].ID, false)
	if err != nil || len(routes) == 0 {
		t.Fatalf("fallback routes for %s: n=%d err=%v", items[0].ID, len(routes), err)
	}
	var nilRepo *ModelRouteRepository
	if nilRepo.hasRoutingTables() {
		t.Fatal("nil repository must not report routing tables")
	}
}
func TestListRoutesByLogicalMatchesListRoutes(t *testing.T) {
	r := NewModelRouteRepository(nil)
	ctx := context.Background()
	items, _ := r.ListLogical(ctx, false)
	ids := []string{" " + items[0].ID + " ", "not-a-model"}
	for _, item := range items {
		ids = append(ids, item.ID, item.ID)
	}
	for _, enabledOnly := range []bool{false, true} {
		grouped, err := r.ListRoutesByLogical(ctx, ids, enabledOnly)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := grouped["not-a-model"]; ok {
			t.Fatal("unknown id must be dropped")
		}
		for _, item := range items {
			want, _ := r.ListRoutes(ctx, item.ID, enabledOnly)
			got := grouped[item.ID]
			if len(got) != len(want) {
				t.Fatalf("%s: got %d routes want %d", item.ID, len(got), len(want))
			}
			for i := range want {
				if got[i].ID != want[i].ID {
					t.Fatalf("%s[%d]: got %s want %s", item.ID, i, got[i].ID, want[i].ID)
				}
			}
		}
	}
}

func TestGroupCanonicalRoutesFiltersAndSorts(t *testing.T) {
	r := NewModelRouteRepository(nil)
	items, _ := r.ListLogical(context.Background(), false)
	id := items[0].ID
	want, _ := r.ListRoutes(context.Background(), id, false)
	reversed := make([]model.ModelRoute, 0, len(want)+1)
	for i := len(want) - 1; i >= 0; i-- {
		reversed = append(reversed, want[i])
	}
	reversed = append(reversed, model.ModelRoute{ID: "rogue", LogicalModelID: "other"})
	got := groupCanonicalRoutes([]string{id}, reversed)
	if len(got) != 1 || len(got[id]) != len(want) {
		t.Fatalf("unexpected grouping: %+v", got)
	}
	for i := range want {
		if got[id][i].ID != want[i].ID {
			t.Fatalf("order mismatch at %d", i)
		}
	}
}
