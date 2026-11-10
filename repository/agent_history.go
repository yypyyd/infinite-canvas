package repository

import (
	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

func ListAgentSessions(organizationID, userID, projectID string, limit int) ([]model.AgentSession, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	sessions := []model.AgentSession{}
	err = db.Where("organization_id = ? AND user_id = ? AND project_id = ? AND status = ?", organizationID, userID, projectID, model.AgentSessionStatusActive).
		Order("updated_at desc").Limit(limit).Find(&sessions).Error
	return sessions, err
}

func ListAgentSessionMessages(organizationID, userID, sessionID string, limit int) ([]model.AgentMessage, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	messages := []model.AgentMessage{}
	err = db.Where("organization_id = ? AND user_id = ? AND session_id = ?", organizationID, userID, sessionID).Order("sequence desc").Limit(limit).Find(&messages).Error
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, err
}

func ListAgentRunsByMessages(organizationID, userID string, messageIDs []string) ([]model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	runs := []model.AgentRun{}
	if len(messageIDs) == 0 {
		return runs, nil
	}
	err = db.Where("organization_id = ? AND user_id = ? AND message_id IN ?", organizationID, userID, messageIDs).Order("created_at asc").Find(&runs).Error
	return runs, err
}

func ListAgentStepsByRuns(organizationID, userID string, runIDs []string) ([]model.AgentStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	steps := []model.AgentStep{}
	if len(runIDs) == 0 {
		return steps, nil
	}
	// 同一批并行工具的创建时间相同，补上 id 让回放顺序稳定。
	err = db.Where("organization_id = ? AND user_id = ? AND run_id IN ?", organizationID, userID, runIDs).Order("created_at asc, id asc").Find(&steps).Error
	return steps, err
}

// ListPendingAgentPresentations 返回这些运行里已经生成完成、还没落到画布的工具步骤。
func ListPendingAgentPresentations(organizationID, userID string, runIDs []string) ([]model.AgentStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	steps := []model.AgentStep{}
	if len(runIDs) == 0 {
		return steps, nil
	}
	err = db.Select("run_id", "tool_call_id").Where("organization_id = ? AND user_id = ? AND run_id IN ? AND type = ? AND status = ? AND present_token NOT IN ?", organizationID, userID, runIDs, model.AgentStepTypeTool, model.AgentStepStatusCompleted, []string{"", model.AgentPresentationDone}).Find(&steps).Error
	return steps, err
}

func ListAgentEventsByRuns(organizationID, userID string, runIDs []string) ([]model.AgentEvent, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	events := []model.AgentEvent{}
	if len(runIDs) == 0 {
		return events, nil
	}
	err = db.Where("organization_id = ? AND user_id = ? AND run_id IN ?", organizationID, userID, runIDs).Order("run_id asc, sequence asc").Find(&events).Error
	return events, err
}

func ArchiveAgentSession(organizationID, userID, sessionID, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.AgentSession{}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, sessionID).Updates(map[string]any{"status": model.AgentSessionStatusArchived, "updated_at": timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
