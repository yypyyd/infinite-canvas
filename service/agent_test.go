package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestParseExplicitAddConfigRequest(t *testing.T) {
	canvas := AgentCanvasContext{
		SelectedNodeIDs: []string{"text-1"},
		VisibleNodeIDs:  []string{"text-1", "image-1"},
		Nodes: []AgentCanvasNode{
			{ID: "text-1", Type: "text", Title: "文案", X: 0},
			{ID: "image-1", Type: "image", Title: "主图", X: 400},
		},
	}
	sources, placement, ok := parseExplicitAddConfigRequest("给画布上的文本节点添加一个配置节点并连上，不要生图", canvas)
	if !ok || placement != "right_of_selection" || len(sources) != 1 || sources[0] != "text-1" {
		t.Fatalf("expected selected text source, got ok=%v placement=%q sources=%v", ok, placement, sources)
	}
	if _, _, ok := parseExplicitAddConfigRequest("生成图片并添加配置节点", canvas); ok {
		t.Fatal("compound generate+config should stay in planner")
	}
	unselected := canvas
	unselected.SelectedNodeIDs = nil
	unselected.FocusNodeIDs = nil
	left, _, ok := parseExplicitAddConfigRequest("给视口最左边那张图添加配置并连上", unselected)
	if !ok || len(left) != 1 || left[0] != "text-1" {
		t.Fatalf("expected leftmost visible node, got ok=%v sources=%v", ok, left)
	}
}

func TestSanitizeAgentUserFacingText(t *testing.T) {
	got := sanitizeAgentUserFacingText("**目标完成**：已为画布上的文本节点（id: text-1789248805391-97cfc）添加配置节点（config-1789248869566-aif4z），并通过连线关联，无图像生成。")
	if strings.Contains(got, "text-") || strings.Contains(got, "config-") || strings.Contains(got, "目标完成") || strings.Contains(got, "**") {
		t.Fatalf("expected ids and goal prefix stripped, got %q", got)
	}
	if !strings.Contains(got, "添加配置节点") {
		t.Fatalf("expected remaining canvas sentence, got %q", got)
	}
}

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

func TestCanvasNativeAgentToolsAuthorizeAndValidate(t *testing.T) {
	authorization := agentNodeAuthorization{
		NodeIDs:       map[string]struct{}{"text-1": {}, "image-1": {}, "image-2": {}, "video-1": {}, "audio-1": {}, "config-1": {}, "config-2": {}},
		ImageNodeIDs:  map[string]struct{}{"image-1": {}, "image-2": {}},
		VideoNodeIDs:  map[string]struct{}{"video-1": {}},
		AudioNodeIDs:  map[string]struct{}{"audio-1": {}},
		TextNodeIDs:   map[string]struct{}{"text-1": {}},
		ConfigNodeIDs: map[string]struct{}{"config-1": {}, "config-2": {}},
	}
	videoArgs, _, err := decodeAgentToolArguments("video.generate", `{"prompt":"咖啡机旋转展示","imageNodeIds":["image-1","image-2"],"videoNodeIds":["video-1"],"audioNodeIds":["audio-1"]}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	video, ok := videoArgs.(videoGenerateArguments)
	if !ok || video.ImageNodeID != "image-1" || len(video.ImageNodeIDs) != 2 || len(video.VideoNodeIDs) != 1 || len(video.AudioNodeIDs) != 1 {
		t.Fatalf("video arguments = %#v", videoArgs)
	}
	if _, _, err := decodeAgentToolArguments("video.generate", `{"prompt":"咖啡机旋转展示","imageNodeIds":["image-9"]}`, authorization); err == nil {
		t.Fatal("expected unauthorized video image rejection")
	}

	configArgs, _, err := decodeAgentToolArguments("canvas.add_config", `{"sourceNodeIds":["text-1","image-1"]}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	config, ok := configArgs.(canvasAddConfigArguments)
	if !ok || config.Placement != "right_of_selection" || len(config.SourceNodeIDs) != 2 {
		t.Fatalf("config arguments = %#v", configArgs)
	}
	if err := validateAgentToolSuccess("canvas.add_config", config, &SubmitAgentToolResultRequest{Status: "success", NodeID: "config-new", Placement: "right_of_selection"}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := decodeAgentToolArguments("canvas.connect", `{"fromNodeId":"config-1","toNodeId":"config-2"}`, authorization); err == nil {
		t.Fatal("expected config-to-config rejection")
	}
	connectArgs, _, err := decodeAgentToolArguments("canvas.connect", `{"fromNodeId":"image-1","toNodeId":"config-1"}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgentToolSuccess("canvas.connect", connectArgs, &SubmitAgentToolResultRequest{Status: "success", FromNodeID: "image-1", ToNodeID: "config-1"}); err != nil {
		t.Fatal(err)
	}

	textArgs, _, err := decodeAgentToolArguments("canvas.add_text", `{"text":"卖点","placement":"below_selection"}`, authorization)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateAgentToolSuccess("canvas.add_text", textArgs, &SubmitAgentToolResultRequest{Status: "success", NodeID: "text-new", Placement: "below_selection"}); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAgentCanvasContextKeepsVisibleNodeIDs(t *testing.T) {
	context, err := normalizeAgentCanvasContext(AgentCanvasContext{
		Autonomy: agentAutonomyStandard, SelectedNodeIDs: []string{"image-1"}, VisibleNodeIDs: []string{"image-1", "image-1", "missing", "text-1"},
		Nodes: []AgentCanvasNode{
			{ID: "image-1", Type: "image", Title: "图", X: 0, Y: 0, Width: 100, Height: 100, StorageKey: "secret/a"},
			{ID: "text-1", Type: "text", Title: "文", X: 200, Y: 0, Width: 100, Height: 50},
		},
	})
	if err != nil || len(context.VisibleNodeIDs) != 2 || context.VisibleNodeIDs[0] != "image-1" || context.VisibleNodeIDs[1] != "text-1" {
		t.Fatalf("visible = %#v err=%v", context.VisibleNodeIDs, err)
	}
	raw, _ := json.Marshal(context)
	modelContext, err := agentModelCanvasContext(string(raw), nil)
	if err != nil || !strings.Contains(modelContext, `"visibleNodeIds"`) || !strings.Contains(modelContext, `"text-1"`) || strings.Contains(modelContext, "secret/a") {
		t.Fatalf("unsafe visible context: %v %s", err, modelContext)
	}
	messages := attachAgentPlanningPreviews([]agentChatMessage{{Role: "user", Content: "看当前画面"}}, "org-1", string(raw))
	encoded, _ := json.Marshal(messages)
	if strings.Contains(string(encoded), "secret/a") || strings.Contains(string(encoded), "storageKey") || strings.Contains(string(encoded), "/api/media/storage/") {
		t.Fatalf("planning preview leaked storage: %s", encoded)
	}
	if !unfetchableAgentPreviewURL("https://huantu.xyz/api/media/storage/file-1?expires=1&signature=x") || unfetchableAgentPreviewURL("https://cdn.example/thumb.webp") {
		t.Fatal("self-hosted preview URL filter failed")
	}
}

func TestNormalizeAuthorizedNodeIDsAndPlacement(t *testing.T) {
	allowed := map[string]struct{}{"a": {}, "b": {}}
	ids, err := normalizeAuthorizedNodeIDs([]string{" a ", "a", "b"}, allowed, 3, "ids")
	if err != nil || len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	if _, err := normalizeAuthorizedNodeIDs([]string{"c"}, allowed, 3, "ids"); err == nil {
		t.Fatal("expected unauthorized rejection")
	}
	if !validAgentPlacement("viewport") || validAgentPlacement("stack") {
		t.Fatal("placement validation failed")
	}
}
