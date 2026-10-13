package service

import (
	"strings"
	"unicode/utf8"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type AgentMemoryUpdate struct {
	Content string `json:"content"`
	Forget  bool   `json:"forget"`
}
type AgentMemoryList struct {
	Items []model.AgentMemory `json:"items"`
	Total int64               `json:"total"`
}

func ListAgentMemories(user model.AuthUser, projectID string, query model.Query) (AgentMemoryList, error) {
	exists, err := repository.UserProjectExists(user.OrganizationID, user.ID, strings.TrimSpace(projectID))
	if err != nil {
		return AgentMemoryList{}, err
	}
	if !exists {
		return AgentMemoryList{}, safeMessageError{message: "画布项目不存在"}
	}
	items, total, err := repository.ListManagedAgentMemories(user.OrganizationID, user.ID, strings.TrimSpace(projectID), query)
	return AgentMemoryList{Items: items, Total: total}, err
}

func UpdateAgentMemory(user model.AuthUser, id string, request AgentMemoryUpdate) (model.AgentMemory, error) {
	if err := RequireOrganizationWrite(user); err != nil {
		return model.AgentMemory{}, err
	}
	item, err := repository.GetOwnedAgentMemory(user.OrganizationID, user.ID, strings.TrimSpace(id))
	if err != nil {
		return item, err
	}
	request.Content = strings.TrimSpace(request.Content)
	if !request.Forget && (request.Content == "" || utf8.RuneCountInString(request.Content) > 1000) {
		return item, safeMessageError{message: "记忆内容需为 1 至 1000 字"}
	}
	item.UpdatedAt = now()
	if request.Forget {
		item.Status, item.ForgottenAt = model.AgentMemoryStatusForgotten, item.UpdatedAt
	} else {
		item.Content, item.Confidence = request.Content, 1
	}
	err = repository.UpdateOwnedAgentMemory(user.OrganizationID, user.ID, item.ID, item.Content, item.UpdatedAt, request.Forget)
	return item, err
}
