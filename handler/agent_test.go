package handler

import (
	"testing"
	"time"
)

func TestAgentEventPollDelayBacksOffAndCaps(t *testing.T) {
	cases := []struct {
		idle int
		want time.Duration
	}{
		{idle: 0, want: 200 * time.Millisecond},
		{idle: 1, want: 400 * time.Millisecond},
		{idle: 2, want: 800 * time.Millisecond},
		{idle: 3, want: time.Second},
		{idle: 20, want: time.Second},
	}
	for _, item := range cases {
		if got := agentEventPollDelay(item.idle); got != item.want {
			t.Fatalf("idle=%d delay=%s, want %s", item.idle, got, item.want)
		}
	}
}
