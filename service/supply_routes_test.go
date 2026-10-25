package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestListSupplyRoutesUnifiesProvidersAndChannels(t *testing.T) {
	off := false
	priority := 42
	private := model.PrivateSetting{
		BuiltinProviders: []model.BuiltinProviderSource{
			{Provider: "chatgpt", Enabled: boolPtr(true), Priority: intPtr(200)},
			{Provider: "adobe", Enabled: &off, Priority: &priority},
		},
		Channels: []model.ModelChannel{
			{ID: "ch-lec", Name: "LecAPI", Protocol: "openai", Enabled: true, Priority: 10, Weight: 2, Models: []model.ChannelModel{{Model: "seedance"}}},
			{ID: BuiltinProviderChannelID("chatgpt"), Name: "ignore-saved-provider", Enabled: true},
		},
	}
	routes := ListSupplyRoutes(private)
	if len(routes) < 5 {
		// chatgpt, grok, byteplus, adobe + lec (migrate fills missing providers)
		t.Fatalf("routes=%d %+v", len(routes), routes)
	}
	byID := map[string]SupplyRoute{}
	for _, r := range routes {
		byID[r.ID] = r
	}
	chatgpt := byID[BuiltinProviderChannelID("chatgpt")]
	if chatgpt.Kind != "provider" || !chatgpt.Enabled || chatgpt.Priority != 200 {
		t.Fatalf("chatgpt=%+v", chatgpt)
	}
	adobe := byID[BuiltinProviderChannelID("adobe")]
	if adobe.Kind != "provider" || adobe.Enabled {
		t.Fatalf("adobe should be disabled: %+v", adobe)
	}
	lec := byID["ch-lec"]
	if lec.Kind != "channel" || !lec.Enabled || lec.ModelCount != 1 || lec.Priority != 10 {
		t.Fatalf("lec=%+v", lec)
	}
	// Highest priority first
	if routes[0].Priority < routes[len(routes)-1].Priority {
		t.Fatalf("expected priority desc, first=%d last=%d", routes[0].Priority, routes[len(routes)-1].Priority)
	}
}

func TestIsProviderSourceKind(t *testing.T) {
	if !IsProviderSourceKind("provider") || !IsProviderSourceKind("pool") || IsProviderSourceKind("channel") {
		t.Fatal("kind helpers")
	}
}
