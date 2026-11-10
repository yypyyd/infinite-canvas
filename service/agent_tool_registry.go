package service

import (
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

// agentServerToolLimit 限制单轮服务端内联工具（计划、读取 Skill）的调用次数，它们不占用户的工具预算。
const agentServerToolLimit = 30

type agentToolDefinition struct {
	Name                 string
	Alias                string
	RequiresConfirmation bool
	Retryable            bool
	Revertible           bool
	MediaCost            int
	// Server 表示由服务端在规划循环内直接执行，不下发给画布页面。
	Server bool
	// Background 表示由服务端在后台执行（生成与视觉验收）：照常下发 tool.call，页面只负责把结果落到画布。
	Background bool
}

var agentToolRegistry = []agentToolDefinition{
	{Name: "canvas.plan", Alias: "canvas_plan", Server: true},
	{Name: "skill.read", Alias: "skill_read", Server: true},
	{Name: "canvas.read", Alias: "canvas_read"},
	{Name: "image.generate", Alias: "image_generate", Retryable: true, Revertible: true, MediaCost: 1, Background: true},
	{Name: "image.edit", Alias: "image_edit", Retryable: true, Revertible: true, MediaCost: 1, Background: true},
	{Name: "image.inspect", Alias: "image_inspect", Background: true},
	{Name: "video.generate", Alias: "video_generate", Retryable: true, Revertible: true, MediaCost: 1, Background: true},
	{Name: "video.inspect", Alias: "video_inspect", Background: true},
	{Name: "canvas.arrange", Alias: "canvas_arrange", Retryable: true, Revertible: true},
	{Name: "canvas.add_text", Alias: "canvas_add_text", Retryable: true, Revertible: true},
	{Name: "canvas.add_config", Alias: "canvas_add_config", Retryable: true, Revertible: true},
	{Name: "canvas.connect", Alias: "canvas_connect", Retryable: true, Revertible: true},
	{Name: "canvas.apply_ops", Alias: "canvas_apply_ops", Retryable: true, Revertible: true},
	{Name: "canvas.delete", Alias: "canvas_delete", RequiresConfirmation: true, Revertible: true},
	{Name: "canvas.update_text", Alias: "canvas_update_text", RequiresConfirmation: true, Revertible: true},
	{Name: "agent.ask_user", Alias: "agent_ask_user", RequiresConfirmation: true},
	{Name: "agent.remember", Alias: "agent_remember", RequiresConfirmation: true},
	{Name: "agent.forget", Alias: "agent_forget", RequiresConfirmation: true},
}

func agentToolDefinitionFor(name string) (agentToolDefinition, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, definition := range agentToolRegistry {
		if name == definition.Name || name == definition.Alias {
			return definition, true
		}
	}
	return agentToolDefinition{}, false
}

func agentToolRequiresConfirmation(name string) bool {
	definition, ok := agentToolDefinitionFor(name)
	return ok && definition.RequiresConfirmation
}

func agentServerTool(name string) bool {
	definition, ok := agentToolDefinitionFor(name)
	return ok && definition.Server
}

func agentBackgroundTool(name string) bool {
	definition, ok := agentToolDefinitionFor(name)
	return ok && definition.Background
}

// agentToolPresentation 是步骤创建时的落地状态：服务端生成的图片和视频需要页面落到画布，其余工具没有这一步。
func agentToolPresentation(name string) string {
	if definition, ok := agentToolDefinitionFor(name); ok && definition.Background && definition.MediaCost > 0 {
		return model.AgentPresentationPending
	}
	return ""
}

func agentToolAlias(name string) string {
	if definition, ok := agentToolDefinitionFor(name); ok {
		return definition.Alias
	}
	return strings.ReplaceAll(name, ".", "_")
}
