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
	if !agentUpstreamShouldRetry(drop, 2*time.Second, 0) || !agentUpstreamShouldRetry(io.ErrUnexpectedEOF, time.Second, 1) {
		t.Fatal("fast drops should retry")
	}
	if agentUpstreamShouldRetry(drop, 2*time.Second, agentUpstreamMaxRetries) {
		t.Fatal("retries must be capped")
	}
	if agentUpstreamShouldRetry(drop, 30*time.Second, 0) {
		t.Fatal("drop after 25s+ should not retry")
	}
	if agentUpstreamShouldRetry(errors.New("context deadline exceeded"), time.Second, 0) {
		t.Fatal("timeouts are not transport drops")
	}
	if agentCompletionDeadline < 2*agentUpstreamAttemptTimeout || agentRunningTimeout <= agentCompletionDeadline {
		t.Fatal("deadlines inconsistent")
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
	} {
		if !agentFakeToolCallPattern.MatchString(text) {
			t.Fatalf("not detected: %q", text)
		}
	}
	if agentFakeToolCallPattern.MatchString("已在画布添加三个节点，并与场景节点连线。") {
		t.Fatal("false positive")
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
