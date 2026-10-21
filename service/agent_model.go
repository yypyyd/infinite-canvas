package service

import (
	"regexp"
	"sort"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// agentFallbackModel 是用户默认文本模型不支持工具调用时画布 Agent 优先回退的模型。
const agentFallbackModel = "grok-4.6"

// agentModelSupportsTools 判断模型能否用于画布 Agent；未标注（nil）时视为支持以兼容旧配置。
func agentModelSupportsTools(item model.ModelDefinition) bool {
	return item.SupportsTools == nil || *item.SupportsTools
}

// resolveAgentModel 把请求的模型解析为可调用工具的文本模型。
func resolveAgentModel(requested string) (string, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return requested, err
	}
	return resolveAgentModelFrom(settings.Public.ModelChannel.Models, requested)
}

func resolveAgentModelFrom(models []model.ModelDefinition, requested string) (string, error) {
	found := false
	for _, item := range models {
		if item.ID == requested {
			found = true
			if agentModelSupportsTools(item) {
				return requested, nil
			}
			break
		}
	}
	if !found {
		// 目录外模型交给渠道选择按原逻辑处理。
		return requested, nil
	}
	usable := func(item model.ModelDefinition) bool {
		return item.Enabled && item.Modality == "text" && agentModelSupportsTools(item)
	}
	for _, item := range models {
		if item.ID == agentFallbackModel && usable(item) {
			return item.ID, nil
		}
	}
	candidates := make([]model.ModelDefinition, 0, len(models))
	for _, item := range models {
		if usable(item) {
			candidates = append(candidates, item)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		explicitI := candidates[i].SupportsTools != nil
		explicitJ := candidates[j].SupportsTools != nil
		if explicitI != explicitJ {
			return explicitI
		}
		return candidates[i].Sort < candidates[j].Sort
	})
	if len(candidates) > 0 {
		return candidates[0].ID, nil
	}
	return "", safeMessageError{message: "当前没有支持工具调用的文本模型，请联系管理员"}
}

// keepModelToolSupport 在后台保存未携带 supportsTools 时保留已保存的标记，避免旧版界面覆盖。
func keepModelToolSupport(next *model.Settings, saved model.Settings) {
	previous := map[string]bool{}
	for _, item := range saved.Public.ModelChannel.Models {
		if item.SupportsTools != nil {
			previous[item.ID] = *item.SupportsTools
		}
	}
	for i := range next.Public.ModelChannel.Models {
		item := &next.Public.ModelChannel.Models[i]
		if item.SupportsTools != nil {
			continue
		}
		if value, ok := previous[item.ID]; ok {
			copied := value
			item.SupportsTools = &copied
		}
	}
}

var agentFakeToolCallPattern = regexp.MustCompile(`(?i)<\s*function[\s_]*call|<\s*tool_call|<\s*invoke\b|<\s*(box|icon|row|title|entity)\b|_parameters\b|"tool_calls"\s*:`)

// agentStreamNeedsPlainRetry 判断流式回复是否把工具调用伪装成了文本或界面标记。
func agentStreamNeedsPlainRetry(bodyValue map[string]any, message agentUpstreamMessage) bool {
	if _, hasTools := bodyValue["tools"]; !hasTools || len(message.ToolCalls) > 0 {
		return false
	}
	return agentFakeToolCallPattern.MatchString(message.Content)
}
