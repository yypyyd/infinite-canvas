package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"
)

func validateAgentMediaOptions(value AgentMediaOptions, authorization agentNodeAuthorization) error {
	for _, text := range []string{value.Model, value.Size, value.Quality, value.Resolution, value.ConfigNodeID} {
		if utf8.RuneCountInString(text) > 120 {
			return errors.New("media options invalid")
		}
	}
	if value.Mode != "" && value.Mode != "image" && value.Mode != "video" {
		return errors.New("media mode invalid")
	}
	if value.Count < 0 || value.Count > 4 || value.Duration < 0 || value.Duration > 20 {
		return errors.New("media count or duration invalid")
	}
	if value.ConfigNodeID != "" {
		if _, ok := authorization.ConfigNodeIDs[value.ConfigNodeID]; !ok {
			return errors.New("configNodeId unauthorized")
		}
	}
	return nil
}

// A read is an observation of the current executor state, never an interpretation of user intent.
func agentCanvasObservation(context AgentCanvasContext, input canvasReadArguments) AgentCanvasContext {
	ids, query := stringSet(input.NodeIDs), strings.ToLower(input.Query)
	result := AgentCanvasContext{Autonomy: context.Autonomy, Nodes: []AgentCanvasNode{}, Connections: []AgentCanvasConnection{}, Defaults: context.Defaults}
	matched := 0
	for _, node := range context.Nodes {
		if len(ids) > 0 {
			if _, ok := ids[node.ID]; !ok {
				continue
			}
		}
		if query != "" && !strings.Contains(strings.ToLower(node.Title+" "+node.Content+" "+node.Prompt), query) {
			continue
		}
		matched++
		if matched <= input.Offset || len(result.Nodes) >= 20 {
			continue
		}
		node.StorageKey = ""
		if len(ids) == 0 {
			node.Content, node.Prompt = truncateAgentRunes(node.Content, 800), truncateAgentRunes(node.Prompt, 800)
		}
		result.Nodes = append(result.Nodes, node)
	}
	retained := map[string]bool{}
	for _, node := range result.Nodes {
		retained[node.ID] = true
	}
	for _, connection := range context.Connections {
		if retained[connection.From] || retained[connection.To] {
			result.Connections = append(result.Connections, connection)
		}
		if len(result.Connections) >= 100 {
			break
		}
	}
	return result
}

func agentModelCapabilities() string {
	settings, err := PublicSettings()
	if err != nil {
		return ""
	}
	encoded, _ := json.Marshal(settings.ModelChannel.Models)
	return "可用模型能力（仅按支持的规格调用；defaults 是当前默认设置，配置节点 settings 优先，明确工具参数优先于配置）：" + string(encoded)
}

func agentMediaArgumentsWithDefaults(name, raw, context string) string {
	if name != "image.generate" && name != "image.edit" && name != "video.generate" {
		return raw
	}
	var input map[string]any
	var canvas AgentCanvasContext
	if json.Unmarshal([]byte(raw), &input) != nil || json.Unmarshal([]byte(context), &canvas) != nil {
		return raw
	}
	configID, _ := input["configNodeId"].(string)
	for _, node := range canvas.Nodes {
		if node.ID != configID || node.Type != "config" || node.Settings == nil {
			continue
		}
		if name == "video.generate" {
			if _, present := input["duration"]; !present && node.Settings.Duration > 0 {
				input["duration"] = node.Settings.Duration
			}
		} else if _, present := input["count"]; !present && node.Settings.Count > 0 {
			input["count"] = node.Settings.Count
		}
		break
	}
	if name == "video.generate" {
		if _, present := input["duration"]; !present {
			if duration, err := strconv.Atoi(canvas.Defaults["duration"]); err == nil && duration > 0 {
				input["duration"] = duration
			}
		}
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return raw
	}
	return string(encoded)
}
