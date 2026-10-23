package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func boolPointer(value bool) *bool { return &value }

func TestResolveAgentModelFallsBackToToolCapableModel(t *testing.T) {
	models := []model.ModelDefinition{
		{ID: "gpt-6-t-mini", Modality: "text", Enabled: true, Sort: 1, SupportsTools: boolPointer(false)},
		{ID: "grok-4.5", Modality: "text", Enabled: true, Sort: 2, SupportsTools: boolPointer(true)},
		{ID: "grok-4.6", Modality: "text", Enabled: true, Sort: 3, SupportsTools: boolPointer(true)},
		{ID: "legacy-text", Modality: "text", Enabled: true, Sort: 0},
	}
	cases := map[string]string{"gpt-6-t-mini": "grok-4.6", "grok-4.5": "grok-4.5", "legacy-text": "legacy-text", "outside-catalog": "outside-catalog"}
	for requested, want := range cases {
		got, err := resolveAgentModelFrom(models, requested)
		if err != nil || got != want {
			t.Fatalf("resolve %s = %q, %v; want %q", requested, got, err, want)
		}
	}
	models[2].Enabled = false
	if got, _ := resolveAgentModelFrom(models, "gpt-6-t-mini"); got != "grok-4.5" {
		t.Fatalf("fallback without grok-4.6 = %q", got)
	}
	if _, err := resolveAgentModelFrom([]model.ModelDefinition{{ID: "a", Modality: "text", Enabled: true, SupportsTools: boolPointer(false)}}, "a"); err == nil {
		t.Fatal("expected error when no tool-capable model exists")
	}
}

func TestKeepModelToolSupportPreservesSavedFlag(t *testing.T) {
	saved := model.Settings{Public: model.PublicSetting{ModelChannel: model.PublicModelChannelSetting{Models: []model.ModelDefinition{{ID: "a", SupportsTools: boolPointer(false)}, {ID: "b", SupportsTools: boolPointer(false)}}}}}
	next := model.Settings{Public: model.PublicSetting{ModelChannel: model.PublicModelChannelSetting{Models: []model.ModelDefinition{{ID: "a"}, {ID: "b", SupportsTools: boolPointer(true)}, {ID: "c"}}}}}
	keepModelToolSupport(&next, saved)
	models := next.Public.ModelChannel.Models
	if models[0].SupportsTools == nil || *models[0].SupportsTools || !*models[1].SupportsTools || models[2].SupportsTools != nil {
		t.Fatalf("unexpected flags: %+v", models)
	}
}

// 真实样本：cliproxy grok-4.6 在 stream=true 时把工具调用写成文本并以 stop 结束。
const fakeToolCallStream = `data: {"id":"e545e4b4","object":"chat.completion.chunk","model":"grok-4.6","choices":[{"index":0,"delta":{"role":"assistant","content":"已创建三列故事板（开场/冲突/结局横向排列）。 \n` + "```xml" + `\n<function call create_text_node with text is 开场 x is 50 y is 100>\n"},"finish_reason":null,"native_finish_reason":null}]}

data: {"id":"e545e4b4","object":"chat.completion.chunk","model":"grok-4.6","choices":[{"index":0,"delta":{"role":"assistant","content":"` + "```" + `"},"finish_reason":null}]}

data: {"id":"e545e4b4","object":"chat.completion.chunk","model":"grok-4.6","choices":[{"index":0,"delta":{},"finish_reason":"stop","native_finish_reason":"stop"}]}

data: [DONE]

`

func TestAgentStreamNeedsPlainRetryDetectsFakeToolCalls(t *testing.T) {
	message, err := parseAgentCompletionStream(strings.NewReader(fakeToolCallStream), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	withTools := map[string]any{"tools": []any{}}
	if !agentStreamNeedsPlainRetry(withTools, message) {
		t.Fatalf("fake tool call text should trigger retry: %q", message.Content)
	}
	markup := agentUpstreamMessage{Content: "建议采用三列横向故事板}>\n <icon name=\"sunrise\" size=\"xl\">"}
	if !agentStreamNeedsPlainRetry(withTools, markup) {
		t.Fatal("UI markup should trigger retry")
	}
	if agentStreamNeedsPlainRetry(withTools, agentUpstreamMessage{Content: "已完成三列故事板。"}) {
		t.Fatal("plain reply must not retry")
	}
	if agentStreamNeedsPlainRetry(map[string]any{}, message) {
		t.Fatal("requests without tools must not retry")
	}
	if agentStreamNeedsPlainRetry(withTools, agentUpstreamMessage{Content: message.Content, ToolCalls: []agentUpstreamToolCall{{ID: "x"}}}) {
		t.Fatal("real tool calls must not retry")
	}
}

func TestRequestAgentUpstreamRetriesPlainAfterFakeStream(t *testing.T) {
	var streams []bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		stream, _ := body["stream"].(bool)
		streams = append(streams, stream)
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, fakeToolCallStream)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"canvas.apply_ops","arguments":"{\"ops\":[]}"}}]},"finish_reason":"tool_calls"}]}`)
	}))
	defer server.Close()
	messages := []agentChatMessage{{Role: "user", Content: "hi"}}
	bodyValue := map[string]any{"model": "grok-4.6", "messages": messages, "tools": []any{map[string]any{"type": "function"}}, "stream": true}
	message, err := requestAgentUpstreamMessage(context.Background(), model.ModelChannel{BaseURL: server.URL, APIKey: "test"}, "req-1", bodyValue, &messages, false, newAgentRunStreamPublisher("run-test"))
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 2 || !streams[0] || streams[1] {
		t.Fatalf("stream sequence = %v", streams)
	}
	if len(message.ToolCalls) != 1 || message.ToolCalls[0].Function.Name != "canvas.apply_ops" {
		t.Fatalf("message = %+v", message)
	}
}
