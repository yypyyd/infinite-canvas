package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestSimpleAgentMediaCommandRejectsCompoundWorkflows(t *testing.T) {
	for _, content := range []string{"生成一张红色运动鞋海报", "制作咖啡机旋转展示视频", "策划一张夏季饮料海报", "生成一张先进科技风海报"} {
		if !simpleAgentMediaCommand(content) {
			t.Fatalf("expected simple media command for %q", content)
		}
	}
	for _, content := range []string{"先分析图片，然后生成海报", "生成图片；再添加文案", "比较图片并且生成视频", "生成图片同时排列节点"} {
		if simpleAgentMediaCommand(content) {
			t.Fatalf("expected compound workflow for %q", content)
		}
	}
}

func TestAgentModelCanvasContextKeepsFocusAndOneHopOnly(t *testing.T) {
	context := AgentCanvasContext{
		Autonomy: agentAutonomyStandard, SelectedNodeIDs: []string{"a"}, FocusNodeIDs: []string{"a"},
		Nodes: []AgentCanvasNode{
			{ID: "a", Type: "text", Title: "目标", X: 0, Y: 0, Width: 100, Height: 50, Content: "保留内容", StorageKey: "secret/a"},
			{ID: "b", Type: "image", Title: "一跳", X: 200, Y: 0, Width: 100, Height: 100, Prompt: "保留提示", StorageKey: "secret/b"},
			{ID: "c", Type: "text", Title: "二跳", X: 400, Y: 0, Width: 100, Height: 50, Content: "不得泄漏的长内容"},
		},
		Connections: []AgentCanvasConnection{{From: "a", To: "b"}, {From: "b", To: "c"}},
	}
	raw, _ := json.Marshal(context)
	result, err := agentModelCanvasContext(string(raw), []model.AgentStep{{ToolName: "canvas.add_text", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","nodeId":"generated-1"}`}})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"id":"a"`, `"id":"b"`, `"nodeCount":3`, `"generated-1"`} {
		if !strings.Contains(result, expected) {
			t.Fatalf("model context missing %s: %s", expected, result)
		}
	}
	for _, forbidden := range []string{`"id":"c"`, "不得泄漏的长内容", "secret/a", "secret/b"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("model context leaked %q: %s", forbidden, result)
		}
	}
}

func TestAgentModelToolResultOmitsMediaStorageKey(t *testing.T) {
	result, err := json.Marshal(agentModelToolResult(model.AgentStep{ToolName: "image.generate", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","images":[{"nodeId":"image-1","storageKey":"private/image-1.png"}]}`}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "image-1") || strings.Contains(string(result), "storageKey") || strings.Contains(string(result), "private/") {
		t.Fatalf("unsafe model tool result: %s", result)
	}
}

func TestAgentMediaRequestNeedsClarification(t *testing.T) {
	for _, content := range []string{"生成图片", "帮我生成一张图片", "请制作短视频", "我想要策划并生成海报"} {
		if !agentMediaRequestNeedsClarification(content) {
			t.Fatalf("expected clarification for %q", content)
		}
	}
	for _, content := range []string{"生成一张红色运动鞋商品主图", "制作咖啡机旋转展示视频"} {
		if agentMediaRequestNeedsClarification(content) {
			t.Fatalf("unexpected clarification for %q", content)
		}
	}
}

func TestAgentMemoryArgumentsNormalizeAndProtectSensitiveKeys(t *testing.T) {
	args, raw, err := decodeAgentToolArguments("agent.remember", `{"kind":"preference","key":"image.style","content":"写实","confidence":0.9}`, agentNodeAuthorization{})
	if err != nil {
		t.Fatal(err)
	}
	input, ok := args.(agentRememberArguments)
	if !ok || input.Scope != "project" || input.Confidence != 0.9 || raw == "" {
		t.Fatalf("normalized memory = %#v raw=%q", args, raw)
	}
	if _, _, err := decodeAgentToolArguments("agent.remember", `{"kind":"fact","key":"api_key","content":"secret"}`, agentNodeAuthorization{}); err == nil {
		t.Fatal("expected sensitive memory key rejection")
	}
	if _, _, err := decodeAgentToolArguments("agent.remember", `{"kind":"fact","key":"bad key","content":"value"}`, agentNodeAuthorization{}); err == nil {
		t.Fatal("expected invalid memory key")
	}
}

func TestCanonicalAgentToolNameAcceptsFunctionSchemaAliases(t *testing.T) {
	for alias, expected := range map[string]string{"agent_remember": "agent.remember", "agent_forget": "agent.forget", "canvas_add_text": "canvas.add_text", "image_inspect": "image.inspect", "video_inspect": "video.inspect"} {
		if actual := canonicalAgentToolName(alias); actual != expected {
			t.Fatalf("%s normalized to %s, expected %s", alias, actual, expected)
		}
	}
}

func TestNormalizeAgentCanvasContextDefaultsAndValidatesAutonomy(t *testing.T) {
	context, err := normalizeAgentCanvasContext(AgentCanvasContext{})
	if err != nil || context.Autonomy != agentAutonomyStandard {
		t.Fatalf("default autonomy = %q, err=%v", context.Autonomy, err)
	}
	context, err = normalizeAgentCanvasContext(AgentCanvasContext{Autonomy: agentAutonomyAutonomous})
	if err != nil || context.Autonomy != agentAutonomyAutonomous {
		t.Fatalf("autonomous context = %#v, err=%v", context, err)
	}
	if _, err := normalizeAgentCanvasContext(AgentCanvasContext{Autonomy: "unrestricted"}); err == nil {
		t.Fatal("expected invalid autonomy rejection")
	}
}

func TestAgentMediaIntentRecognizesNaturalImageRequest(t *testing.T) {
	image, video := agentMediaIntent("我想生成一个美女图片", false)
	if !image || video {
		t.Fatalf("image=%v video=%v", image, video)
	}
}

func TestAgentImageExecutionPromptKeepsGoalAndSafeDefaults(t *testing.T) {
	prompt := agentImageExecutionPrompt("生成红色运动鞋商品主图")
	for _, expected := range []string{"生成红色运动鞋商品主图", "保持用户明确指定", "单一核心主体", "不要添加用户未要求的文字"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q: %s", expected, prompt)
		}
	}
}

func TestAgentVideoExecutionPromptKeepsGoalAndSafeDefaults(t *testing.T) {
	prompt := agentVideoExecutionPrompt("生成咖啡机旋转展示视频")
	for _, expected := range []string{"生成咖啡机旋转展示视频", "保持用户明确指定", "动作连贯", "不要添加用户未要求的文字"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt missing %q: %s", expected, prompt)
		}
	}
}

func TestImageInspectArgumentsAndResultValidation(t *testing.T) {
	authorization := agentNodeAuthorization{ImageNodeIDs: map[string]struct{}{"image-1": {}}}
	arguments, _, err := decodeAgentToolArguments("image.inspect", `{"nodeIds":["image-1"],"criteria":"检查主体与构图"}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	request := SubmitAgentToolResultRequest{Status: "success", Inspection: &AgentToolInspection{Status: "needs_revision", Summary: "主体被裁切", Issues: []string{"鞋尖超出画面"}, RevisedPrompt: "完整展示红色运动鞋，鞋尖不得裁切"}}
	if err := validateAgentToolSuccess("image.inspect", arguments, &request); err != nil {
		t.Fatal(err)
	}
	request.Inspection.RevisedPrompt = ""
	if err := validateAgentToolSuccess("image.inspect", arguments, &request); err == nil {
		t.Fatal("expected missing revised prompt rejection")
	}
	request.Inspection = &AgentToolInspection{Status: "unavailable", Summary: "视觉模型不可用"}
	if err := validateAgentToolSuccess("image.inspect", arguments, &request); err != nil {
		t.Fatal(err)
	}
	request.Inspection = &AgentToolInspection{Status: "passed", Summary: "验收通过", Issues: []string{"不应存在的问题"}}
	if err := validateAgentToolSuccess("image.inspect", arguments, &request); err == nil {
		t.Fatal("expected passed inspection issues rejection")
	}
	if _, _, err := decodeAgentToolArguments("image.inspect", `{"nodeIds":["image-2"],"criteria":"检查主体"}`, authorization); err == nil {
		t.Fatal("expected unauthorized image rejection")
	}
}

func TestPendingAgentImageInspectionMatchesGeneratedNodes(t *testing.T) {
	steps := []model.AgentStep{
		{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Input: `{"nodeIds":["existing"],"criteria":"检查"}`},
		{ToolName: "image.generate", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","images":[{"nodeId":"generated-1","storageKey":"agent/generated-1.png"}]}`},
	}
	nodeIDs, pending, err := pendingAgentImageInspection(steps)
	if err != nil || !pending || len(nodeIDs) != 1 || nodeIDs[0] != "generated-1" {
		t.Fatalf("pending=%v nodeIDs=%v err=%v", pending, nodeIDs, err)
	}
	steps = append(steps, model.AgentStep{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Input: `{"nodeIds":["generated-1"],"criteria":"检查"}`})
	if nodeIDs, pending, err = pendingAgentImageInspection(steps); err != nil || pending || len(nodeIDs) != 0 {
		t.Fatalf("pending=%v nodeIDs=%v err=%v", pending, nodeIDs, err)
	}
}

func TestVideoInspectArgumentsAndPendingInspection(t *testing.T) {
	authorization := agentNodeAuthorization{VideoNodeIDs: map[string]struct{}{"video-1": {}}}
	arguments, _, err := decodeAgentToolArguments("video.inspect", `{"nodeId":"video-1","criteria":"检查主体一致性与镜头连续性"}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	request := SubmitAgentToolResultRequest{Status: "success", Inspection: &AgentToolInspection{Status: "passed", Summary: "视频目标满足", Issues: []string{}}}
	if err := validateAgentToolSuccess("video.inspect", arguments, &request); err != nil {
		t.Fatal(err)
	}
	if _, _, err := decodeAgentToolArguments("video.inspect", `{"nodeId":"video-2","criteria":"检查镜头"}`, authorization); err == nil {
		t.Fatal("expected unauthorized video rejection")
	}

	steps := []model.AgentStep{{ToolName: "video.generate", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","video":{"nodeId":"video-1","storageKey":"agent/video-1.mp4"}}`}}
	nodeID, pending, err := pendingAgentVideoInspection(steps)
	if err != nil || !pending || nodeID != "video-1" {
		t.Fatalf("pending=%v nodeID=%q err=%v", pending, nodeID, err)
	}
	steps = append(steps, model.AgentStep{ToolName: "video.inspect", Status: model.AgentStepStatusCompleted, Input: `{"nodeId":"video-1","criteria":"检查镜头"}`})
	if nodeID, pending, err = pendingAgentVideoInspection(steps); err != nil || pending || nodeID != "" {
		t.Fatalf("pending=%v nodeID=%q err=%v", pending, nodeID, err)
	}
}

func TestAgentMediaRevisionLimitStopsSecondAdjustment(t *testing.T) {
	run := model.AgentRun{Context: `{"autonomy":"autonomous"}`}
	steps := []model.AgentStep{
		{ToolName: "image.generate", Status: model.AgentStepStatusCompleted},
		{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"第一次问题","issues":["主体裁切"],"revisedPrompt":"完整展示主体"}}`},
		{ToolName: "image.generate", Status: model.AgentStepStatusCompleted},
		{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"仍有问题","issues":["文字乱码"],"revisedPrompt":"完整展示主体且不要文字"}}`},
	}
	if !agentMediaRevisionLimitReached(run, steps, "image.generate") {
		t.Fatal("expected second semantic adjustment to be blocked")
	}
	if agentMediaRevisionLimitReached(run, steps[:2], "image.generate") {
		t.Fatal("first semantic adjustment should remain available")
	}
	alternatingSteps := []model.AgentStep{
		{ToolName: "image.edit", Status: model.AgentStepStatusCompleted},
		{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"第一次问题","issues":["主体裁切"],"revisedPrompt":"完整展示主体"}}`},
		{ToolName: "image.generate", Status: model.AgentStepStatusCompleted},
		{ToolName: "image.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"仍有问题","issues":["文字乱码"],"revisedPrompt":"完整展示主体且不要文字"}}`},
	}
	if !agentMediaRevisionLimitReached(run, alternatingSteps, "image.edit") {
		t.Fatal("expected alternating image tools to share the adjustment limit")
	}
	videoSteps := []model.AgentStep{
		{ToolName: "video.generate", Status: model.AgentStepStatusCompleted},
		{ToolName: "video.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"第一次问题","issues":["主体跳变"],"revisedPrompt":"保持主体一致"}}`},
		{ToolName: "video.generate", Status: model.AgentStepStatusCompleted},
		{ToolName: "video.inspect", Status: model.AgentStepStatusCompleted, Output: `{"status":"success","inspection":{"status":"needs_revision","summary":"仍有问题","issues":["镜头抖动"],"revisedPrompt":"保持主体一致且镜头稳定"}}`},
	}
	if !agentMediaRevisionLimitReached(run, videoSteps, "video.generate") {
		t.Fatal("expected second video adjustment to be blocked")
	}
	standardRun := model.AgentRun{Context: `{"autonomy":"standard"}`}
	if !agentMediaRevisionLimitReached(standardRun, videoSteps[:2], "video.generate") {
		t.Fatal("standard mode should report inspection issues without regenerating")
	}
}
