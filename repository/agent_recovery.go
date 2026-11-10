package repository

import "github.com/yypyyd/infinite-canvas/model"

// ListActiveAgentRuns 返回项目里需要页面接着处理的运行：尚未结束的，以及已有生成结果还没落到画布的。
func ListActiveAgentRuns(organizationID, userID, projectID string) ([]model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	runs := []model.AgentRun{}
	err = db.Where("organization_id = ? AND user_id = ? AND session_id IN (?)", organizationID, userID,
		db.Model(&model.AgentSession{}).Select("id").Where("organization_id = ? AND user_id = ? AND project_id = ? AND status = ?", organizationID, userID, projectID, model.AgentSessionStatusActive),
	).Where("status IN ? OR id IN (?)",
		[]model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingTool, model.AgentRunStatusWaitingConfirmation},
		db.Model(&model.AgentStep{}).Select("run_id").Where("organization_id = ? AND user_id = ? AND type = ? AND status = ? AND present_token NOT IN ?", organizationID, userID, model.AgentStepTypeTool, model.AgentStepStatusCompleted, []string{"", model.AgentPresentationDone}),
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
