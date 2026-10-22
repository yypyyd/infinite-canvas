package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func poolTestChannels(t *testing.T, pool *model.BuiltinPoolSource, saved ...model.ModelChannel) []ModelChannelSelection {
	t.Helper()
	stubPoolModels(t, testPoolModels)
	for i := range saved {
		saved[i] = normalizeModelChannel(saved[i])
	}
	private := normalizePrivateSetting(model.PrivateSetting{Channels: saved, BuiltinPool: pool})
	return modelChannelSelectionsForRequest(runtimeChannelsFor(private), PricingRequest{Model: "gpt-image-2", Modality: "image", Operation: "generation"})
}

func thirdImageChannel(name string, weight, priority int) model.ModelChannel {
	return model.ModelChannel{Name: name, BaseURL: "https://" + name + ".example.com", APIKey: "k", Weight: weight, Priority: priority, Enabled: true,
		Models: []model.ChannelModel{{Model: "gpt-image-2", Modality: "image", Operations: []string{"generation"}}}}
}

// Without any saved source settings the order must equal the pre-priority
// behaviour: pool first, then a weighted pick among third-party channels.
func TestDefaultSourceOrderKeepsPoolFirst(t *testing.T) {
	selections := poolTestChannels(t, nil, thirdImageChannel("a", 1000, 0), thirdImageChannel("b", 1, 0))
	for draw := 0; draw < 3; draw++ {
		got, ok := pickChannelSelection(selections, func(int) int { return draw })
		if !ok || got.Channel.ID != BuiltinPoolChannelID {
			t.Fatalf("draw %d picked %q", draw, got.Channel.Name)
		}
	}
	rest := modelChannelSelectionsExcluding(selections, []string{BuiltinPoolChannelName})
	for draw, want := range map[int]string{0: "a", 999: "a", 1000: "b"} {
		got, _ := pickChannelSelection(rest, func(total int) int {
			if total != 1001 {
				t.Fatalf("total = %d", total)
			}
			return draw
		})
		if got.Channel.Name != want {
			t.Fatalf("draw %d picked %q want %q", draw, got.Channel.Name, want)
		}
	}
}

func TestChannelPriorityAbovePoolGoesFirst(t *testing.T) {
	selections := poolTestChannels(t, nil, thirdImageChannel("a", 1, 200))
	if got, _ := pickChannelSelection(selections, nil); got.Channel.Name != "a" {
		t.Fatalf("picked %q", got.Channel.Name)
	}
	rest := modelChannelSelectionsExcluding(selections, []string{"a"})
	if got, _ := pickChannelSelection(rest, nil); got.Channel.ID != BuiltinPoolChannelID {
		t.Fatalf("failover picked %q", got.Channel.Name)
	}
}

func TestPoolSourceSettingsChangeOrderAndWeight(t *testing.T) {
	// Pool lowered to the channel group: weighted pick across both.
	selections := poolTestChannels(t, &model.BuiltinPoolSource{Priority: intPtr(0), Weight: intPtr(3)}, thirdImageChannel("a", 1, 0))
	seen := map[string]bool{}
	for draw := 0; draw < 4; draw++ {
		got, _ := pickChannelSelection(selections, func(total int) int {
			if total != 4 {
				t.Fatalf("total = %d", total)
			}
			return draw
		})
		seen[got.Channel.Name] = true
	}
	if !seen["a"] || !seen[BuiltinPoolChannelName] {
		t.Fatalf("seen = %v", seen)
	}
	// Disabled pool source drops out entirely.
	if got := poolTestChannels(t, &model.BuiltinPoolSource{Enabled: boolPtr(false)}, thirdImageChannel("a", 1, 0)); len(got) != 1 || got[0].Channel.Name != "a" {
		t.Fatalf("selections = %#v", got)
	}
	// A per-model override beats the pool-wide value.
	override := &model.BuiltinPoolSource{Models: []model.BuiltinPoolModelSource{{Model: "gpt-image-2", Priority: intPtr(0)}}}
	selections = poolTestChannels(t, override, thirdImageChannel("a", 1, 50))
	if got, _ := pickChannelSelection(selections, nil); got.Channel.Name != "a" {
		t.Fatalf("picked %q", got.Channel.Name)
	}
	disabled := &model.BuiltinPoolSource{Models: []model.BuiltinPoolModelSource{{Model: "gpt-image-2", Disabled: true}}}
	if got := poolTestChannels(t, disabled, thirdImageChannel("a", 1, 0)); len(got) != 1 {
		t.Fatalf("selections = %#v", got)
	}
}

func TestChannelModelOverridesAndDisable(t *testing.T) {
	a := thirdImageChannel("a", 1, 0)
	a.Models[0].Priority = intPtr(500)
	b := thirdImageChannel("b", 1, 0)
	b.Models[0].Disabled = true
	selections := poolTestChannels(t, nil, a, b)
	if len(selections) != 2 {
		t.Fatalf("disabled model still selectable: %#v", selections)
	}
	if got, _ := pickChannelSelection(selections, nil); got.Channel.Name != "a" {
		t.Fatalf("picked %q", got.Channel.Name)
	}
}

func TestNormalizeBuiltinPoolSourceClamps(t *testing.T) {
	if normalizeBuiltinPoolSource(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	got := normalizeBuiltinPoolSource(&model.BuiltinPoolSource{Priority: intPtr(5000), Weight: intPtr(0), Models: []model.BuiltinPoolModelSource{{Model: " x "}, {Model: "y", Weight: intPtr(-1)}}})
	if *got.Priority != SourcePriorityMax || *got.Weight != SourceWeightMin || len(got.Models) != 1 || got.Models[0].Model != "y" || *got.Models[0].Weight != 1 {
		t.Fatalf("got %#v", got)
	}
}
