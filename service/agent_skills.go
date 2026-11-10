package service

import (
	"embed"
	"strings"
)

//go:embed agent_skills/*.md
var agentSkillFiles embed.FS

type agentSkill struct {
	ID          string
	Title       string
	Description string
}

// agentSkills 是模型可读取的专业方法论；读取时会附带 creative-core 通用规范。
var agentSkills = []agentSkill{
	{ID: "product-image", Title: "电商商品图", Description: "商品主图、白底图、场景图、卖点图、细节图和整套商品图"},
	{ID: "poster-design", Title: "海报与详情页", Description: "活动海报、banner、社媒封面、详情页模块与文案层级"},
	{ID: "video-direction", Title: "短视频与营销视频", Description: "视频脚本、分镜、镜头语言、视频提示词、关键帧与多段一致性"},
	{ID: "canvas-workflow", Title: "画布工作流", Description: "搭建可复用的生成流程、配置节点、批量摆放与连线"},
}

type AgentToolSkill struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content,omitempty"`
}

func agentSkillFor(id string) (agentSkill, bool) {
	for _, skill := range agentSkills {
		if skill.ID == id {
			return skill, true
		}
	}
	return agentSkill{}, false
}

func agentSkillIDs() []string {
	ids := make([]string, 0, len(agentSkills))
	for _, skill := range agentSkills {
		ids = append(ids, skill.ID)
	}
	return ids
}

func agentSkillContent(id string) (string, error) {
	body, err := agentSkillFiles.ReadFile("agent_skills/" + id + ".md")
	if err != nil {
		return "", err
	}
	core, err := agentSkillFiles.ReadFile("agent_skills/creative-core.md")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)) + "\n\n" + strings.TrimSpace(string(core)), nil
}

func agentSkillRoutingPrompt() string {
	lines := make([]string, 0, len(agentSkills))
	for _, skill := range agentSkills {
		lines = append(lines, "- "+skill.ID+"（"+skill.Title+"）："+skill.Description)
	}
	return strings.Join(lines, "\n")
}
