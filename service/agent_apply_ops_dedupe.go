package service

import (
	"encoding/json"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

// agentRunCanvasLedger 汇总本轮已真实执行成功的画布改动：新建节点（按内容索引）与已建立的连线。
// 模型在后续轮次重复下发同样的新增或连线时，据此按操作去重，避免画布上出现重复的一排节点。
type agentRunCanvasLedger struct {
	textNodes   map[string]string
	configNodes map[string]string
	connections map[string]struct{}
	created     []agentLedgerNode
}

type agentLedgerNode struct {
	Ref    string `json:"ref,omitempty"`
	NodeID string `json:"nodeId"`
	Kind   string `json:"kind"`
	Text   string `json:"text,omitempty"`
}

func agentLedgerTextKey(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func agentLedgerConfigKey(mode, prompt string) string {
	if mode == "" {
		mode = "image"
	}
	return mode + "\x00" + agentLedgerTextKey(prompt)
}

func agentLedgerConnectionKey(from, to string) string {
	if from > to {
		from, to = to, from
	}
	return from + "\x00" + to
}

func buildAgentRunCanvasLedger(steps []model.AgentStep) agentRunCanvasLedger {
	ledger := agentRunCanvasLedger{textNodes: map[string]string{}, configNodes: map[string]string{}, connections: map[string]struct{}{}}
	for _, step := range steps {
		if !agentStepSucceeded(step) {
			continue
		}
		var result SubmitAgentToolResultRequest
		if json.Unmarshal([]byte(step.Output), &result) != nil {
			continue
		}
		switch step.ToolName {
		case "canvas.apply_ops":
			var input canvasApplyOpsArguments
			if json.Unmarshal([]byte(step.Input), &input) != nil || len(input.Ops) != len(result.Ops) {
				continue
			}
			for i, op := range input.Ops {
				receipt := result.Ops[i]
				switch op.Op {
				case "add_text":
					if receipt.NodeID != "" {
						ledger.recordNode(op.Ref, receipt.NodeID, "text", op.Text, agentLedgerTextKey(op.Text))
					}
				case "add_config":
					if receipt.NodeID != "" {
						ledger.recordNode(op.Ref, receipt.NodeID, "config", op.Prompt, agentLedgerConfigKey(op.Mode, op.Prompt))
					}
				case "connect":
					if receipt.FromNodeID != "" && receipt.ToNodeID != "" {
						ledger.connections[agentLedgerConnectionKey(receipt.FromNodeID, receipt.ToNodeID)] = struct{}{}
					}
				}
			}
		case "canvas.add_text":
			var input struct {
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(step.Input), &input) == nil && result.NodeID != "" {
				ledger.recordNode("", result.NodeID, "text", input.Text, agentLedgerTextKey(input.Text))
			}
		case "canvas.connect":
			if result.FromNodeID != "" && result.ToNodeID != "" {
				ledger.connections[agentLedgerConnectionKey(result.FromNodeID, result.ToNodeID)] = struct{}{}
			}
		}
	}
	return ledger
}

func (ledger *agentRunCanvasLedger) recordNode(ref, nodeID, kind, text, key string) {
	index := ledger.textNodes
	if kind == "config" {
		index = ledger.configNodes
	}
	if key != "" && strings.TrimSpace(text) != "" {
		if _, exists := index[key]; !exists {
			index[key] = nodeID
		}
	}
	ledger.created = append(ledger.created, agentLedgerNode{Ref: ref, NodeID: nodeID, Kind: kind, Text: truncateAgentRunes(text, 80)})
}

// dedupeAgentApplyOps 去掉本轮已执行过的新增与连线：重复新增的 ref 改指向已创建节点，
// 已存在的连线直接跳过。返回改写后的参数 JSON、被跳过的操作数，以及是否已无剩余操作。
func dedupeAgentApplyOps(arguments string, steps []model.AgentStep) (string, int, bool) {
	ledger := buildAgentRunCanvasLedger(steps)
	if len(ledger.created) == 0 && len(ledger.connections) == 0 {
		return arguments, 0, false
	}
	var input canvasApplyOpsArguments
	if decodeAgentJSONValue(arguments, &input) != nil || len(input.Ops) == 0 {
		return arguments, 0, false
	}
	remap := map[string]string{}
	resolve := func(id string) string {
		id = strings.TrimSpace(id)
		if existing, ok := remap[id]; ok {
			return existing
		}
		return id
	}
	pending := map[string]struct{}{}
	kept := make([]canvasApplyOp, 0, len(input.Ops))
	skipped := 0
	for _, op := range input.Ops {
		op.Op = strings.TrimSpace(op.Op)
		switch op.Op {
		case "add_text", "add_config":
			key, index := agentLedgerTextKey(op.Text), ledger.textNodes
			if op.Op == "add_config" {
				key, index = agentLedgerConfigKey(strings.TrimSpace(op.Mode), op.Prompt), ledger.configNodes
			}
			if existing, ok := index[key]; ok && strings.TrimSpace(op.Text+op.Prompt) != "" {
				remap[strings.TrimSpace(op.Ref)] = existing
				skipped++
				continue
			}
			for i := range op.SourceNodeIDs {
				op.SourceNodeIDs[i] = resolve(op.SourceNodeIDs[i])
			}
		case "connect":
			op.From, op.To = resolve(op.From), resolve(op.To)
			key := agentLedgerConnectionKey(op.From, op.To)
			if _, ok := ledger.connections[key]; ok {
				skipped++
				continue
			}
			if _, ok := pending[key]; ok {
				skipped++
				continue
			}
			pending[key] = struct{}{}
		case "move", "resize":
			op.NodeID = resolve(op.NodeID)
		}
		kept = append(kept, op)
	}
	if skipped == 0 {
		return arguments, 0, false
	}
	if len(kept) == 0 {
		return "", skipped, true
	}
	encoded, err := json.Marshal(canvasApplyOpsArguments{Ops: kept})
	if err != nil {
		return arguments, 0, false
	}
	return string(encoded), skipped, false
}

// agentApplyOpsModelResult 给模型的批量操作回执：逐项列出新建节点 id 与连线，明确这些改动已真实生效。
func agentApplyOpsModelResult(step model.AgentStep, request SubmitAgentToolResultRequest) map[string]any {
	var input canvasApplyOpsArguments
	_ = json.Unmarshal([]byte(step.Input), &input)
	created := make([]agentLedgerNode, 0)
	connected := make([]map[string]string, 0)
	moved := make([]string, 0)
	for i, receipt := range request.Ops {
		switch receipt.Op {
		case "add_text", "add_config":
			node := agentLedgerNode{Ref: receipt.Ref, NodeID: receipt.NodeID, Kind: strings.TrimPrefix(receipt.Op, "add_")}
			if i < len(input.Ops) {
				node.Text = truncateAgentRunes(input.Ops[i].Text+input.Ops[i].Prompt, 80)
			}
			created = append(created, node)
		case "connect":
			connected = append(connected, map[string]string{"from": receipt.FromNodeID, "to": receipt.ToNodeID})
		case "move", "resize":
			moved = append(moved, receipt.NodeID)
		}
	}
	return map[string]any{
		"status":    request.Status,
		"created":   created,
		"connected": connected,
		"moved":     moved,
		"ops":       request.Ops,
		"note":      "以上节点与连线已真实存在于画布，不要再次创建；后续操作直接引用这些 nodeId，只补做尚未完成的部分",
	}
}
