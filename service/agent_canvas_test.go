package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestAgentCanvasSearchPagesWithoutLeakingMediaKeys(t *testing.T) {
	canvas := AgentCanvasContext{Autonomy: "standard", Connections: []AgentCanvasConnection{{From: "node-0", To: "node-25"}}}
	for i := 0; i < 35; i++ {
		canvas.Nodes = append(canvas.Nodes, AgentCanvasNode{ID: fmt.Sprintf("node-%d", i), Type: "image", Title: "产品图", StorageKey: "private-key", Content: strings.Repeat("文", 1200)})
	}
	page := agentCanvasObservation(canvas, canvasReadArguments{Query: "产品", Offset: 20})
	if len(page.Nodes) != 15 || page.Nodes[0].ID != "node-20" || len(page.Connections) != 1 {
		t.Fatalf("wrong page: %#v", page)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "private-key") {
		t.Fatal("media key leaked")
	}
	if len([]rune(page.Nodes[0].Content)) > 800 {
		t.Fatal("observation not bounded")
	}
	if len(agentCanvasObservation(canvas, canvasReadArguments{Query: "不存在"}).Nodes) != 0 {
		t.Fatal("search invented nodes")
	}
}

func TestAgentModelContextRemainsBoundedOnLargeSelection(t *testing.T) {
	canvas := AgentCanvasContext{Autonomy: "standard"}
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("node-%d", i)
		canvas.Nodes = append(canvas.Nodes, AgentCanvasNode{ID: id, Type: "text", Title: "文案", Width: 100, Height: 80, Content: strings.Repeat("字", 4000)})
		canvas.SelectedNodeIDs = append(canvas.SelectedNodeIDs, id)
	}
	normalized, err := normalizeAgentCanvasContext(canvas)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(normalized)
	context, err := agentModelCanvasContext(string(raw), nil)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Nodes   []AgentCanvasNode
		Summary struct {
			NodeCount int `json:"nodeCount"`
		}
	}
	if err := json.Unmarshal([]byte(context), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) > 60 || result.Summary.NodeCount != 1000 {
		t.Fatalf("unbounded context: nodes=%d summary=%d", len(result.Nodes), result.Summary.NodeCount)
	}
}

func TestAgentConfigurationDefaultsAndExplicitOverrides(t *testing.T) {
	authorization := agentNodeAuthorization{ConfigNodeIDs: map[string]struct{}{"config": {}}}
	context := `{"defaults":{"duration":"12"},"nodes":[{"id":"config","type":"config","settings":{"count":3,"duration":10}}]}`
	for _, tc := range []struct {
		name, raw       string
		count, duration int
	}{
		{"image.generate", `{"configNodeId":"config"}`, 3, 0},
		{"image.generate", `{"configNodeId":"config","count":2,"size":"2048x2048"}`, 2, 0},
		{"video.generate", `{"configNodeId":"config"}`, 0, 10},
		{"video.generate", `{"configNodeId":"config","duration":8}`, 0, 8},
		{"video.generate", `{"prompt":"商品展示"}`, 0, 12},
	} {
		_, raw, err := decodeAgentToolArguments(tc.name, agentMediaArgumentsWithDefaults(tc.name, tc.raw, context), authorization)
		if err != nil {
			t.Fatal(err)
		}
		var options AgentMediaOptions
		if err := json.Unmarshal([]byte(raw), &options); err != nil {
			t.Fatal(err)
		}
		if options.Count != tc.count || options.Duration != tc.duration {
			t.Fatalf("explicit/default options changed: %s", raw)
		}
	}
	if _, _, err := decodeAgentToolArguments("image.generate", `{"configNodeId":"foreign"}`, authorization); err == nil {
		t.Fatal("unowned config accepted")
	}
	for _, name := range []string{"canvas.read", "image.inspect", "video.inspect", "agent.ask_user", "agent.remember"} {
		if agentToolAdvancesPlan(name) {
			t.Fatalf("%s consumed a business step", name)
		}
	}
	if !agentToolAdvancesPlan("canvas.add_config") {
		t.Fatal("config did not advance plan")
	}
	steps := []model.AgentStep{{ToolName: "image.generate", Status: model.AgentStepStatusCompleted}}
	if agentMediaRevisionLimitReached(model.AgentRun{}, steps, "image.generate") {
		t.Fatal("second media batch mistaken for revision")
	}
}
