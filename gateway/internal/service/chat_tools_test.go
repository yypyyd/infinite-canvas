package service

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestParseEmulatedToolCalls(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "image.generate",
				"description": "Generate an image",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}
	allowed := allowedToolNames(tools)
	raw := "{\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"image.generate\",\"arguments\":\"{\\\"prompt\\\":\\\"a girl\\\"}\"}}]}"
	calls, ok := parseEmulatedToolCalls(raw, allowed)
	if !ok || len(calls) != 1 {
		t.Fatalf("parse failed: ok=%v calls=%v", ok, calls)
	}
	if calls[0].Name != "image.generate" {
		t.Fatalf("name=%q", calls[0].Name)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Arguments), &args); err != nil {
		t.Fatal(err)
	}
	if args["prompt"] != "a girl" {
		t.Fatalf("args=%v", args)
	}
}

func TestParseEmulatedToolCallsObjectArguments(t *testing.T) {
	allowed := map[string]bool{"canvas.apply_ops": true}
	raw := "{\"tool_calls\":[{\"function\":{\"name\":\"canvas.apply_ops\",\"arguments\":{\"ops\":[{\"op\":\"add\"}]}}}]}"
	calls, ok := parseEmulatedToolCalls(raw, allowed)
	if !ok || len(calls) != 1 {
		t.Fatalf("parse failed: ok=%v", ok)
	}
	if !strings.Contains(calls[0].Arguments, "add") {
		t.Fatalf("args=%s", calls[0].Arguments)
	}
}

func TestSanitizeGeneratedChatText(t *testing.T) {
	marker := "图片视觉参考"
	in := "prefix " + marker + "\" /> trailing description"
	out := sanitizeGeneratedChatText(in)
	if strings.Contains(out, marker) {
		t.Fatalf("not sanitized: %q", out)
	}
	if !strings.Contains(out, "prefix") || !strings.Contains(out, "trailing") {
		t.Fatalf("lost surrounding text: %q", out)
	}
}

func TestReshapeGenerateTextWithTools(t *testing.T) {
	tools := []any{
		map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "image.generate", "parameters": map[string]any{}},
		},
	}
	raw := "```json\n{\"tool_calls\":[{\"id\":\"c1\",\"type\":\"function\",\"function\":{\"name\":\"image.generate\",\"arguments\":\"{}\"}}]}\n```"
	h, body := reshapeGenerateTextWithTools("gpt-6", raw, tools, false)
	if !strings.Contains(h.Get("Content-Type"), "json") {
		t.Fatalf("ctype=%s", h.Get("Content-Type"))
	}
	b, _ := io.ReadAll(body)
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatal(err)
	}
	choices := payload["choices"].([]any)
	msg := choices[0].(map[string]any)["message"].(map[string]any)
	if msg["tool_calls"] == nil {
		t.Fatalf("missing tool_calls: %s", string(b))
	}
	if choices[0].(map[string]any)["finish_reason"] != "tool_calls" {
		t.Fatalf("finish=%v", choices[0].(map[string]any)["finish_reason"])
	}
}

func TestChatPromptWithToolsIncludesCatalog(t *testing.T) {
	tools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "image.generate",
				"description": "Create image",
				"parameters":  map[string]any{"type": "object"},
			},
		},
	}
	prompt := chatPromptWithTools([]any{map[string]any{"role": "user", "content": "draw a girl"}}, tools)
	if !strings.Contains(prompt, "image.generate") || !strings.Contains(prompt, "tool_calls") {
		t.Fatalf("prompt missing tools: %s", prompt)
	}
}

func TestGenerateTextHasToolCalls(t *testing.T) {
	tools := []any{map[string]any{"type": "function", "function": map[string]any{"name": "image_generate"}}}
	if generateTextHasToolCalls("already generated an image for you", tools) {
		t.Fatal("plain success prose must not count as tool calls")
	}
	emulated := `{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"image_generate","arguments":"{\"prompt\":\"girl\"}"}}]}`
	if !generateTextHasToolCalls(emulated, tools) {
		t.Fatal("emulated tool_calls JSON should parse")
	}
}

func TestChatToolChoiceForceHint(t *testing.T) {
	hint := chatToolChoiceForceHint(map[string]any{"tool_choice": map[string]any{"type": "function", "function": map[string]any{"name": "image_generate"}}})
	if hint == "" || !strings.Contains(hint, "image_generate") {
		t.Fatalf("expected forced tool hint, got %q", hint)
	}
}
