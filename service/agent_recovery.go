package service

import (
	"encoding/json"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type AgentRunRecovery struct {
	Run               model.AgentRun `json:"run"`
	Content           string         `json:"content"`
	ReferenceNodeIDs  []string       `json:"referenceNodeIds"`
	AuthorizedNodeIDs []string       `json:"authorizedNodeIds"`
}

func ListRecoverableAgentRuns(user model.AuthUser, projectID string) ([]AgentRunRecovery, error) {
	projectID = strings.TrimSpace(projectID)
	exists, err := repository.UserProjectExists(user.OrganizationID, user.ID, projectID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, safeMessageError{message: "画布项目不存在"}
	}
	runs, err := repository.ListActiveAgentRuns(user.OrganizationID, user.ID, projectID)
	if err != nil {
		return nil, err
	}
	items := make([]AgentRunRecovery, 0, len(runs))
	for _, run := range runs {
		message, err := repository.GetAgentMessage(user.OrganizationID, user.ID, run.MessageID)
		if err != nil {
			return nil, err
		}
		var context AgentCanvasContext
		if err := json.Unmarshal([]byte(run.Context), &context); err != nil {
			return nil, err
		}
		ids := make([]string, 0, len(context.Nodes))
		for _, node := range context.Nodes {
			ids = append(ids, node.ID)
		}
		items = append(items, AgentRunRecovery{Run: run, Content: message.Content, ReferenceNodeIDs: context.SelectedNodeIDs, AuthorizedNodeIDs: ids})
	}
	return items, nil
}
