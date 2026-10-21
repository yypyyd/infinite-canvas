package repository

import (
	"errors"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const agentMemoryMaxLimit = 40

var (
	ErrAgentToolResultConflict   = errors.New("agent tool result conflicts with saved result")
	ErrAgentToolExecutionClaimed = errors.New("agent tool execution already claimed")
	ErrAgentToolNotRevertible    = errors.New("agent tool cannot be reverted")
	ErrAgentStepNotRetryable     = errors.New("agent step cannot be retried")
)

func CreateAgentSession(session model.AgentSession) (model.AgentSession, error) {
	db, err := DB()
	if err != nil {
		return session, err
	}
	var existing model.AgentSession
	if err := db.Where("organization_id = ? AND user_id = ? AND id = ?", session.OrganizationID, session.UserID, session.ID).First(&existing).Error; err == nil {
		if existing.ProjectID != session.ProjectID || existing.Profile != session.Profile || existing.Title != session.Title {
			return session, ErrAgentToolResultConflict
		}
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return session, err
	}
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error { return tx.Create(&session).Error })
	if err != nil {
		if lookupErr := db.Where("organization_id = ? AND user_id = ? AND id = ?", session.OrganizationID, session.UserID, session.ID).First(&existing).Error; lookupErr == nil {
			if existing.ProjectID != session.ProjectID || existing.Profile != session.Profile || existing.Title != session.Title {
				return session, ErrAgentToolResultConflict
			}
			return existing, nil
		}
	}
	return session, err
}

func UserProjectExists(organizationID, userID, projectID string) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var count int64
	err = db.Model(&model.UserProject{}).Where("organization_id = ? AND user_id = ? AND id = ? AND deleted_at = ''", organizationID, userID, projectID).Count(&count).Error
	return count > 0, err
}

func GetAgentSession(organizationID, userID, sessionID string) (model.AgentSession, error) {
	db, err := DB()
	if err != nil {
		return model.AgentSession{}, err
	}
	var session model.AgentSession
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, sessionID).First(&session).Error
	return session, err
}

func CreateAgentRun(message model.AgentMessage, run model.AgentRun, step model.AgentStep, event model.AgentEvent, snapshots ...model.AgentRunSnapshot) (model.AgentMessage, model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return message, run, err
	}
	if existingMessage, existingRun, found, existingErr := getExistingAgentRunSubmission(db, message, run, snapshots); existingErr != nil {
		return message, run, existingErr
	} else if found {
		return existingMessage, existingRun, nil
	}
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var session model.AgentSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", run.OrganizationID, run.UserID, run.SessionID, model.AgentSessionStatusActive).First(&session).Error; err != nil {
			return err
		}
		var sequence int64
		if err := tx.Model(&model.AgentMessage{}).Where("organization_id = ? AND user_id = ? AND session_id = ?", run.OrganizationID, run.UserID, run.SessionID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil {
			return err
		}
		message.Sequence = sequence + 1
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if len(snapshots) > 0 {
			snapshot := snapshots[0]
			if err := tx.Create(&snapshot).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&step).Error; err != nil {
			return err
		}
		event.Sequence = 1
		return tx.Create(&event).Error
	})
	if err != nil {
		if existingMessage, existingRun, found, existingErr := getExistingAgentRunSubmission(db, message, run, snapshots); found || existingErr != nil {
			return existingMessage, existingRun, existingErr
		}
	}
	return message, run, err
}

func getExistingAgentRunSubmission(db *gorm.DB, message model.AgentMessage, run model.AgentRun, snapshots []model.AgentRunSnapshot) (model.AgentMessage, model.AgentRun, bool, error) {
	var existing model.AgentRun
	if err := db.Where("organization_id = ? AND user_id = ? AND id = ?", run.OrganizationID, run.UserID, run.ID).First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return message, run, false, nil
	} else if err != nil {
		return message, run, false, err
	}
	if existing.SessionID != run.SessionID || existing.Model != run.Model || len(snapshots) == 0 && existing.Context != run.Context || existing.MaxToolCalls != run.MaxToolCalls || existing.MaxMediaCalls != run.MaxMediaCalls || existing.MaxDurationSec != run.MaxDurationSec || existing.MaxCredits != run.MaxCredits {
		return message, run, true, ErrAgentToolResultConflict
	}
	if len(snapshots) > 0 {
		var existingSnapshot model.AgentRunSnapshot
		if err := db.Where("organization_id = ? AND user_id = ? AND run_id = ?", run.OrganizationID, run.UserID, run.ID).First(&existingSnapshot).Error; err != nil {
			return message, run, true, err
		}
		if existingSnapshot.Checksum != snapshots[0].Checksum {
			return message, run, true, ErrAgentToolResultConflict
		}
	}
	var existingStep model.AgentStep
	if err := db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ?", run.OrganizationID, run.UserID, run.ID, model.AgentStepTypeCompletion).Order("created_at asc").First(&existingStep).Error; err != nil {
		return message, run, true, err
	}
	if existingStep.Input != message.Content {
		return message, run, true, ErrAgentToolResultConflict
	}
	var existingMessage model.AgentMessage
	if err := db.Where("organization_id = ? AND user_id = ? AND session_id = ? AND id = ?", run.OrganizationID, run.UserID, run.SessionID, existing.MessageID).First(&existingMessage).Error; err != nil {
		return message, run, true, err
	}
	return existingMessage, existing, true, nil
}

func ListRecentAgentMessages(organizationID, userID, sessionID string, limit int) ([]model.AgentMessage, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 30
	}
	var messages []model.AgentMessage
	err = db.Where("organization_id = ? AND user_id = ? AND session_id = ?", organizationID, userID, sessionID).Order("sequence desc").Limit(limit).Find(&messages).Error
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, err
}

func SaveAgentMemory(memory model.AgentMemory) (model.AgentMemory, error) {
	db, err := DB()
	if err != nil {
		return memory, err
	}
	return saveAgentMemory(db, memory)
}

func saveAgentMemory(db *gorm.DB, memory model.AgentMemory) (model.AgentMemory, error) {
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "user_id"}, {Name: "project_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"kind", "content", "source_run_id", "source_message_id", "confidence", "status", "expires_at", "forgotten_at", "updated_at"}),
	}).Create(&memory).Error
	if err != nil {
		return memory, err
	}
	var saved model.AgentMemory
	err = db.Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", memory.OrganizationID, memory.UserID, memory.ProjectID, memory.Key).First(&saved).Error
	return saved, err
}
func ListActiveAgentMemories(organizationID, userID, projectID string, limit int) ([]model.AgentMemory, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > agentMemoryMaxLimit {
		limit = agentMemoryMaxLimit
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	var memories []model.AgentMemory
	err = db.Where(
		"organization_id = ? AND user_id = ? AND status = ? AND confidence >= ? AND (project_id = ? OR project_id = '') AND (expires_at = '' OR expires_at > ?)",
		organizationID, userID, model.AgentMemoryStatusActive, 0.35, projectID, now,
	).Order("confidence desc, updated_at desc").Limit(limit).Find(&memories).Error
	return memories, err
}

func ForgetAgentMemory(organizationID, userID, projectID, key, timestamp string) (model.AgentMemory, error) {
	db, err := DB()
	if err != nil {
		return model.AgentMemory{}, err
	}
	var memory model.AgentMemory
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", organizationID, userID, projectID, key).First(&memory).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if memory.Status != model.AgentMemoryStatusActive {
			return nil
		}
		return tx.Model(&memory).Updates(map[string]any{"status": model.AgentMemoryStatusForgotten, "forgotten_at": timestamp, "updated_at": timestamp}).Error
	})
	return memory, err
}

func CompleteAgentRun(organizationID, userID, runID, messageID, deltaEventID, completedEventID, content, deltaPayload, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, run.SessionID).First(&model.AgentSession{}).Error; err != nil {
			return err
		}
		var sequence int64
		if err := tx.Model(&model.AgentMessage{}).Where("organization_id = ? AND user_id = ? AND session_id = ?", organizationID, userID, run.SessionID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil {
			return err
		}
		message := model.AgentMessage{ID: messageID, OrganizationID: organizationID, UserID: userID, SessionID: run.SessionID, Role: model.AgentMessageRoleAssistant, Content: content, Sequence: sequence + 1, CreatedAt: timestamp}
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": content, "error": "", "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: deltaEventID, Type: model.AgentEventMessageDelta, Payload: deltaPayload, CreatedAt: timestamp}); err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: completedEventID, Type: model.AgentEventRunCompleted, Payload: "{}", CreatedAt: timestamp}); err != nil {
			return err
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusCompleted, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func WaitAgentRunForTool(organizationID, userID, runID, completionOutput string, toolStep model.AgentStep, timestamp string, events ...model.AgentEvent) error {
	return WaitAgentRunForTools(organizationID, userID, runID, completionOutput, []model.AgentStep{toolStep}, timestamp, events...)
}

// WaitAgentRunForTools 一次登记本轮并行下发的全部工具步骤，运行进入等待工具结果状态。
func WaitAgentRunForTools(organizationID, userID, runID, completionOutput string, toolSteps []model.AgentStep, timestamp string, events ...model.AgentEvent) error {
	if len(toolSteps) == 0 {
		return gorm.ErrRecordNotFound
	}
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil {
			return err
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": completionOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		for i := range toolSteps {
			if err := tx.Create(&toolSteps[i]).Error; err != nil {
				return err
			}
		}
		for _, event := range events {
			if err := appendAgentEvent(tx, run, event); err != nil {
				return err
			}
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusWaitingTool, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func WaitAgentRunForConfirmation(organizationID, userID, runID, completionOutput string, toolStep model.AgentStep, event model.AgentEvent, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil {
			return err
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": completionOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Create(&toolStep).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, event); err != nil {
			return err
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusWaitingConfirmation, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func ConfirmAgentTool(organizationID, userID, runID, callID, decision, rejectedOutput, timestamp string, approvedEvent, rejectedEvent model.AgentEvent, completionStep model.AgentStep) (model.AgentRun, model.AgentStep, bool, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, model.AgentStep{}, false, false, err
	}
	var run model.AgentRun
	var toolStep model.AgentStep
	approved, resume := false, false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusWaitingConfirmation {
			if toolStep.Confirmation == decision {
				return nil
			}
			return gorm.ErrRecordNotFound
		}
		if toolStep.Status != model.AgentStepStatusRunning {
			return gorm.ErrRecordNotFound
		}
		if decision == "approved" {
			result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Update("confirmation", decision)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			if err := appendAgentEvent(tx, run, approvedEvent); err != nil {
				return err
			}
			result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusWaitingTool, "updated_at": timestamp})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			toolStep.Confirmation = decision
			run.Status, run.UpdatedAt, approved = model.AgentRunStatusWaitingTool, timestamp, true
			return nil
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "confirmation": decision, "output": rejectedOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := appendAgentEvent(tx, run, rejectedEvent); err != nil {
			return err
		}
		if err := tx.Create(&completionStep).Error; err != nil {
			return err
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Confirmation, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = model.AgentStepStatusCompleted, decision, rejectedOutput, "", timestamp, timestamp
		resume = true
		return nil
	})
	return run, toolStep, approved, resume, err
}

func AnswerAgentAskUser(organizationID, userID, runID, callID, decision, output, timestamp string, completedEvent model.AgentEvent, completionStep model.AgentStep, contexts ...string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, false, err
	}
	var run model.AgentRun
	var toolStep model.AgentStep
	resume := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusWaitingConfirmation {
			if toolStep.Confirmation == decision && toolStep.Output == output {
				return nil
			}
			return ErrAgentToolResultConflict
		}
		if toolStep.Status != model.AgentStepStatusRunning {
			return gorm.ErrRecordNotFound
		}
		if len(contexts) > 0 && contexts[0] != "" {
			if err := tx.Model(&run).Update("context", contexts[0]).Error; err != nil {
				return err
			}
			run.Context = contexts[0]
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "confirmation": decision, "output": output, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := appendAgentEvent(tx, run, completedEvent); err != nil {
			return err
		}
		if err := tx.Create(&completionStep).Error; err != nil {
			return err
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Confirmation, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = model.AgentStepStatusCompleted, decision, output, "", timestamp, timestamp
		resume = true
		return nil
	})
	return run, resume, err
}

type AgentToolCommit struct {
	Memory  *model.AgentMemory
	Context string
}

func SubmitAgentToolResult(organizationID, userID, runID, callID, executionToken, output, toolError, timestamp string, succeeded, continueAfterFailure bool, completedEvent, failedEvent model.AgentEvent, completionStep model.AgentStep, commits ...AgentToolCommit) (model.AgentRun, model.AgentStep, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, model.AgentStep{}, false, err
	}
	var run model.AgentRun
	var toolStep model.AgentStep
	resume := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil {
			return err
		}
		if toolStep.Status != model.AgentStepStatusRunning {
			if toolStep.Output == output && toolStep.Error == toolError {
				return nil
			}
			return ErrAgentToolResultConflict
		}
		claimedAt, claimErr := time.Parse(time.RFC3339Nano, toolStep.ExecutionAt)
		submittedAt, submittedErr := time.Parse(time.RFC3339Nano, timestamp)
		if executionToken == "" || toolStep.ExecutionToken != executionToken || claimErr != nil || submittedErr != nil || submittedAt.Sub(claimedAt) >= 90*time.Second {
			return ErrAgentToolExecutionClaimed
		}
		if run.Status != model.AgentRunStatusWaitingTool {
			return gorm.ErrRecordNotFound
		}
		commit := AgentToolCommit{}
		if len(commits) > 0 {
			commit = commits[0]
		}
		if commit.Context != "" {
			if err := tx.Model(&run).Update("context", commit.Context).Error; err != nil {
				return err
			}
			run.Context = commit.Context
		}
		if commit.Memory != nil {
			if toolStep.Confirmation != "approved" || !succeeded {
				return ErrAgentToolResultConflict
			}
			memory := *commit.Memory
			if memory.OrganizationID != organizationID || memory.UserID != userID || (toolStep.ToolName != "agent.remember" && toolStep.ToolName != "agent.forget") {
				return ErrAgentToolResultConflict
			}
			if memory.Status == model.AgentMemoryStatusForgotten {
				if err := tx.Model(&model.AgentMemory{}).Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", organizationID, userID, memory.ProjectID, memory.Key).Updates(map[string]any{"status": memory.Status, "forgotten_at": timestamp, "updated_at": timestamp}).Error; err != nil {
					return err
				}
			} else if _, err := saveAgentMemory(tx, memory); err != nil {
				return err
			}
		}
		stepStatus := model.AgentStepStatusCompleted
		if !succeeded {
			stepStatus = model.AgentStepStatusFailed
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND execution_token = ?", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning, executionToken).Updates(map[string]any{"status": stepStatus, "output": output, "error": toolError, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := appendAgentEvent(tx, run, completedEvent); err != nil {
			return err
		}
		if !succeeded && !continueAfterFailure {
			if err := appendAgentEvent(tx, run, failedEvent); err != nil {
				return err
			}
			result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": toolError, "completed_at": timestamp, "updated_at": timestamp})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return gorm.ErrRecordNotFound
			}
			run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusFailed, toolError, timestamp, timestamp
			toolStep.Status, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = stepStatus, output, toolError, timestamp, timestamp
			return nil
		}
		// 并行下发的工具全部返回后才继续规划；其余步骤仍在执行时保持等待状态。
		var pendingTools int64
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeTool, model.AgentStepStatusRunning).Count(&pendingTools).Error; err != nil {
			return err
		}
		if pendingTools > 0 {
			if err := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool).Update("updated_at", timestamp).Error; err != nil {
				return err
			}
			run.UpdatedAt = timestamp
			toolStep.Status, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = stepStatus, output, toolError, timestamp, timestamp
			return nil
		}
		if err := tx.Create(&completionStep).Error; err != nil {
			return err
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = stepStatus, output, toolError, timestamp, timestamp
		resume = true
		return nil
	})
	return run, toolStep, resume, err
}

func ExpireRunningAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp, deadline string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, false, err
	}
	var run model.AgentRun
	expired := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusRunning || run.UpdatedAt > deadline {
			return nil
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND updated_at <= ?", organizationID, userID, runID, model.AgentRunStatusRunning, deadline).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunFailed, Payload: payload, CreatedAt: timestamp}); err != nil {
			return err
		}
		run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusFailed, message, timestamp, timestamp
		expired = true
		return nil
	})
	return run, expired, err
}

func ExpireWaitingAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp, deadline string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, false, err
	}
	var run model.AgentRun
	expired := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusWaitingTool || run.UpdatedAt > deadline {
			return nil
		}
		var activeTools int64
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ? AND execution_at > ?", organizationID, userID, runID, model.AgentStepTypeTool, model.AgentStepStatusRunning, deadline).Count(&activeTools).Error; err != nil {
			return err
		}
		if activeTools > 0 {
			return nil
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND updated_at <= ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool, deadline).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeTool, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunFailed, Payload: payload, CreatedAt: timestamp}); err != nil {
			return err
		}
		run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusFailed, message, timestamp, timestamp
		expired = true
		return nil
	})
	return run, expired, err
}

func FailAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp string) error {
	return finishAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp, model.AgentRunStatusFailed, model.AgentStepStatusFailed, model.AgentEventRunFailed)
}

func CancelAgentRun(organizationID, userID, runID, eventID, timestamp string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, false, err
	}
	var run model.AgentRun
	cancelled := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusRunning && run.Status != model.AgentRunStatusWaitingConfirmation && run.Status != model.AgentRunStatusWaitingTool {
			return nil
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status IN ?", organizationID, userID, runID, []model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingConfirmation, model.AgentRunStatusWaitingTool}).Updates(map[string]any{"status": model.AgentRunStatusCancelled, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCancelled, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunCancelled, Payload: "{}", CreatedAt: timestamp}); err != nil {
			return err
		}
		run.Status, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusCancelled, timestamp, timestamp
		cancelled = true
		return nil
	})
	return run, cancelled, err
}

func RevertAgentTool(organizationID, userID, runID, callID, timestamp string, revertedEvent, cancelledEvent model.AgentEvent) (model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, err
	}
	var run model.AgentRun
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		var toolStep model.AgentStep
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil {
			return err
		}
		if toolStep.Confirmation == "rejected" || (toolStep.Status != model.AgentStepStatusRunning && toolStep.Status != model.AgentStepStatusCompleted) {
			return ErrAgentToolNotRevertible
		}
		var eventCount int64
		if err := tx.Model(&model.AgentEvent{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND payload = ?", organizationID, userID, runID, model.AgentEventToolReverted, revertedEvent.Payload).Count(&eventCount).Error; err != nil {
			return err
		}
		if eventCount > 0 {
			return nil
		}
		if err := appendAgentEvent(tx, run, revertedEvent); err != nil {
			return err
		}
		if run.Terminal() {
			return nil
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status IN ?", organizationID, userID, runID, []model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingConfirmation, model.AgentRunStatusWaitingTool}).Updates(map[string]any{"status": model.AgentRunStatusCancelled, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCancelled, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, cancelledEvent); err != nil {
			return err
		}
		run.Status, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusCancelled, timestamp, timestamp
		return nil
	})
	return run, err
}

func RevertAgentRun(organizationID, userID, runID string, callIDs []string, timestamp string, revertedEvents []model.AgentEvent, cancelledEvent model.AgentEvent) (model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, err
	}
	var run model.AgentRun
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND tool_call_id IN ?", organizationID, userID, runID, model.AgentStepTypeTool, callIDs).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(callIDs)) || len(revertedEvents) != len(callIDs) {
			return ErrAgentToolNotRevertible
		}
		payloads := make([]string, len(revertedEvents))
		for i, event := range revertedEvents {
			payloads[i] = event.Payload
		}
		var existingPayloads []string
		if len(payloads) > 0 {
			if err := tx.Model(&model.AgentEvent{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND payload IN ?", organizationID, userID, runID, model.AgentEventToolReverted, payloads).Distinct().Pluck("payload", &existingPayloads).Error; err != nil {
				return err
			}
		}
		existing := make(map[string]bool, len(existingPayloads))
		for _, payload := range existingPayloads {
			existing[payload] = true
		}
		for _, event := range revertedEvents {
			if existing[event.Payload] {
				continue
			}
			if err := appendAgentEvent(tx, run, event); err != nil {
				return err
			}
			existing[event.Payload] = true
		}
		if run.Terminal() {
			return nil
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status IN ?", organizationID, userID, runID, []model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingConfirmation, model.AgentRunStatusWaitingTool}).Updates(map[string]any{"status": model.AgentRunStatusCancelled, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCancelled, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, cancelledEvent); err != nil {
			return err
		}
		run.Status, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusCancelled, timestamp, timestamp
		return nil
	})
	return run, err
}

func GetAgentRun(organizationID, userID, runID string) (model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, err
	}
	var run model.AgentRun
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error
	return run, err
}

func GetAgentToolStep(organizationID, userID, runID, callID string) (model.AgentStep, error) {
	db, err := DB()
	if err != nil {
		return model.AgentStep{}, err
	}
	var step model.AgentStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&step).Error
	return step, err
}

func ListAgentSteps(organizationID, userID, runID string) ([]model.AgentStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var steps []model.AgentStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ?", organizationID, userID, runID).Order("created_at asc").Find(&steps).Error
	return steps, err
}

func RetryAgentStep(organizationID, userID, runID, sourceCallID, timestamp string, retryStep model.AgentStep, retryEvent model.AgentEvent) (model.AgentRun, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRun{}, err
	}
	var run model.AgentRun
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil {
			return err
		}
		if run.Status != model.AgentRunStatusFailed {
			return ErrAgentStepNotRetryable
		}
		var source model.AgentStep
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ? AND status = ?", organizationID, userID, runID, sourceCallID, model.AgentStepTypeTool, model.AgentStepStatusFailed).First(&source).Error; err != nil {
			return ErrAgentStepNotRetryable
		}
		if source.ToolName != retryStep.ToolName || source.Input != retryStep.Input {
			return ErrAgentStepNotRetryable
		}
		if err := tx.Create(&retryStep).Error; err != nil {
			return err
		}
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusFailed).Updates(map[string]any{"status": model.AgentRunStatusWaitingTool, "error": "", "completed_at": "", "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAgentStepNotRetryable
		}
		if err := appendAgentEvent(tx, run, retryEvent); err != nil {
			return err
		}
		run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusWaitingTool, "", "", timestamp
		return nil
	})
	return run, err
}

func ListCompletedAgentToolSteps(organizationID, userID, runID string) ([]model.AgentStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var steps []model.AgentStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status IN ?", organizationID, userID, runID, model.AgentStepTypeTool, []model.AgentStepStatus{model.AgentStepStatusCompleted, model.AgentStepStatusFailed}).Order("created_at asc").Find(&steps).Error
	return steps, err
}

func ClaimAgentRunExecution(organizationID, userID, runID, token, timestamp, deadline string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.AgentStep{}).Where(
		"organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ? AND (execution_token = '' OR execution_at <= ?)",
		organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning, deadline,
	).Updates(map[string]any{"execution_token": token, "execution_at": timestamp, "updated_at": timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrAgentToolExecutionClaimed
	}
	return nil
}

func ClaimAgentToolExecution(organizationID, userID, runID, callID, token, timestamp, deadline string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var step model.AgentStep
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ? AND status = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool, model.AgentStepStatusRunning).First(&step).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAgentToolExecutionClaimed
			}
			return err
		}
		if step.ExecutionToken != "" && step.ExecutionToken != token && step.ExecutionAt > deadline {
			return ErrAgentToolExecutionClaimed
		}
		takeover := step.ExecutionToken != "" && step.ExecutionToken != token
		if err := tx.Model(&step).Updates(map[string]any{"execution_token": token, "execution_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if takeover {
			return tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).UpdateColumn("tool_lease_takeovers", gorm.Expr("tool_lease_takeovers + 1")).Error
		}
		return nil
	})
}

func GetAgentToolOutput(organizationID, userID, runID, callID string) (string, error) {
	db, err := DB()
	if err != nil {
		return "", err
	}
	var step model.AgentStep
	err = db.Select("output").Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ? AND status = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool, model.AgentStepStatusCompleted).First(&step).Error
	return step.Output, err
}

func ListAgentEvents(organizationID, userID, runID string, after int64, limit int) ([]model.AgentEvent, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var events []model.AgentEvent
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND sequence > ?", organizationID, userID, runID, after).Order("sequence asc").Limit(limit).Find(&events).Error
	return events, err
}

func finishAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp string, runStatus model.AgentRunStatus, stepStatus model.AgentStepStatus, eventType model.AgentEventType) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": stepStatus, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: eventType, Payload: payload, CreatedAt: timestamp}); err != nil {
			return err
		}
		return tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": runStatus, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error
	})
}

func lockRunningAgentRun(tx *gorm.DB, organizationID, userID, runID string) (model.AgentRun, error) {
	var run model.AgentRun
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).First(&run).Error
	return run, err
}

func appendAgentEvent(tx *gorm.DB, run model.AgentRun, event model.AgentEvent) error {
	var sequence int64
	if err := tx.Model(&model.AgentEvent{}).Where("organization_id = ? AND user_id = ? AND run_id = ?", run.OrganizationID, run.UserID, run.ID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil {
		return err
	}
	event.OrganizationID, event.UserID, event.RunID, event.Sequence = run.OrganizationID, run.UserID, run.ID, sequence+1
	if err := tx.Create(&event).Error; err != nil {
		return err
	}
	notifyAgentEventAfterCommit(tx, run.ID)
	return nil
}
