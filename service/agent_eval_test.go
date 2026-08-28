package service

import (
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestAgentProductionEvaluationSuite(t *testing.T) {
	t.Run("compound goals stay in planner", func(t *testing.T) {
		if simpleAgentMediaCommand("先生成商品主图，然后添加卖点并排列") {
			t.Fatal("compound goal bypassed planner")
		}
	})

	t.Run("tool registry keeps safety policy", func(t *testing.T) {
		for _, name := range []string{"canvas.delete", "canvas.update_text", "agent.remember", "agent.forget"} {
			if !agentToolRequiresConfirmation(name) {
				t.Fatalf("%s lost confirmation policy", name)
			}
		}
		if retryableAgentTool("canvas.delete") || !retryableAgentTool("image.generate") {
			t.Fatal("retry policy regressed")
		}
	})

	t.Run("tool and media budgets stop before overspend", func(t *testing.T) {
		run := model.AgentRun{MaxToolCalls: 3, MaxMediaCalls: 1, MaxDurationSec: 900, StartedAt: now()}
		steps := []model.AgentStep{{Type: model.AgentStepTypeTool, ToolName: "image.generate"}}
		if reason := agentRunBudgetReason(run, steps, "video.generate"); reason != "media_calls" {
			t.Fatalf("unexpected media budget result %q", reason)
		}
		steps = append(steps, model.AgentStep{Type: model.AgentStepTypeTool, ToolName: "image.inspect"}, model.AgentStep{Type: model.AgentStepTypeTool, ToolName: "canvas.add_text"})
		if reason := agentRunBudgetReason(run, steps, ""); reason != "tool_calls" {
			t.Fatalf("unexpected tool budget result %q", reason)
		}
	})

	t.Run("model context never exposes storage keys", func(t *testing.T) {
		raw := `{"autonomy":"standard","selectedNodeIds":["image-1"],"focusNodeIds":["image-1"],"nodes":[{"id":"image-1","type":"image","title":"图","x":0,"y":0,"width":100,"height":100,"storageKey":"private/secret.png"}],"connections":[]}`
		context, err := agentModelCanvasContext(raw, nil)
		if err != nil || strings.Contains(context, "private/secret.png") {
			t.Fatalf("unsafe model context: %v %s", err, context)
		}
	})
}
