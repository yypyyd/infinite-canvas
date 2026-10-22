package service

import (
	"context"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

type mapSettings map[string]string

func (m mapSettings) GetValue(_ context.Context, key string) (string, error) { return m[key], nil }

func gptImageRoutes(t *testing.T) []model.ModelRoute {
	t.Helper()
	routes, err := repo.NewModelRepository(nil).Routes().MatchRoutes(context.Background(), "gpt-image-2", model.RouteRequirements{Operation: "generation", Ratio: "1:1", Resolution: "1K"})
	if err != nil || len(routes) != 3 {
		t.Fatalf("routes=%d err=%v, want 3", len(routes), err)
	}
	return routes
}

func providerOrder(routes []model.ModelRoute) []string {
	out := make([]string, len(routes))
	for i, route := range routes {
		out[i] = route.Provider
	}
	return out
}

func withShuffle(t *testing.T, values ...float64) {
	t.Helper()
	previous := routeShuffle
	i := 0
	routeShuffle = func() float64 { v := values[i%len(values)]; i++; return v }
	t.Cleanup(func() { routeShuffle = previous })
}

func TestOrderRoutesUsesBuiltinProviderDefaults(t *testing.T) {
	got, err := orderRoutes(context.Background(), nil, gptImageRoutes(t))
	if err != nil {
		t.Fatal(err)
	}
	if order := providerOrder(got); order[0] != "chatgpt" || order[1] != "byteplus" || order[2] != "adobe" {
		t.Fatalf("order=%v, want chatgpt, byteplus, adobe", order)
	}
}

func TestOrderRoutesProviderSettingOverridesDefault(t *testing.T) {
	settings := mapSettings{providerPrioritySettingKey("adobe"): "250"}
	got, err := orderRoutes(context.Background(), settings, gptImageRoutes(t))
	if err != nil {
		t.Fatal(err)
	}
	if order := providerOrder(got); order[0] != "adobe" || order[1] != "chatgpt" {
		t.Fatalf("order=%v, want adobe first", order)
	}
	// Out-of-range values are ignored, falling back to the built-in default.
	got, _ = orderRoutes(context.Background(), mapSettings{providerPrioritySettingKey("adobe"): "99999"}, gptImageRoutes(t))
	if providerOrder(got)[0] != "chatgpt" {
		t.Fatalf("invalid setting must be ignored: %v", providerOrder(got))
	}
}

func TestOrderRoutesPerRouteOverrideBeatsProvider(t *testing.T) {
	routes := gptImageRoutes(t)
	high := 250
	for i := range routes {
		if routes[i].Provider == "byteplus" {
			routes[i].Priority = &high
		}
	}
	got, err := orderRoutes(context.Background(), mapSettings{}, routes)
	if err != nil {
		t.Fatal(err)
	}
	if order := providerOrder(got); order[0] != "byteplus" || order[1] != "chatgpt" || order[2] != "adobe" {
		t.Fatalf("order=%v, want byteplus override first", order)
	}
	schedules, _ := routeSchedules(context.Background(), nil, got[:1])
	if schedules[0].Priority != 250 || schedules[0].PriorityInherited || !schedules[0].WeightInherited {
		t.Fatalf("schedule=%+v", schedules[0])
	}
}

func TestOrderRoutesEqualPriorityWeightedSplit(t *testing.T) {
	settings := mapSettings{
		providerPrioritySettingKey("chatgpt"):  "50",
		providerPrioritySettingKey("byteplus"): "90",
		providerPrioritySettingKey("adobe"):    "90",
		providerWeightSettingKey("byteplus"):   "3",
		providerWeightSettingKey("adobe"):      "1",
	}
	// Total weight 4; the group is in ID order (adobe, byteplus): draws below
	// 0.25 pick adobe (1/4), the rest pick byteplus (3/4).
	for _, test := range []struct {
		draw  float64
		first string
	}{{0.0, "adobe"}, {0.24, "adobe"}, {0.26, "byteplus"}, {0.99, "byteplus"}} {
		withShuffle(t, test.draw)
		got, err := orderRoutes(context.Background(), settings, gptImageRoutes(t))
		if err != nil {
			t.Fatal(err)
		}
		order := providerOrder(got)
		if order[0] != test.first || order[2] != "chatgpt" {
			t.Fatalf("draw=%v order=%v, want %s first and chatgpt last", test.draw, order, test.first)
		}
	}
	// Statistical check with the real generator: roughly a 3:1 split.
	routeShuffle = defaultRouteShuffleForTest
	counts := map[string]int{}
	for i := 0; i < 4000; i++ {
		got, _ := orderRoutes(context.Background(), settings, gptImageRoutes(t))
		counts[got[0].Provider]++
	}
	if share := float64(counts["byteplus"]) / 4000; share < 0.68 || share > 0.82 {
		t.Fatalf("byteplus share=%.3f, want ~0.75 (%v)", share, counts)
	}
}
