package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
)

func TestFlatProvidersReplaceLegacyPool(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	channels := runtimeChannelsFor(model.PrivateSetting{})
	if len(channels) != 4 {
		t.Fatalf("want 4 providers, got %#v", channels)
	}
	seen := map[string]int{}
	for _, ch := range channels {
		p := BuiltinProviderOf(ch)
		if p == "" || !ch.Enabled {
			t.Fatalf("channel %#v", ch)
		}
		seen[p] = ch.Priority
	}
	if seen["chatgpt"] != 200 || seen["grok"] != 200 || seen["byteplus"] != 190 || seen["adobe"] != 180 {
		t.Fatalf("priorities %#v", seen)
	}
}

func TestMigrateBuiltinProvidersFromLegacyPool(t *testing.T) {
	private := model.PrivateSetting{
		BuiltinPool:      &model.BuiltinPoolSource{Enabled: boolPtr(true), Priority: intPtr(50), Weight: intPtr(3)},
		PoolModelAliases: []model.PoolModelAlias{{Name: "img", Target: "gpt-image-2", Enabled: true}},
	}
	providers := migrateBuiltinProviders(private)
	if len(providers) != 4 {
		t.Fatalf("providers %#v", providers)
	}
	for _, p := range providers {
		if p.Priority == nil || *p.Priority != 50 || p.Weight == nil || *p.Weight != 3 {
			t.Fatalf("migrated %#v", p)
		}
	}
	// Non-empty BuiltinProviders wins over legacy.
	custom := []model.BuiltinProviderSource{{Provider: "chatgpt", Enabled: boolPtr(true), Priority: intPtr(9)}}
	got := migrateBuiltinProviders(model.PrivateSetting{BuiltinProviders: custom, BuiltinPool: private.BuiltinPool})
	if len(got) < 1 || got[0].Priority == nil || *got[0].Priority != 9 {
		t.Fatalf("custom %#v", got)
	}
}

func TestDisabledProviderSkipped(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	private := model.PrivateSetting{
		BuiltinProviders: []model.BuiltinProviderSource{
			{Provider: "chatgpt", Enabled: boolPtr(false)},
			{Provider: "grok", Enabled: boolPtr(true), Priority: intPtr(10)},
			{Provider: "byteplus", Enabled: boolPtr(false)},
			{Provider: "adobe", Enabled: boolPtr(false)},
		},
	}
	channels := runtimeChannelsFor(private)
	if len(channels) != 1 || BuiltinProviderOf(channels[0]) != "grok" {
		t.Fatalf("channels %#v", channels)
	}
}

func TestPreferredProviderForChannel(t *testing.T) {
	ch := model.ModelChannel{ID: BuiltinProviderChannelID("byteplus"), BaseURL: gateway.InternalBaseURL}
	if PreferredProviderForChannel(ch) != "byteplus" {
		t.Fatalf("got %q", PreferredProviderForChannel(ch))
	}
	if PreferredProviderForChannel(model.ModelChannel{Name: "third", BaseURL: "https://x"}) != "" {
		t.Fatal("third-party should be empty")
	}
}

func TestSelectFixedModelChannelByID(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	// Wire settings through normalize so runtime channels exist — use selections helper.
	private := normalizePrivateSetting(model.PrivateSetting{})
	channels := runtimeChannelsFor(private)
	request := PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"}
	selections := modelChannelSelectionsForRequest(channels, request)
	if len(selections) == 0 {
		t.Fatal("no selections")
	}
	want := BuiltinProviderChannelID("adobe")
	found := false
	for _, s := range selections {
		if s.Channel.ID == want {
			found = true
		}
	}
	if !found {
		t.Fatalf("adobe missing in %#v", selections)
	}
	// Direct pick by id among selections (SelectFixed uses channelSelectionsFor which needs DB;
	// assert ID match path here).
	var matched ModelChannelSelection
	for _, s := range selections {
		if s.Channel.ID == want || s.Channel.Name == BuiltinProviderChannelName("adobe") {
			matched = s
			break
		}
	}
	if BuiltinProviderOf(matched.Channel) != "adobe" {
		t.Fatalf("matched %#v", matched)
	}
}

func TestProviderPriorityFailoverOrder(t *testing.T) {
	stubPoolModels(t, testPoolModels)
	third := normalizeModelChannel(thirdImageChannel("third", 1000, 0))
	private := model.PrivateSetting{Channels: []model.ModelChannel{third}}
	selections := modelChannelSelectionsForRequest(runtimeChannelsFor(private), PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"})
	got, ok := pickChannelSelection(selections, func(int) int { return 0 })
	if !ok || !IsBuiltinProviderChannel(got.Channel) {
		t.Fatalf("expected builtin provider first, got %#v", got)
	}
	// Exclude all builtin providers → third party.
	excluded := []string{}
	for _, s := range selections {
		if IsBuiltinProviderChannel(s.Channel) {
			excluded = append(excluded, s.Channel.Name)
		}
	}
	rest := modelChannelSelectionsExcluding(selections, excluded)
	got, ok = pickChannelSelection(rest, nil)
	if !ok || got.Channel.Name != "third" {
		t.Fatalf("failover %#v rest=%d", got, len(rest))
	}
}
