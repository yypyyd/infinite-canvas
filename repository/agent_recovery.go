package repository

import "github.com/yypyyd/infinite-canvas/model"

func ListActiveAgentRuns(organizationID, userID, projectID string) ([]model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	runs := []model.AgentRun{}
	err = db.Where("organization_id = ? AND user_id = ? AND status IN ? AND session_id IN (?)", organizationID, userID,
		[]model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingTool, model.AgentRunStatusWaitingConfirmation},
		db.Model(&model.AgentSession{}).Select("id").Where("organization_id = ? AND user_id = ? AND project_id = ?", organizationID, userID, projectID),
	).Order("created_at asc").Find(&runs).Error
	return runs, err
}

func GetAgentMessage(organizationID, userID, messageID string) (model.AgentMessage, error) {
	db, err := DB()
	if err != nil {
		return model.AgentMessage{}, err
	}
	var message model.AgentMessage
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, messageID).First(&message).Error
	return message, err
}
