package service

import (
	"strings"
	"testing"
)

func TestParseAgentCompletionStreamAccumulatesContentAndToolCalls(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"好的，"}}]}`,
		`data: {"choices":[{"delta":{"content":"马上排版"}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-a","type":"function","function":{"name":"canvas_add_text","arguments":"{\"text\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"标题\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call-b","function":{"name":"canvas_connect","arguments":"{\"fromNodeId\":\"a\","}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\"toNodeId\":\"b\"}"}}]}}]}`,
		`data: [DONE]`,
	}, "\n\n")
	contents := []string{}
	ready := []string{}
	message, err := parseAgentCompletionStream(strings.NewReader(stream), func(text string) { contents = append(contents, text) }, func(index int, call agentUpstreamToolCall) {
		ready = append(ready, call.Function.Name+":"+call.Function.Arguments)
	})
	if err != nil {
		t.Fatal(err)
	}
	if message.Content != "好的，马上排版" || len(contents) != 2 || contents[1] != "好的，马上排版" {
		t.Fatalf("content = %q deltas=%#v", message.Content, contents)
	}
	if len(message.ToolCalls) != 2 || message.ToolCalls[0].ID != "call-a" || message.ToolCalls[0].Type != "function" || message.ToolCalls[1].Type != "function" || message.ToolCalls[1].Function.Arguments != `{"fromNodeId":"a","toNodeId":"b"}` {
		t.Fatalf("tool calls = %#v", message.ToolCalls)
	}
	// 第一个调用在第二个调用开始时就已完整，应当立即回调，而不是等到流结束。
	if len(ready) != 2 || ready[0] != `canvas_add_text:{"text":"标题"}` {
		t.Fatalf("ready = %#v", ready)
	}
}

func TestParseAgentCompletionStreamHandlesWholeMessageAndErrors(t *testing.T) {
	message, err := parseAgentCompletionStream(strings.NewReader(`data: {"choices":[{"message":{"content":"完成","tool_calls":[{"id":"x","function":{"name":"canvas_read","arguments":"{}"}}]}}]}`+"\n\ndata: [DONE]\n\n"), nil, nil)
	if err != nil || message.Content != "完成" || len(message.ToolCalls) != 1 || message.ToolCalls[0].Type != "function" {
		t.Fatalf("message = %#v err=%v", message, err)
	}
	if _, err := parseAgentCompletionStream(strings.NewReader(`data: {"error":{"message":"rate limited"}}`+"\n\n"), nil, nil); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("stream error = %v", err)
	}
	if _, err := parseAgentCompletionStream(strings.NewReader("data: {broken\n\n"), nil, nil); err == nil {
		t.Fatal("broken chunk should fail")
	}
}

func TestParseAgentCompletionJSONFallback(t *testing.T) {
	message, err := parseAgentCompletionJSON([]byte(`{"choices":[{"message":{"content":"你好","tool_calls":[]}}]}`))
	if err != nil || message.Content != "你好" {
		t.Fatalf("message = %#v err=%v", message, err)
	}
	if _, err := parseAgentCompletionJSON([]byte(`{"choices":[]}`)); err == nil {
		t.Fatal("empty choices should fail")
	}
}

func TestAgentToolAllowsParallelExcludesConfirmationAndPlanning(t *testing.T) {
	for _, name := range []string{"canvas.add_text", "canvas.connect", "canvas.apply_ops", "image.generate"} {
		if !agentToolAllowsParallel(name) {
			t.Fatalf("%s should run in parallel", name)
		}
	}
	for _, name := range []string{"canvas.plan", "agent.ask_user", "canvas.delete", "canvas.update_text", "agent.remember", "unknown.tool"} {
		if agentToolAllowsParallel(name) {
			t.Fatalf("%s must not run in parallel", name)
		}
	}
}

func TestAgentDefaultBudgetAllowsLongerRuns(t *testing.T) {
	budget, err := normalizeAgentRunBudget(AgentRunBudget{})
	if err != nil || budget.MaxToolCalls != 20 {
		t.Fatalf("budget = %#v err=%v", budget, err)
	}
	if _, err := normalizeAgentRunBudget(AgentRunBudget{MaxToolCalls: 30}); err != nil {
		t.Fatalf("30 tool calls should be allowed: %v", err)
	}
	if _, err := normalizeAgentRunBudget(AgentRunBudget{MaxToolCalls: 31}); err == nil {
		t.Fatal("31 tool calls should be rejected")
	}
}
