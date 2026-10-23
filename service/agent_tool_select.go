package service

import (
	"regexp"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

// 常驻工具：读取、批量编辑、文字、连线、排列、规划与澄清。其余按本轮消息意图或已用过的工具按需附带，缩小请求体。
var agentCoreToolNames = map[string]struct{}{
	"canvas_read": {}, "canvas_plan": {}, "canvas_apply_ops": {}, "canvas_add_text": {}, "canvas_connect": {}, "canvas_arrange": {}, "agent_ask_user": {},
}

var agentToolIntentPatterns = []struct {
	pattern *regexp.Regexp
	tools   []string
}{
	{regexp.MustCompile(`(?i)图|画|照片|海报|插画|封面|头像|配图|视觉|渲染|image|photo|picture|draw|poster|生成`), []string{"image_generate", "image_edit", "image_inspect", "canvas_add_config"}},
	{regexp.MustCompile(`(?i)视频|短片|动画|影片|片段|video|clip|movie|animat`), []string{"video_generate", "video_inspect", "canvas_add_config"}},
	{regexp.MustCompile(`(?i)配置|config`), []string{"canvas_add_config"}},
	{regexp.MustCompile(`(?i)删|移除|清除|去掉|delete|remove`), []string{"canvas_delete"}},
	{regexp.MustCompile(`(?i)改|修改|更新|替换|重写|润色|edit|update|rewrite|replace`), []string{"canvas_update_text", "image_edit"}},
	{regexp.MustCompile(`(?i)记住|记忆|忘|偏好|remember|forget|memory`), []string{"agent_remember", "agent_forget"}},
}

// selectAgentToolSchemas 返回本轮需要发给模型的工具定义。
func selectAgentToolSchemas(content string, steps []model.AgentStep) []any {
	wanted := make(map[string]struct{}, len(agentCoreToolNames)+8)
	for name := range agentCoreToolNames {
		wanted[name] = struct{}{}
	}
	for _, intent := range agentToolIntentPatterns {
		if intent.pattern.MatchString(content) {
			for _, name := range intent.tools {
				wanted[name] = struct{}{}
			}
		}
	}
	for _, step := range steps {
		if step.ToolName != "" {
			wanted[strings.ReplaceAll(step.ToolName, ".", "_")] = struct{}{}
			switch step.ToolName {
			case "image.generate", "image.edit":
				wanted["image_inspect"] = struct{}{}
			case "video.generate":
				wanted["video_inspect"] = struct{}{}
			}
		}
	}
	all := agentToolSchemas()
	selected := make([]any, 0, len(wanted))
	for _, schema := range all {
		name, _ := schema.(map[string]any)["function"].(map[string]any)["name"].(string)
		if _, ok := wanted[name]; ok {
			selected = append(selected, schema)
		}
	}
	return selected
}

// agentIntentWantsImage reports whether the user message should trigger image tools.
func agentIntentWantsImage(content string) bool {
	for _, intent := range agentToolIntentPatterns {
		for _, name := range intent.tools {
			if name == "image_generate" && intent.pattern.MatchString(content) {
				return true
			}
		}
	}
	return false
}

func agentToolSchemasInclude(schemas []any, alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return false
	}
	for _, schema := range schemas {
		name, _ := schema.(map[string]any)["function"].(map[string]any)["name"].(string)
		if name == alias || name == strings.ReplaceAll(alias, "_", ".") || name == strings.ReplaceAll(alias, ".", "_") {
			return true
		}
	}
	return false
}

var agentFakeMediaSuccessPattern = regexp.MustCompile(`(?i)已经为你生成|已为你生成|已生成.{0,24}(图|形象|画面|照片|image|picture)|generated (an? |the )?(image|picture)|here('s| is) (an? |the )?(image|picture)`)

// agentClaimedMediaWithoutTools detects assistant prose that pretends media/canvas
// work finished when no tool_calls were returned.
func agentClaimedMediaWithoutTools(content string) bool {
	return agentFakeMediaSuccessPattern.MatchString(strings.TrimSpace(content))
}
