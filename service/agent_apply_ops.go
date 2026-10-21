package service

import (
	"errors"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	agentApplyOpsMax      = 30
	agentApplyOpsMinSize  = 40
	agentApplyOpsMaxSize  = 4000
	agentApplyOpsMaxCoord = 1e7
)

var agentApplyOpRefPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,39}$`)

// canvasApplyOp 是 canvas.apply_ops 的单步操作；nodeId/from/to/sourceNodeIds 可引用已有节点 id，
// 也可引用本批前面 add_* 操作的 ref。
type canvasApplyOp struct {
	Op            string   `json:"op"`
	Ref           string   `json:"ref,omitempty"`
	NodeID        string   `json:"nodeId,omitempty"`
	Text          string   `json:"text,omitempty"`
	X             *float64 `json:"x,omitempty"`
	Y             *float64 `json:"y,omitempty"`
	Width         *float64 `json:"width,omitempty"`
	Height        *float64 `json:"height,omitempty"`
	From          string   `json:"from,omitempty"`
	To            string   `json:"to,omitempty"`
	SourceNodeIDs []string `json:"sourceNodeIds,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	Prompt        string   `json:"prompt,omitempty"`
	Model         string   `json:"model,omitempty"`
	Size          string   `json:"size,omitempty"`
	Quality       string   `json:"quality,omitempty"`
	Resolution    string   `json:"resolution,omitempty"`
	Count         int      `json:"count,omitempty"`
	Duration      int      `json:"duration,omitempty"`
	GenerateAudio *bool    `json:"generateAudio,omitempty"`
}

type canvasApplyOpsArguments struct {
	Ops []canvasApplyOp `json:"ops"`
}

// AgentToolOpResult 是 canvas.apply_ops 每一步的执行回执。
type AgentToolOpResult struct {
	Op         string `json:"op"`
	Ref        string `json:"ref,omitempty"`
	NodeID     string `json:"nodeId,omitempty"`
	FromNodeID string `json:"fromNodeId,omitempty"`
	ToNodeID   string `json:"toNodeId,omitempty"`
}

func validAgentCoordinate(value *float64) bool {
	return value != nil && finite(*value) && math.Abs(*value) <= agentApplyOpsMaxCoord
}

func validAgentOptionalSize(value *float64) bool {
	return value == nil || (finite(*value) && *value >= agentApplyOpsMinSize && *value <= agentApplyOpsMaxSize)
}

// decodeCanvasApplyOps 校验批量操作：坐标必填且有限，引用只能指向授权节点或本批更早创建的 ref。
func decodeCanvasApplyOps(value string, authorization agentNodeAuthorization) (canvasApplyOpsArguments, error) {
	var input canvasApplyOpsArguments
	if err := decodeAgentJSONValue(value, &input); err != nil {
		return input, errors.New("canvas.apply_ops arguments invalid")
	}
	if len(input.Ops) < 1 || len(input.Ops) > agentApplyOpsMax {
		return input, errors.New("canvas.apply_ops ops invalid")
	}
	refs := map[string]string{}
	resolve := func(id string) (string, bool) {
		id = strings.TrimSpace(id)
		if kind, ok := refs[id]; ok {
			return kind, true
		}
		if _, ok := authorization.NodeIDs[id]; ok {
			if _, config := authorization.ConfigNodeIDs[id]; config {
				return "config", true
			}
			return "node", true
		}
		return "", false
	}
	for i := range input.Ops {
		op := &input.Ops[i]
		op.Op, op.Ref, op.NodeID, op.From, op.To = strings.TrimSpace(op.Op), strings.TrimSpace(op.Ref), strings.TrimSpace(op.NodeID), strings.TrimSpace(op.From), strings.TrimSpace(op.To)
		if !validAgentOptionalSize(op.Width) || !validAgentOptionalSize(op.Height) {
			return input, errors.New("canvas.apply_ops size invalid")
		}
		seen := make(map[string]struct{}, len(op.SourceNodeIDs))
		for j, id := range op.SourceNodeIDs {
			id = strings.TrimSpace(id)
			if _, ok := resolve(id); !ok || id == "" {
				return input, errors.New("canvas.apply_ops sourceNodeIds unauthorized")
			}
			if _, exists := seen[id]; exists {
				return input, errors.New("canvas.apply_ops sourceNodeIds invalid")
			}
			seen[id], op.SourceNodeIDs[j] = struct{}{}, id
		}
		if len(op.SourceNodeIDs) > 20 {
			return input, errors.New("canvas.apply_ops sourceNodeIds invalid")
		}
		switch op.Op {
		case "add_text", "add_config":
			if !agentApplyOpRefPattern.MatchString(op.Ref) {
				return input, errors.New("canvas.apply_ops ref invalid")
			}
			if _, exists := refs[op.Ref]; exists {
				return input, errors.New("canvas.apply_ops ref duplicated")
			}
			if _, exists := authorization.NodeIDs[op.Ref]; exists {
				return input, errors.New("canvas.apply_ops ref conflicts with node id")
			}
			if !validAgentCoordinate(op.X) || !validAgentCoordinate(op.Y) {
				return input, errors.New("canvas.apply_ops position invalid")
			}
			if op.NodeID != "" || op.From != "" || op.To != "" {
				return input, errors.New("canvas.apply_ops arguments invalid")
			}
			if op.Op == "add_text" {
				op.Text = strings.TrimSpace(op.Text)
				if op.Text == "" || utf8.RuneCountInString(op.Text) > 4000 || op.Mode != "" || op.Prompt != "" || op.Model != "" || op.Size != "" || op.Quality != "" || op.Resolution != "" || op.Count != 0 || op.Duration != 0 || op.GenerateAudio != nil {
					return input, errors.New("canvas.apply_ops add_text invalid")
				}
				refs[op.Ref] = "text"
				continue
			}
			op.Mode = strings.TrimSpace(op.Mode)
			if op.Mode == "" {
				op.Mode = "image"
			}
			if op.Text != "" || (op.Mode != "image" && op.Mode != "video") || utf8.RuneCountInString(op.Prompt) > 8000 || op.Count < 0 || op.Count > 4 || op.Duration < 0 || op.Duration > 20 {
				return input, errors.New("canvas.apply_ops add_config invalid")
			}
			if err := validateAgentMediaOptions(AgentMediaOptions{Model: op.Model, Size: op.Size, Quality: op.Quality, Resolution: op.Resolution, GenerateAudio: op.GenerateAudio, Mode: op.Mode, Count: op.Count, Duration: op.Duration}, authorization); err != nil {
				return input, err
			}
			for _, id := range op.SourceNodeIDs {
				if kind, _ := resolve(id); kind == "config" {
					return input, errors.New("canvas.apply_ops cannot link two config nodes")
				}
			}
			refs[op.Ref] = "config"
		case "connect":
			if op.Ref != "" || op.NodeID != "" || op.X != nil || op.Y != nil || op.Width != nil || op.Height != nil || op.Text != "" || len(op.SourceNodeIDs) != 0 {
				return input, errors.New("canvas.apply_ops connect invalid")
			}
			fromKind, fromOK := resolve(op.From)
			toKind, toOK := resolve(op.To)
			if !fromOK || !toOK || op.From == op.To {
				return input, errors.New("canvas.apply_ops connect unauthorized")
			}
			if fromKind == "config" && toKind == "config" {
				return input, errors.New("canvas.apply_ops cannot link two config nodes")
			}
		case "move", "resize":
			if op.Ref != "" || op.From != "" || op.To != "" || op.Text != "" || len(op.SourceNodeIDs) != 0 {
				return input, errors.New("canvas.apply_ops " + op.Op + " invalid")
			}
			if _, ok := resolve(op.NodeID); !ok || op.NodeID == "" {
				return input, errors.New("canvas.apply_ops nodeId unauthorized")
			}
			if op.Op == "move" && (!validAgentCoordinate(op.X) || !validAgentCoordinate(op.Y) || op.Width != nil || op.Height != nil) {
				return input, errors.New("canvas.apply_ops move invalid")
			}
			if op.Op == "resize" && (op.Width == nil || op.Height == nil || op.X != nil || op.Y != nil) {
				return input, errors.New("canvas.apply_ops resize invalid")
			}
		default:
			return input, errors.New("canvas.apply_ops op invalid")
		}
		if op.Op != "add_config" && (op.Mode != "" || op.Prompt != "" || op.Model != "" || op.Size != "" || op.Quality != "" || op.Resolution != "" || op.Count != 0 || op.Duration != 0 || op.GenerateAudio != nil) {
			return input, errors.New("canvas.apply_ops arguments invalid")
		}
	}
	return input, nil
}

// validateCanvasApplyOpsResult 校验浏览器回执与参数逐项一致，并解析 ref 对应的新节点 id。
func validateCanvasApplyOpsResult(input canvasApplyOpsArguments, request *SubmitAgentToolResultRequest) error {
	invalid := safeMessageError{message: "批量画布操作结果无效"}
	if request.Plan != nil || request.Video != nil || len(request.Images) != 0 || len(request.Positions) != 0 || request.NodeID != "" || request.Text != "" || request.Placement != "" || request.FromNodeID != "" || request.ToNodeID != "" || len(request.Ops) != len(input.Ops) {
		return invalid
	}
	created := map[string]string{}
	resolve := func(id string) string {
		if nodeID, ok := created[id]; ok {
			return nodeID
		}
		return id
	}
	touched := []string{}
	seenTouched := map[string]struct{}{}
	touch := func(id string) {
		if _, ok := seenTouched[id]; !ok {
			seenTouched[id] = struct{}{}
			touched = append(touched, id)
		}
	}
	for i, op := range input.Ops {
		result := &request.Ops[i]
		result.Op, result.Ref, result.NodeID, result.FromNodeID, result.ToNodeID = strings.TrimSpace(result.Op), strings.TrimSpace(result.Ref), strings.TrimSpace(result.NodeID), strings.TrimSpace(result.FromNodeID), strings.TrimSpace(result.ToNodeID)
		if result.Op != op.Op || result.Ref != op.Ref {
			return invalid
		}
		switch op.Op {
		case "add_text", "add_config":
			if result.NodeID == "" || len(result.NodeID) > 191 || result.FromNodeID != "" || result.ToNodeID != "" {
				return invalid
			}
			if _, exists := seenTouched[result.NodeID]; exists {
				return invalid
			}
			created[op.Ref] = result.NodeID
			touch(result.NodeID)
		case "connect":
			// 画布会按节点类型规范连线方向，回执允许与请求方向相反。
			from, to := resolve(op.From), resolve(op.To)
			if result.NodeID != "" || !((result.FromNodeID == from && result.ToNodeID == to) || (result.FromNodeID == to && result.ToNodeID == from)) {
				return invalid
			}
		case "move", "resize":
			if result.NodeID != resolve(op.NodeID) || result.FromNodeID != "" || result.ToNodeID != "" {
				return invalid
			}
			touch(result.NodeID)
		}
	}
	if len(touched) == 0 {
		if len(request.NodeIDs) != 0 {
			return invalid
		}
		return nil
	}
	if !sameTrimmedStringSet(request.NodeIDs, touched) {
		return invalid
	}
	return nil
}
func agentApplyOpsSchema() map[string]any {
	number := map[string]any{"type": "number"}
	size := map[string]any{"type": "number", "minimum": agentApplyOpsMinSize, "maximum": agentApplyOpsMaxSize}
	nodeRef := map[string]any{"type": "string", "maxLength": 191}
	op := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"op":            map[string]any{"type": "string", "enum": []string{"add_text", "add_config", "connect", "move", "resize"}},
			"ref":           map[string]any{"type": "string", "maxLength": 40, "description": "add_text/add_config 必填，本批内唯一的临时名称，字母开头，后续操作可用它引用新节点"},
			"nodeId":        map[string]any{"type": "string", "maxLength": 191, "description": "move/resize 的目标：已有节点 id 或本批 ref"},
			"text":          map[string]any{"type": "string", "maxLength": 4000, "description": "add_text 的文本内容"},
			"x":             number,
			"y":             number,
			"width":         size,
			"height":        size,
			"from":          nodeRef,
			"to":            nodeRef,
			"sourceNodeIds": map[string]any{"type": "array", "maxItems": 20, "uniqueItems": true, "items": nodeRef},
			"mode":          map[string]any{"type": "string", "enum": []string{"image", "video"}},
			"prompt":        map[string]any{"type": "string", "maxLength": 8000},
			"model":         map[string]any{"type": "string", "maxLength": 120},
			"size":          map[string]any{"type": "string", "maxLength": 120},
			"quality":       map[string]any{"type": "string", "maxLength": 120},
			"resolution":    map[string]any{"type": "string", "maxLength": 120},
			"count":         map[string]any{"type": "integer", "minimum": 1, "maximum": 4},
			"duration":      map[string]any{"type": "integer", "minimum": 1, "maximum": 20},
			"generateAudio": map[string]any{"type": "boolean"},
		},
		"required":             []string{"op"},
		"additionalProperties": false,
	}
	return map[string]any{"type": "function", "function": map[string]any{
		"name":        "canvas_apply_ops",
		"description": "按画布坐标批量编辑画布，按顺序逐步执行并实时展示：add_text(ref,text,x,y[,width,height,sourceNodeIds])、add_config(ref,x,y,mode,prompt[,生成规格,sourceNodeIds])、connect(from,to)、move(nodeId,x,y)、resize(nodeId,width,height)。x/y 是节点左上角坐标；只能引用本轮授权节点或本批更早的 ref；不能删除节点或修改已有文本",
		"parameters": map[string]any{
			"type":                 "object",
			"properties":           map[string]any{"ops": map[string]any{"type": "array", "minItems": 1, "maxItems": agentApplyOpsMax, "items": op}},
			"required":             []string{"ops"},
			"additionalProperties": false,
		},
	}}
}
