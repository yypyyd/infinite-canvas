package service

import "strings"

type agentToolDefinition struct {
	Name                 string
	Alias                string
	RequiresConfirmation bool
	Retryable            bool
	Revertible           bool
	MediaCost            int
}

var agentToolRegistry = []agentToolDefinition{
	{Name: "canvas.plan", Alias: "canvas_plan"},
	{Name: "image.generate", Alias: "image_generate", Retryable: true, Revertible: true, MediaCost: 1},
	{Name: "image.edit", Alias: "image_edit", Retryable: true, Revertible: true, MediaCost: 1},
	{Name: "image.inspect", Alias: "image_inspect"},
	{Name: "video.generate", Alias: "video_generate", Retryable: true, Revertible: true, MediaCost: 1},
	{Name: "video.inspect", Alias: "video_inspect"},
	{Name: "canvas.arrange", Alias: "canvas_arrange", Retryable: true, Revertible: true},
	{Name: "canvas.add_text", Alias: "canvas_add_text", Retryable: true, Revertible: true},
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
