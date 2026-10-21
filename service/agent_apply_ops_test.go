package service

import (
	"encoding/json"
	"testing"
)

func applyOpsAuthorization() agentNodeAuthorization {
	return agentNodeAuthorization{
		NodeIDs:       map[string]struct{}{"image-1": {}, "config-1": {}},
		ImageNodeIDs:  map[string]struct{}{"image-1": {}},
		ConfigNodeIDs: map[string]struct{}{"config-1": {}},
	}
}

func TestDecodeCanvasApplyOpsResolvesRefsAndRejectsUnauthorized(t *testing.T) {
	authorization := applyOpsAuthorization()
	valid := `{"ops":[{"op":"add_text","ref":"title","text":"标题","x":0,"y":-200,"width":320},{"op":"add_config","ref":"poster","x":400,"y":0,"mode":"image","prompt":"海报","sourceNodeIds":["title","image-1"]},{"op":"connect","from":"image-1","to":"poster"},{"op":"move","nodeId":"image-1","x":-400,"y":0},{"op":"resize","nodeId":"title","width":360,"height":120}]}`
	arguments, raw, err := decodeAgentToolArguments("canvas_apply_ops", valid, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if len(arguments.(canvasApplyOpsArguments).Ops) != 5 || raw == "" {
		t.Fatalf("arguments = %#v", arguments)
	}
	for name, value := range map[string]string{
		"unknown node":       `{"ops":[{"op":"move","nodeId":"other","x":0,"y":0}]}`,
		"missing position":   `{"ops":[{"op":"add_text","ref":"a","text":"x"}]}`,
		"duplicate ref":      `{"ops":[{"op":"add_text","ref":"a","text":"x","x":0,"y":0},{"op":"add_text","ref":"a","text":"y","x":0,"y":0}]}`,
		"ref used before":    `{"ops":[{"op":"connect","from":"image-1","to":"later"},{"op":"add_text","ref":"later","text":"x","x":0,"y":0}]}`,
		"config to config":   `{"ops":[{"op":"add_config","ref":"c2","x":0,"y":0},{"op":"connect","from":"config-1","to":"c2"}]}`,
		"ref shadows node":   `{"ops":[{"op":"add_text","ref":"image-1","text":"x","x":0,"y":0}]}`,
		"resize too small":   `{"ops":[{"op":"resize","nodeId":"image-1","width":5,"height":5}]}`,
		"unknown op":         `{"ops":[{"op":"delete","nodeId":"image-1"}]}`,
		"empty ops":          `{"ops":[]}`,
		"stray config field": `{"ops":[{"op":"move","nodeId":"image-1","x":0,"y":0,"prompt":"x"}]}`,
	} {
		if _, _, err := decodeAgentToolArguments("canvas.apply_ops", value, authorization); err == nil {
			t.Fatalf("%s should be rejected", name)
		}
	}
}

func TestValidateCanvasApplyOpsResultMatchesEveryStep(t *testing.T) {
	authorization := applyOpsAuthorization()
	arguments, _, err := decodeAgentToolArguments("canvas.apply_ops", `{"ops":[{"op":"add_text","ref":"title","text":"标题","x":0,"y":0},{"op":"connect","from":"image-1","to":"title"},{"op":"move","nodeId":"image-1","x":10,"y":20}]}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	request := SubmitAgentToolResultRequest{Status: "success", NodeIDs: []string{"text-9", "image-1"}, Ops: []AgentToolOpResult{
		{Op: "add_text", Ref: "title", NodeID: "text-9"},
		{Op: "connect", FromNodeID: "image-1", ToNodeID: "text-9"},
		{Op: "move", NodeID: "image-1"},
	}}
	if err := validateAgentToolSuccess("canvas.apply_ops", arguments, &request); err != nil {
		t.Fatal(err)
	}
	output, _ := json.Marshal(agentToolResultOutput("canvas.apply_ops", request))
	if string(output) != `{"nodeIds":["text-9","image-1"],"ops":[{"op":"add_text","ref":"title","nodeId":"text-9"},{"op":"connect","fromNodeId":"image-1","toNodeId":"text-9"},{"op":"move","nodeId":"image-1"}]}` {
		t.Fatalf("output = %s", output)
	}
	bad := request
	bad.Ops = append([]AgentToolOpResult{}, request.Ops...)
	bad.Ops[1].ToNodeID = "title"
	if err := validateAgentToolSuccess("canvas.apply_ops", arguments, &bad); err == nil {
		t.Fatal("connect must resolve ref to created node id")
	}
	missing := request
	missing.NodeIDs = []string{"text-9"}
	if err := validateAgentToolSuccess("canvas.apply_ops", arguments, &missing); err == nil {
		t.Fatal("nodeIds must list every created or moved node")
	}
	stray := SubmitAgentToolResultRequest{Status: "success", NodeID: "x", Placement: "center", Ops: request.Ops}
	if err := validateAgentToolSuccess("canvas.add_text", canvasAddTextArguments{Text: "x", Placement: "center"}, &stray); err == nil {
		t.Fatal("other tools must not carry ops")
	}
}

func TestApplyOpsSchemaIsRegistered(t *testing.T) {
	found := false
	for _, schema := range agentToolSchemas() {
		if schema.(map[string]any)["function"].(map[string]any)["name"] == "canvas_apply_ops" {
			found = true
		}
	}
	if !found || canonicalAgentToolName("canvas_apply_ops") != "canvas.apply_ops" || !revertibleAgentTool("canvas.apply_ops") || agentToolRequiresConfirmation("canvas.apply_ops") {
		t.Fatal("canvas.apply_ops is not registered correctly")
	}
}

func TestNormalizeAgentCanvasContextKeepsValidViewport(t *testing.T) {
	context, err := normalizeAgentCanvasContext(AgentCanvasContext{Autonomy: "standard", Viewport: &AgentCanvasViewport{X: -100, Y: 50, Width: 1200, Height: 800, Zoom: 0.8}})
	if err != nil || context.Viewport == nil || context.Viewport.Width != 1200 {
		t.Fatalf("context = %#v err=%v", context, err)
	}
	context, err = normalizeAgentCanvasContext(AgentCanvasContext{Autonomy: "standard", Viewport: &AgentCanvasViewport{Width: -1, Height: 10}})
	if err != nil || context.Viewport != nil {
		t.Fatalf("invalid viewport should be dropped: %#v err=%v", context.Viewport, err)
	}
}
