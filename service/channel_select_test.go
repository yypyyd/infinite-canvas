package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestPickChannelSelectionPrefersPoolThenWeight(t *testing.T) {
	a := ModelChannelSelection{Channel: model.ModelChannel{Name: "a", Weight: 1}}
	b := ModelChannelSelection{Channel: model.ModelChannel{Name: "b", Weight: 3}}
	pool := ModelChannelSelection{Channel: model.ModelChannel{ID: BuiltinPoolChannelID, Name: BuiltinPoolChannelName, Weight: 1, Priority: DefaultBuiltinPoolPriority}}
	if got, ok := pickChannelSelection([]ModelChannelSelection{a, pool, b}, func(int) int { return 0 }); !ok || got.Channel.Name != BuiltinPoolChannelName {
		t.Fatalf("pool must win, got %q", got.Channel.Name)
	}
	for draw, want := range map[int]string{0: "a", 1: "b", 3: "b"} {
		got, ok := pickChannelSelection([]ModelChannelSelection{a, b}, func(total int) int {
			if total != 4 {
				t.Fatalf("total=%d", total)
			}
			return draw
		})
		if !ok || got.Channel.Name != want {
			t.Fatalf("draw %d picked %q, want %q", draw, got.Channel.Name, want)
		}
	}
	if _, ok := pickChannelSelection(nil, nil); ok {
		t.Fatal("empty selections must report no channel")
	}
	zero := ModelChannelSelection{Channel: model.ModelChannel{Name: "z"}}
	if got, ok := pickChannelSelection([]ModelChannelSelection{zero}, nil); !ok || got.Channel.Name != "z" {
		t.Fatal("zero-weight channels must still be selectable")
	}
}
