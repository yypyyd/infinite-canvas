package service

import (
	"encoding/json"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func stubRouteDetails(t *testing.T, items []gateway.RouteDetail) {
	t.Helper()
	previous := poolRouteDetails
	poolRouteDetails = func() ([]gateway.RouteDetail, bool) { return items, true }
	t.Cleanup(func() { poolRouteDetails = previous })
}

func overviewModel(t *testing.T, overview ModelOverview, id string) ModelOverviewItem {
	t.Helper()
	for _, item := range overview.Models {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("model %s missing", id)
	return ModelOverviewItem{}
}

func TestModelOverviewOrdersSourcesPoolFirstByDefault(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	routes := []gateway.RouteDetail{
		{LogicalModelID: "gpt-image-2", LogicalEnabled: true, RouteID: "r1", Provider: "chatgpt", Enabled: true, ProviderEnabled: true, EffectivePriority: 100, EffectiveWeight: 1, BoundAccounts: 3, AvailableAccounts: 2, CooldownAccounts: 1},
		{LogicalModelID: "gpt-image-2", LogicalEnabled: true, RouteID: "r2", Provider: "adobe", Enabled: true, ProviderEnabled: false, EffectivePriority: 80, EffectiveWeight: 1},
		{LogicalModelID: "flux-x", LogicalEnabled: true, RouteID: "r3", Provider: "byteplus", Enabled: true, ProviderEnabled: true, EffectivePriority: 90, EffectiveWeight: 1},
	}
	third := thirdImageChannel("third", 5, 0)
	third.Models[0].ResolutionTiers = []string{"1k"}
	third = normalizeModelChannel(third)
	third.ID = "ch1"
	off := normalizeModelChannel(thirdImageChannel("off", 1, 0))
	off.ID, off.Enabled = "ch2", false
	settings := normalizeSettings(model.Settings{
		Public:  model.PublicSetting{ModelChannel: model.PublicModelChannelSetting{Models: []model.ModelDefinition{{ID: "gpt-image-2", Modality: "image", Enabled: true, Operations: []string{"generation"}}}}},
		Private: model.PrivateSetting{Channels: []model.ModelChannel{off, third}},
	})
	overview := buildModelOverview(settings, routes, true)
	if !overview.Pool.Enabled || overview.Pool.Priority != DefaultBuiltinPoolPriority || overview.Pool.Weight != 1 {
		t.Fatalf("pool = %+v", overview.Pool)
	}
	item := overviewModel(t, overview, "gpt-image-2")
	if len(item.Sources) != 3 || item.Sources[0].Kind != "pool" || item.Sources[0].Rank != 1 || item.Sources[1].ChannelName != "third" || item.Sources[1].Rank != 2 || item.Sources[2].Active {
		t.Fatalf("sources = %+v", item.Sources)
	}
	if !item.Available || len(item.Sources[0].Routes) != 2 || !item.Sources[0].Routes[0].Available || item.Sources[0].Routes[1].Available {
		t.Fatalf("item = %+v", item)
	}
	if item.Sources[0].Routes[0].CooldownAccounts != 1 || item.Sources[2].Reasons[0] != "渠道已停用" {
		t.Fatalf("route/reasons = %+v", item.Sources)
	}
	if len(item.Gaps) == 0 || len(item.Warnings) == 0 {
		t.Fatalf("expected pricing gaps: %+v", item)
	}
	// A pool model the gateway knows but cannot serve right now.
	flux := overviewModel(t, overview, "flux-x")
	if flux.Available || flux.Managed || len(flux.Sources) != 1 || flux.Sources[0].Active || flux.Sources[0].Reasons[0] != "没有可用的平台路由" {
		t.Fatalf("flux = %+v", flux)
	}
}

func TestModelOverviewHonoursPoolSourceSettings(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	third := normalizeModelChannel(thirdImageChannel("third", 1, 150))
	third.ID = "ch1"
	settings := normalizeSettings(model.Settings{Private: model.PrivateSetting{Channels: []model.ModelChannel{third},
		BuiltinPool: &model.BuiltinPoolSource{Models: []model.BuiltinPoolModelSource{{Model: "kling-3", Disabled: true}}}}})
	overview := buildModelOverview(settings, nil, false)
	img := overviewModel(t, overview, "gpt-image-2")
	if img.Sources[0].ChannelName != "third" || img.Sources[1].Kind != "pool" || img.Sources[1].Rank != 2 {
		t.Fatalf("sources = %+v", img.Sources)
	}
	kling := overviewModel(t, overview, "kling-3")
	if kling.Sources[0].Active || kling.Sources[0].Enabled {
		t.Fatalf("kling = %+v", kling.Sources)
	}
}

func TestOptionalIntDistinguishesAbsentAndNull(t *testing.T) {
	var patch ModelSourcePatch
	if err := json.Unmarshal([]byte(`{"kind":"pool","priority":null,"weight":5}`), &patch); err != nil {
		t.Fatal(err)
	}
	if !patch.Priority.Set || patch.Priority.Value != nil || !patch.Weight.Set || *patch.Weight.Value != 5 {
		t.Fatalf("patch = %+v", patch)
	}
	var empty ModelSourcePatch
	_ = json.Unmarshal([]byte(`{"kind":"pool"}`), &empty)
	if empty.Priority.Set || empty.Weight.Set {
		t.Fatal("absent fields must not be set")
	}
}

func TestPatchModelSourceUpdatesChannelPoolAndRoute(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, now()) })
	settings := saved
	settings.Private.Storage.Driver, settings.Private.Storage.LocalPath = "local", t.TempDir()
	third := thirdImageChannel("third", 1, 0)
	third.ID = "ch-patch"
	settings.Private.Channels = []model.ModelChannel{third}
	settings.Private.BuiltinPool = nil
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		t.Fatal(err)
	}
	p := func(v int) OptionalInt { return OptionalInt{Set: true, Value: &v} }
	off := false
	steps := []ModelSourcePatch{
		{Kind: "channel", ChannelID: "ch-patch", Model: "gpt-image-2", Priority: p(300), Weight: p(7)},
		{Kind: "channel", ChannelID: "ch-patch", Priority: p(20)},
		{Kind: "pool", Priority: p(50)},
		{Kind: "pool", Model: "kling-3", Enabled: &off},
	}
	for _, step := range steps {
		if err := PatchModelSource(step); err != nil {
			t.Fatalf("%+v: %v", step, err)
		}
	}
	got, _ := repository.GetSettings()
	channel := got.Private.Channels[0]
	if channel.Priority != 20 || *channel.Models[0].Priority != 300 || *channel.Models[0].Weight != 7 {
		t.Fatalf("channel = %+v", channel)
	}
	if got.Private.BuiltinPool == nil || *got.Private.BuiltinPool.Priority != 50 || len(got.Private.BuiltinPool.Models) != 1 || !got.Private.BuiltinPool.Models[0].Disabled {
		t.Fatalf("pool = %+v", got.Private.BuiltinPool)
	}
	// Clearing an override with null.
	if err := PatchModelSource(ModelSourcePatch{Kind: "channel", ChannelID: "ch-patch", Model: "gpt-image-2", Priority: OptionalInt{Set: true}}); err != nil {
		t.Fatal(err)
	}
	got, _ = repository.GetSettings()
	if got.Private.Channels[0].Models[0].Priority != nil {
		t.Fatal("override not cleared")
	}
	// Selection follows the saved settings: channel model 20 < pool 50.
	selection, err := SelectModelChannelExcluding(PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"}, nil)
	if err != nil || selection.Channel.ID != BuiltinPoolChannelID {
		t.Fatalf("selection = %+v err=%v", selection.Channel, err)
	}
	if err := PatchModelSource(ModelSourcePatch{Kind: "pool", Priority: p(5000)}); err == nil {
		t.Fatal("out-of-range priority accepted")
	}
	if err := PatchModelSource(ModelSourcePatch{Kind: "channel", ChannelID: "missing", Priority: p(1)}); err == nil {
		t.Fatal("missing channel accepted")
	}
	// Pool routes go through the gateway.
	var gotRoute gateway.RouteUpdate
	previous := updatePoolRoute
	updatePoolRoute = func(logical, route string, update gateway.RouteUpdate) error {
		if logical != "gpt-image-2" || route != "r1" {
			t.Fatalf("route %s/%s", logical, route)
		}
		gotRoute = update
		return nil
	}
	t.Cleanup(func() { updatePoolRoute = previous })
	if err := PatchModelSource(ModelSourcePatch{Kind: "pool-route", LogicalModelID: "gpt-image-2", RouteID: "r1", Priority: OptionalInt{Set: true}, Weight: p(9)}); err != nil {
		t.Fatal(err)
	}
	if !gotRoute.ClearPriority || gotRoute.Weight == nil || *gotRoute.Weight != 9 || gotRoute.ClearWeight {
		t.Fatalf("route update = %+v", gotRoute)
	}
}
