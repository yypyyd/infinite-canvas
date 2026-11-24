package chatgpt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	baseURL                  = "https://chatgpt.com"
	defaultUserAgent         = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36 Edg/149.0.0.0"
	defaultClientVersion     = "prod-db390ebea64862bf1899c420a4c736e0cf639747"
	defaultClientBuildNumber = "7904904"
	defaultPOWScript         = "https://chatgpt.com/backend-api/sentinel/sdk.js"
	// sseAsyncGrace bounds how long startImageGeneration keeps reading the SSE
	// waiting for the image_gen_async marker after the conversation id is known.
	// The marker normally arrives within ~1s; when it never streams (intermittent
	// on gpt-6-t-mini) we must not hold the stream open for the whole ctx
	// budget, so we break after this grace and fall through to polling.
	sseAsyncGrace = 10 * time.Second
)

var (
	fileServiceIDPattern = regexp.MustCompile(`file-service://([A-Za-z0-9_-]+)`)
	sedimentIDPattern    = regexp.MustCompile(`sediment://([A-Za-z0-9_-]+)`)
	realImageIDPattern   = regexp.MustCompile(`\bfile_00000000[a-f0-9]{24}\b`)
	conversationIDRE     = regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`)
	// skipped_mainline is currently sent inside a JSON string in the SSE
	// payload. Depending on the upstream serializer it can be compact, contain
	// whitespace, or have one or more escaping backslashes before the quotes.
	// Matching the exact compact spelling caused valid delegated image tasks to
	// be rejected as "no async marker".
	skippedMainlineRE = regexp.MustCompile(`\\*"skipped_mainline\\*"\s*:\s*true`)
	scriptSrcRE       = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)
	dataBuildPathRE   = regexp.MustCompile(`c/[^/]*/_`)
	htmlDataBuildRE   = regexp.MustCompile(`<html[^>]*data-build="([^"]*)"`)

	// asyncMarkers signal that ChatGPT accepted the prompt and switched to the
	// async image pipeline (image is delivered later via conversation polling
	// rather than inline in the SSE stream). Their presence means "generating —
	// keep polling", NOT failure.
	asyncMarkers = []string{
		"image_gen_async",
		"image_gen_task_id",
		"trigger_async_ux",
	}

	// contentPolicyMarkers are stable substrings of ChatGPT's content-audit
	// refusal message. When one appears in an assistant turn the prompt was
	// rejected upstream — no image will ever arrive, so we must fail fast
	// instead of polling until timeout.
	contentPolicyMarkers = []string{
		"this request may violate our content polic",
		"this prompt may violate our content polic",
		"may violate our content policies",
		"i can't help with",
		"i can\u2019t help with",
		"i cannot help with",
		"i can't assist with",
		"i can\u2019t assist with",
		"i cannot assist with",
		"i'm unable to help with",
		"i\u2019m unable to help with",
		"can't create images",
		"can\u2019t create images",
		"cannot create images",
		"can't generate images",
		"can\u2019t generate images",
		"cannot generate images",
		"unable to create images",
		"unable to generate images",
	}
)

// containsAsyncMarker reports whether the SSE payload indicates the async image
// pipeline was engaged.
func containsAsyncMarker(text string) bool {
	for _, m := range asyncMarkers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return skippedMainlineRE.MatchString(text)
}

// detectContentPolicyRejection reports whether text contains a ChatGPT content
// audit refusal. Matching is case-insensitive for the English variants.
func detectContentPolicyRejection(text string) bool {
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	for _, m := range contentPolicyMarkers {
		if strings.Contains(text, m) || strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// collectText concatenates every string found under value (recursively) into sb.
func collectText(value any, sb *strings.Builder) {
	switch x := value.(type) {
	case string:
		sb.WriteString(x)
		sb.WriteByte('\n')
	case map[string]any:
		for _, item := range x {
			collectText(item, sb)
		}
	case []any:
		for _, item := range x {
			collectText(item, sb)
		}
	}
}

// conversationRejected scans assistant turns of a fetched conversation for a
// content-policy refusal.
func conversationRejected(conversation map[string]any) bool {
	mapping, _ := conversation["mapping"].(map[string]any)
	for _, rawNode := range mapping {
		node, _ := rawNode.(map[string]any)
		message, _ := node["message"].(map[string]any)
		if message == nil {
			continue
		}
		author, _ := message["author"].(map[string]any)
		role := strings.ToLower(strings.TrimSpace(stringValue(author["role"])))
		if role != "assistant" {
			continue
		}
		var sb strings.Builder
		collectText(message["content"], &sb)
		if detectContentPolicyRejection(sb.String()) {
			return true
		}
	}
	return false
}

// Only a closed, current assistant text turn establishes that no image was returned.
// Tool turns and async markers still take precedence over a text completion.
func completedTextNode(conv map[string]any) string {
	mapping, _ := conv["mapping"].(map[string]any)
	id, _ := conv["current_node"].(string)
	node, _ := mapping[id].(map[string]any)
	m, _ := node["message"].(map[string]any)
	author, _ := m["author"].(map[string]any)
	content, _ := m["content"].(map[string]any)
	if id == "" || author["role"] != "assistant" || content["content_type"] != "text" || m["status"] != "finished_successfully" || m["end_turn"] != true {
		return ""
	}
	if recipient := stringValue(m["recipient"]); recipient != "" && recipient != "all" {
		return ""
	}
	if channel := stringValue(m["channel"]); channel != "" && channel != "final" {
		return ""
	}
	parts, _ := content["parts"].([]any)
	hasText := false
	for _, part := range parts {
		if s, ok := part.(string); ok && strings.TrimSpace(s) != "" {
			hasText = true
			break
		}
	}
	if !hasText {
		return ""
	}
	for _, v := range mapping {
		n, _ := v.(map[string]any)
		msg, _ := n["message"].(map[string]any)
		meta, _ := msg["metadata"].(map[string]any)
		a, _ := msg["author"].(map[string]any)
		recipient, _ := msg["recipient"].(string)
		if a["role"] == "tool" || (a["role"] == "assistant" && recipient != "" && recipient != "all") ||
			stringValue(meta["image_gen_task_id"]) != "" || meta["image_gen_async"] == true || meta["trigger_async_ux"] == true {
			return ""
		}
	}
	return id
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case float32:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func decodeJWTPayload(token string) map[string]any {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return map[string]any{}
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func newUUID() string {
	return uuid.NewString()
}

func clip(v []byte, n int) string {
	s := strings.TrimSpace(string(v))
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func parsePOWResources(html string) ([]string, string) {
	matches := scriptSrcRE.FindAllStringSubmatch(html, -1)
	sources := make([]string, 0, len(matches))
	dataBuild := ""
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		src := strings.TrimSpace(match[1])
		if src == "" {
			continue
		}
		sources = append(sources, src)
		if dataBuild == "" {
			if path := dataBuildPathRE.FindString(src); path != "" {
				dataBuild = path
			}
		}
	}
	if dataBuild == "" {
		if match := htmlDataBuildRE.FindStringSubmatch(html); len(match) >= 2 {
			dataBuild = strings.TrimSpace(match[1])
		}
	}
	if len(sources) == 0 {
		sources = []string{defaultPOWScript}
	}
	return sources, dataBuild
}

func timeMillis() int64 {
	return time.Now().UnixMilli()
}
