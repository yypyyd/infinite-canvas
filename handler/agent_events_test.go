package handler

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestWriteAgentEventOmitsIDForEphemeralEvents(t *testing.T) {
	var buffer bytes.Buffer
	if err := writeAgentEvent(&buffer, model.AgentEvent{ID: "e1", RunID: "run", Type: model.AgentEventMessageStream, Payload: `{"content":"你好"}`}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buffer.String(), "id:") || !strings.HasPrefix(buffer.String(), "event: message.stream\n") || !strings.Contains(buffer.String(), `"content":"你好"`) {
		t.Fatalf("ephemeral frame = %q", buffer.String())
	}
	buffer.Reset()
	if err := writeAgentEvent(&buffer, model.AgentEvent{ID: "e2", RunID: "run", Sequence: 7, Type: model.AgentEventToolCall, Payload: `{}`}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buffer.String(), "id: 7\n") {
		t.Fatalf("persisted frame = %q", buffer.String())
	}
}

func TestAgentEventFallbackDelayStaysSlow(t *testing.T) {
	if agentEventFallbackDelay(0) < agentEventPollMax || agentEventFallbackDelay(10) > 2*agentEventPollMax {
		t.Fatalf("fallback delays = %v %v", agentEventFallbackDelay(0), agentEventFallbackDelay(10))
	}
}