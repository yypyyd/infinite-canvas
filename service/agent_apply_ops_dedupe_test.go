package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func dedupeTestStep(t *testing.T) model.AgentStep {
	t.Helper()
	input := `{"ops":[{"op":"add_text","ref":"a","text":"镜头建议：全景","x":100,"y":480},{"op":"add_text","ref":"b","text":"镜头建议：跟拍","x":400,"y":480},{"op":"add_text","ref":"c","text":"镜头建议：特写","x":700,"y":480}]}`
	output := `{"callId":"call-1","status":"success","nodeIds":["n1","n2","n3"],"ops":[{"op":"add_text","ref":"a","nodeId":"n1"},{"op":"add_text","ref":"b","nodeId":"n2","x":472,"y":480,"adjusted":true},{"op":"add_text","ref":"c","nodeId":"n3","x":844,"y":480,"adjusted":true}]}`
	return model.AgentStep{Type: model.AgentStepTypeTool, Status: model.AgentStepStatusCompleted, ToolName: "canvas.apply_ops", ToolCallID: "call-1", Input: input, Output: output}
}

// 复现线上事故：模型下一轮换了 ref 和 y 再次新增同样三段文字并补连线，只应保留连线且指向已建节点。
func TestDedupeAgentApplyOpsRemapsRepeatedAdds(t *testing.T) {
	steps := []model.AgentStep{dedupeTestStep(t)}
	next := `{"ops":[{"op":"add_text","ref":"x1","text":"镜头建议：全景","x":100,"y":720},{"op":"add_text","ref":"x2","text":"镜头建议：跟拍","x":400,"y":720},{"op":"add_text","ref":"x3","text":" 镜头建议：特写 ","x":700,"y":720},{"op":"connect","from":"x1","to":"s1"},{"op":"connect","from":"x2","to":"s2"},{"op":"connect","from":"x3","to":"s3"}]}`
	rewritten, skipped, empty := dedupeAgentApplyOps(next, steps)
	if empty || skipped != 3 {
		t.Fatalf("skipped=%d empty=%v", skipped, empty)
	}
	var got canvasApplyOpsArguments
	if err := json.Unmarshal([]byte(rewritten), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Ops) != 3 {
		t.Fatalf("ops=%+v", got.Ops)
	}
	for i, want := range [][2]string{{"n1", "s1"}, {"n2", "s2"}, {"n3", "s3"}} {
		if got.Ops[i].Op != "connect" || got.Ops[i].From != want[0] || got.Ops[i].To != want[1] {
			t.Fatalf("op %d = %+v", i, got.Ops[i])
		}
	}
	authorization := agentNodeAuthorization{NodeIDs: map[string]struct{}{"n1": {}, "n2": {}, "n3": {}, "s1": {}, "s2": {}, "s3": {}}, ConfigNodeIDs: map[string]struct{}{}}
	if _, err := decodeCanvasApplyOps(rewritten, authorization); err != nil {
		t.Fatalf("rewritten ops should validate: %v", err)
	}
}

func TestDedupeAgentApplyOpsFullRepeatIsEmpty(t *testing.T) {
	connected := model.AgentStep{Type: model.AgentStepTypeTool, Status: model.AgentStepStatusCompleted, ToolName: "canvas.apply_ops", Input: `{"ops":[{"op":"connect","from":"n1","to":"s1"}]}`, Output: `{"callId":"call-2","status":"success","ops":[{"op":"connect","fromNodeId":"s1","toNodeId":"n1"}]}`}
	steps := []model.AgentStep{dedupeTestStep(t), connected}
	repeat := `{"ops":[{"op":"add_text","ref":"again","text":"镜头建议：全景","x":100,"y":960},{"op":"connect","from":"again","to":"s1"}]}`
	if _, skipped, empty := dedupeAgentApplyOps(repeat, steps); !empty || skipped != 2 {
		t.Fatalf("skipped=%d empty=%v", skipped, empty)
	}
	fresh := `{"ops":[{"op":"add_text","ref":"new","text":"全新的内容","x":100,"y":960}]}`
	if out, skipped, empty := dedupeAgentApplyOps(fresh, steps); empty || skipped != 0 || out != fresh {
		t.Fatalf("fresh ops changed: %q %d %v", out, skipped, empty)
	}
}

func TestAgentApplyOpsModelResultListsCreatedNodes(t *testing.T) {
	step := dedupeTestStep(t)
	encoded, _ := json.Marshal(agentModelToolResult(step))
	text := string(encoded)
	for _, want := range []string{`"created"`, `"nodeId":"n1"`, `"nodeId":"n3"`, "镜头建议：特写", "不要再次创建"} {
		if !strings.Contains(text, want) {
			t.Fatalf("model result missing %s: %s", want, text)
		}
	}
}

func TestAgentAskUserSkippedModelResult(t *testing.T) {
	step := model.AgentStep{Type: model.AgentStepTypeTool, Status: model.AgentStepStatusCompleted, ToolName: "agent.ask_user", Confirmation: "rejected", Output: `{"answer":"用户跳过了回答","status":"success"}`}
	encoded, _ := json.Marshal(agentModelToolResult(step))
	if !strings.Contains(string(encoded), "skipped") || !strings.Contains(string(encoded), "不要再次询问") {
		t.Fatalf("unexpected: %s", encoded)
	}
}
