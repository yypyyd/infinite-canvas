package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// chatRequestTools extracts the OpenAI-compatible tools array from a chat
// completion body. Builtin ChatGPT/Grok text routes only speak plain GenerateText,
// so non-empty tools are turned into prompt instructions and parsed back into
// tool_calls (see chatPromptWithTools / parseEmulatedToolCalls).
func chatRequestTools(request map[string]any) []any {
	raw, ok := request["tools"].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]any, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func chatPromptWithTools(messages []any, tools []any) string {
	prompt := chatPrompt(messages)
	if len(tools) == 0 {
		return prompt
	}
	return strings.TrimSpace(prompt) + "\n\n" + emulatedToolInstructions(tools)
}

func emulatedToolInstructions(tools []any) string {
	var b strings.Builder
	b.WriteString("You can call tools via OpenAI-compatible function calling. ")
	b.WriteString("This environment supports image generation, canvas edits, and other tools listed below. ")
	b.WriteString("Never claim you cannot generate images or edit the canvas when a matching tool exists; call the tool instead. ")
	b.WriteString("When you need a tool, reply with ONLY a JSON object of this exact shape (no markdown, no commentary, no HTML):\n")
	b.WriteString(`{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"TOOL_NAME","arguments":"{\"key\":\"value\"}"}}]}`)
	b.WriteString("\nUse one or more entries in tool_calls. arguments MUST be a JSON string. ")
	b.WriteString("When no tool is needed, reply with plain assistant text only and do not mention tool_calls. Never emit HTML/UI markup.\n")
	b.WriteString("Available tools:\n")
	for _, raw := range tools {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		fn, _ := item["function"].(map[string]any)
		if fn == nil {
			fn = item
		}
		name := strings.TrimSpace(stringValue(fn["name"]))
		if name == "" {
			continue
		}
		desc := strings.TrimSpace(stringValue(fn["description"]))
		params, _ := json.Marshal(fn["parameters"])
		if desc == "" {
			desc = "no description"
		}
		if len(params) == 0 || string(params) == "null" {
			params = []byte("{}")
		}
		b.WriteString("- ")
		b.WriteString(name)
		b.WriteString(": ")
		b.WriteString(desc)
		b.WriteString(" parameters=")
		b.Write(params)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

var (
	emulatedToolJSONFence = regexp.MustCompile("(?is)```(?:json)?\\s*(\\{.*?\\})\\s*```")
	emulatedToolCallsKey  = regexp.MustCompile(`(?is)"tool_calls"\s*:`)
)

type emulatedToolCall struct {
	ID        string
	Type      string
	Name      string
	Arguments string
}

// parseEmulatedToolCalls extracts tool_calls from a GenerateText reply.
// Returns the calls and whether the whole reply should be treated as a tool turn
// (content cleared).
func parseEmulatedToolCalls(text string, allowed map[string]bool) (calls []emulatedToolCall, asToolTurn bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || !emulatedToolCallsKey.MatchString(trimmed) {
		return nil, false
	}
	candidates := make([]string, 0, 4)
	if match := emulatedToolJSONFence.FindStringSubmatch(trimmed); len(match) == 2 {
		candidates = append(candidates, strings.TrimSpace(match[1]))
	}
	if start := strings.Index(trimmed, "{"); start >= 0 {
		if end := strings.LastIndex(trimmed, "}"); end > start {
			candidates = append(candidates, trimmed[start:end+1])
		}
	}
	candidates = append(candidates, trimmed)
	for _, candidate := range candidates {
		parsed, ok := decodeEmulatedToolCalls(candidate, allowed)
		if ok && len(parsed) > 0 {
			return parsed, true
		}
	}
	return nil, false
}

func decodeEmulatedToolCalls(raw string, allowed map[string]bool) ([]emulatedToolCall, bool) {
	var payload struct {
		ToolCalls []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	if json.Unmarshal([]byte(raw), &payload) != nil || len(payload.ToolCalls) == 0 {
		return nil, false
	}
	out := make([]emulatedToolCall, 0, len(payload.ToolCalls))
	for i, item := range payload.ToolCalls {
		name := strings.TrimSpace(item.Function.Name)
		if name == "" {
			return nil, false
		}
		if allowed != nil && !toolNameAllowed(name, allowed) {
			return nil, false
		}
		args := normalizeToolArguments(item.Function.Arguments)
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = fmt.Sprintf("call_%d", i+1)
		}
		typ := strings.TrimSpace(item.Type)
		if typ == "" {
			typ = "function"
		}
		out = append(out, emulatedToolCall{ID: id, Type: typ, Name: name, Arguments: args})
	}
	return out, true
}

func toolNameAllowed(name string, allowed map[string]bool) bool {
	if allowed[name] {
		return true
	}
	if allowed[strings.ReplaceAll(name, ".", "_")] {
		return true
	}
	if allowed[strings.ReplaceAll(name, "_", ".")] {
		return true
	}
	return false
}

func normalizeToolArguments(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return "{}"
	}
	if raw[0] == '"' {
		var unquoted string
		if json.Unmarshal(raw, &unquoted) == nil {
			if strings.TrimSpace(unquoted) == "" {
				return "{}"
			}
			return unquoted
		}
	}
	if json.Valid(raw) {
		return string(raw)
	}
	return "{}"
}

func allowedToolNames(tools []any) map[string]bool {
	allowed := map[string]bool{}
	for _, raw := range tools {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		fn, _ := item["function"].(map[string]any)
		if fn == nil {
			fn = item
		}
		name := strings.TrimSpace(stringValue(fn["name"]))
		if name == "" {
			continue
		}
		allowed[name] = true
		allowed[strings.ReplaceAll(name, ".", "_")] = true
		allowed[strings.ReplaceAll(name, "_", ".")] = true
	}
	return allowed
}

// sanitizeGeneratedChatText strips ChatGPT web UI fragments that leak into
// GenerateText replies (e.g. broken image-card markup) so the canvas agent
// does not show truncated HTML to users.
func sanitizeGeneratedChatText(text string) string {
	text = strings.ReplaceAll(text, "\u0000", "")
	re := regexp.MustCompile(`(?s)图片视觉参考\s*"\s*/\s*>`)
	text = re.ReplaceAllString(text, "")
	re = regexp.MustCompile(`(?s)<[^>]*(?:img|image|visual|reference)[^>]*>`)
	text = re.ReplaceAllString(text, "")
	re = regexp.MustCompile(`[ \t]+\n`)
	text = re.ReplaceAllString(text, "\n")
	re = regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func openAIToolCallResponse(modelName string, calls []emulatedToolCall, stream bool) (http.Header, io.ReadCloser) {
	id := "chatcmpl-" + randomUpper(16)
	created := time.Now().Unix()
	toolCalls := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"id":   call.ID,
			"type": call.Type,
			"function": map[string]any{
				"name":      call.Name,
				"arguments": call.Arguments,
			},
		})
	}
	contentType := "application/json"
	var raw []byte
	if !stream {
		raw, _ = json.Marshal(map[string]any{
			"id": id, "object": "chat.completion", "created": created, "model": modelName,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":       "assistant",
					"content":    nil,
					"tool_calls": toolCalls,
				},
				"logprobs": nil, "finish_reason": "tool_calls",
			}},
			"usage": map[string]any{"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
		})
	} else {
		contentType = "text/event-stream"
		parts := make([]string, 0, len(toolCalls)+3)
		first, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": modelName,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
		})
		parts = append(parts, "data: "+string(first))
		for index, call := range toolCalls {
			fn, _ := call["function"].(map[string]any)
			chunk, _ := json.Marshal(map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": modelName,
				"choices": []map[string]any{{
					"index": 0,
					"delta": map[string]any{
						"tool_calls": []map[string]any{{
							"index": index,
							"id":    call["id"],
							"type":  call["type"],
							"function": map[string]any{
								"name":      fn["name"],
								"arguments": fn["arguments"],
							},
						}},
					},
					"finish_reason": nil,
				}},
			})
			parts = append(parts, "data: "+string(chunk))
		}
		last, _ := json.Marshal(map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": created, "model": modelName,
			"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
		})
		parts = append(parts, "data: "+string(last), "data: [DONE]")
		raw = []byte(strings.Join(parts, "\n\n") + "\n\n")
	}
	return http.Header{"Content-Type": []string{contentType}}, io.NopCloser(bytes.NewReader(raw))
}

func reshapeGenerateTextWithTools(modelName, text string, tools []any, stream bool) (http.Header, io.ReadCloser) {
	text = sanitizeGeneratedChatText(text)
	if calls, ok := parseEmulatedToolCalls(text, allowedToolNames(tools)); ok {
		return openAIToolCallResponse(modelName, calls, stream)
	}
	return openAITextResponse(modelName, text, stream)
}

// generateTextHasToolCalls reports whether a GenerateText reply already contains
// emulated tool_calls that reshapeGenerateTextWithTools would honor.
func generateTextHasToolCalls(text string, tools []any) bool {
	if len(tools) == 0 {
		return false
	}
	_, ok := parseEmulatedToolCalls(sanitizeGeneratedChatText(text), allowedToolNames(tools))
	return ok
}

// forcedToolRetryPrompt asks the model again when it answered in plain text
// despite tools being available (common ChatGPT GenerateText failure mode).
func forcedToolRetryPrompt(originalPrompt, previousText string, tools []any) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(originalPrompt))
	b.WriteString("\n\nCRITICAL CORRECTION: Your previous reply did not call any tool. ")
	b.WriteString("Plain-text claims that you already generated an image or edited the canvas are invalid in this API. ")
	b.WriteString("You MUST reply with ONLY the tool_calls JSON object now (no markdown, no HTML, no assistant prose). ")
	b.WriteString("If the user asked for an image, call image.generate or image_generate with a concrete prompt. ")
	b.WriteString("Available tool names: ")
	names := make([]string, 0, len(tools))
	for _, raw := range tools {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		fn, _ := item["function"].(map[string]any)
		if fn == nil {
			fn = item
		}
		name := strings.TrimSpace(stringValue(fn["name"]))
		if name != "" {
			names = append(names, name)
		}
	}
	b.WriteString(strings.Join(names, ", "))
	b.WriteString(".\nPrevious invalid reply (do not repeat):\n")
	b.WriteString(truncateForToolRetry(previousText, 500))
	return b.String()
}

func truncateForToolRetry(text string, maxRunes int) string {
	text = strings.TrimSpace(sanitizeGeneratedChatText(text))
	if maxRunes <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

// chatToolChoiceForceHint strengthens the prompt when the client asked for
// required / specific tool_choice (agent does this for image intents).
func chatToolChoiceForceHint(request map[string]any) string {
	raw, ok := request["tool_choice"]
	if !ok || raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		if strings.EqualFold(strings.TrimSpace(v), "required") {
			return "\nYou MUST call at least one tool via the tool_calls JSON object. Do not answer with plain text."
		}
	case map[string]any:
		fn, _ := v["function"].(map[string]any)
		name := strings.TrimSpace(stringValue(fn["name"]))
		if name == "" {
			name = strings.TrimSpace(stringValue(v["name"]))
		}
		if name != "" {
			return "\nYou MUST call tool \"" + name + "\" via the tool_calls JSON object now. Do not answer with plain text or claim the work is already done."
		}
		if typ := strings.TrimSpace(stringValue(v["type"])); strings.EqualFold(typ, "required") || strings.EqualFold(typ, "any") {
			return "\nYou MUST call at least one tool via the tool_calls JSON object. Do not answer with plain text."
		}
	}
	return ""
}
