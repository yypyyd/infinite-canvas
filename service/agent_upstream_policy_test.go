package service

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestAgentUpstreamRetryPolicy(t *testing.T) {
	drop := errors.New(`Post "x": EOF`)
	check := func(err error, elapsed time.Duration, quick, late int, wantRetry, wantLate bool) {
		t.Helper()
		retry, isLate := agentUpstreamShouldRetry(err, elapsed, quick, late)
		if retry != wantRetry || isLate != wantLate {
			t.Fatalf("elapsed=%v quick=%d late=%d: got retry=%v late=%v", elapsed, quick, late, retry, isLate)
		}
	}
	check(drop, 2*time.Second, 0, 0, true, false)
	check(io.ErrUnexpectedEOF, time.Second, 1, 0, true, false)
	check(drop, 2*time.Second, agentUpstreamMaxRetries, 0, false, false)
	// 代理约 30.5s 空闲掐断：重试一次（共两次晚断尝试），快速重试次数不影响。
	check(drop, 30500*time.Millisecond, 0, 0, true, true)
	check(drop, 30500*time.Millisecond, agentUpstreamMaxRetries, 0, true, true)
	check(drop, 30500*time.Millisecond, 0, 1, false, true)
	check(drop, 45*time.Second, 0, 0, false, true)
	check(errors.New("context deadline exceeded"), time.Second, 0, 0, false, false)
	worst := agentUpstreamLateDropLimit*time.Duration(1+agentUpstreamMaxLateRetries) + agentUpstreamSlowDropThreshold*time.Duration(agentUpstreamMaxRetries) + agentUpstreamAttemptTimeout
	if worst > agentCompletionDeadline+agentUpstreamAttemptTimeout || agentRunningTimeout <= agentCompletionDeadline {
		t.Fatalf("deadlines inconsistent: worst=%v", worst)
	}
}

func TestAgentChannelStreams(t *testing.T) {
	if agentChannelStreams(model.ModelChannel{BaseURL: "https://cliproxy.2008923.xyz/v1"}) {
		t.Fatal("cliproxy should use non-streaming")
	}
	if !agentChannelStreams(model.ModelChannel{BaseURL: "https://api.huantu.xyz/v1"}) {
		t.Fatal("other channels stream")
	}
}

func TestAgentFakeToolCallTextForms(t *testing.T) {
	for _, text := range []string{
		"```xml\ncall canvas.apply_ops with ops is [{\"op\":\"add_text\"}]",
		"```xml\n<function-call>\n<tool_name>canvas_add_text</tool_name>",
		"<function call canvas_read>",
		"tool call canvas_add_text with text is 开场",
		"invoke canvas_apply_ops with ops is []",
		"calling image.generate with prompt is 猫",
		"call agent_ask_user with question is 什么",
	} {
		if !agentFakeToolCallPattern.MatchString(text) {
			t.Fatalf("not detected: %q", text)
		}
	}
	for _, ok := range []string{"已在画布添加三个节点，并与场景节点连线。", "Call me with any questions.", "invoke the muse with care"} {
		if agentFakeToolCallPattern.MatchString(ok) {
			t.Fatalf("false positive: %q", ok)
		}
	}
}

func agentSchemaNames(schemas []any) map[string]bool {
	names := map[string]bool{}
	for _, schema := range schemas {
		names[schema.(map[string]any)["function"].(map[string]any)["name"].(string)] = true
	}
	return names
}

func TestSelectAgentToolSchemas(t *testing.T) {
	text := agentSchemaNames(selectAgentToolSchemas("在画布上加三个文字节点，分别写开场、冲突、结局，并依次连线。", nil))
	if !text["canvas_apply_ops"] || !text["canvas_connect"] || text["video_generate"] || text["agent_remember"] || text["canvas_delete"] {
		t.Fatalf("text request tools: %v", text)
	}
	if len(text) >= len(agentToolSchemas()) {
		t.Fatal("expected fewer tools")
	}
	image := agentSchemaNames(selectAgentToolSchemas("帮我生成一张海报", nil))
	if !image["image_generate"] || !image["image_inspect"] {
		t.Fatalf("image tools missing: %v", image)
	}
	used := agentSchemaNames(selectAgentToolSchemas("好的", []model.AgentStep{{ToolName: "video.generate"}}))
	if !used["video_generate"] || !used["video_inspect"] {
		t.Fatalf("previously used tools missing: %v", used)
	}
}

func TestStripAgentFakeToolCallText(t *testing.T) {
	content := "已为你添加三个镜头建议节点。\n```xml\ntool call canvas_add_text with text is 开场\n</tool_call>\n```"
	if got := stripAgentFakeToolCallText(content); got != "已为你添加三个镜头建议节点。" {
		t.Fatalf("got %q", got)
	}
	if got := stripAgentFakeToolCallText("invoke canvas_apply_ops with ops is []"); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := stripAgentFakeToolCallText("普通回复"); got != "普通回复" {
		t.Fatalf("got %q", got)
	}
}

func TestAgentPlainRetryOnFakeTextOnly(t *testing.T) {
	body := map[string]any{"tools": []any{}}
	fake := agentUpstreamMessage{Content: "tool call canvas_add_text with text is 开场"}
	if !agentStreamNeedsPlainRetry(body, fake) {
		t.Fatal("fake text without tool call should retry")
	}
	fake.ToolCalls = []agentUpstreamToolCall{{Type: "function"}}
	if agentStreamNeedsPlainRetry(body, fake) {
		t.Fatal("real tool call should be used instead of retrying")
	}
}
