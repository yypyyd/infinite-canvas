package service

import (
	"regexp"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

var agentFakeMediaSuccessPattern = regexp.MustCompile(`(?i)已经为你生成|已为你生成|已生成.{0,24}(图|形象|画面|照片|视频|image|picture|video)|generated (an? |the )?(image|picture|video)|here('s| is) (an? |the )?(image|picture|video)`)

// agentClaimedMediaWithoutTools detects assistant prose that pretends media/canvas
// work finished when no tool_calls were returned.
func agentClaimedMediaWithoutTools(content string) bool {
	return agentFakeMediaSuccessPattern.MatchString(strings.TrimSpace(content))
}

// agentRunGeneratedMedia 判断本轮是否已有成功的媒体生成：有的话，正文里的“已生成”是如实汇报。
func agentRunGeneratedMedia(steps []model.AgentStep) bool {
	for _, step := range steps {
		if (step.ToolName == "image.generate" || step.ToolName == "image.edit" || step.ToolName == "video.generate") && agentStepSucceeded(step) {
			return true
		}
	}
	return false
}
