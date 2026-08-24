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
	ErrAgentToolResultConflict = errors.New("agent tool result conflicts with saved result")
	ErrAgentToolExecutionClaimed = errors.New("agent tool execution already claimed")
	ErrAgentToolNotRevertible = errors.New("agent tool cannot be reverted")
)

func CreateAgentSession(session model.AgentSession) (model.AgentSession, error) {
	db, err := DB()
	if err != nil { return session, err }
	var existing model.AgentSession
	if err := db.Where("organization_id = ? AND user_id = ? AND id = ?", session.OrganizationID, session.UserID, session.ID).First(&existing).Error; err == nil {
		if existing.ProjectID != session.ProjectID || existing.Profile != session.Profile || existing.Title != session.Title { return session, ErrAgentToolResultConflict }
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) { return session, err }
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error { return tx.Create(&session).Error })
	if err != nil {
		if lookupErr := db.Where("organization_id = ? AND user_id = ? AND id = ?", session.OrganizationID, session.UserID, session.ID).First(&existing).Error; lookupErr == nil {
			if existing.ProjectID != session.ProjectID || existing.Profile != session.Profile || existing.Title != session.Title { return session, ErrAgentToolResultConflict }
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
	if err != nil { return model.AgentSession{}, err }
	var session model.AgentSession
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, sessionID).First(&session).Error
	return session, err
}

func CreateAgentRun(message model.AgentMessage, run model.AgentRun, step model.AgentStep, event model.AgentEvent) (model.AgentMessage, model.AgentRun, error) {
	db, err := DB()
	if err != nil { return message, run, err }
	if existingMessage, existingRun, found, existingErr := getExistingAgentRunSubmission(db, message, run); existingErr != nil {
		return message, run, existingErr
	} else if found {
		return existingMessage, existingRun, nil
	}
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var session model.AgentSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", run.OrganizationID, run.UserID, run.SessionID, model.AgentSessionStatusActive).First(&session).Error; err != nil { return err }
		var sequence int64
		if err := tx.Model(&model.AgentMessage{}).Where("organization_id = ? AND user_id = ? AND session_id = ?", run.OrganizationID, run.UserID, run.SessionID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil { return err }
		message.Sequence = sequence + 1
		if err := tx.Create(&message).Error; err != nil { return err }
		if err := tx.Create(&run).Error; err != nil { return err }
		if err := tx.Create(&step).Error; err != nil { return err }
		event.Sequence = 1
		return tx.Create(&event).Error
	})
	if err != nil {
		if existingMessage, existingRun, found, existingErr := getExistingAgentRunSubmission(db, message, run); found || existingErr != nil {
			return existingMessage, existingRun, existingErr
		}
	}
	return message, run, err
}

func getExistingAgentRunSubmission(db *gorm.DB, message model.AgentMessage, run model.AgentRun) (model.AgentMessage, model.AgentRun, bool, error) {
	var existing model.AgentRun
	if err := db.Where("organization_id = ? AND user_id = ? AND id = ?", run.OrganizationID, run.UserID, run.ID).First(&existing).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return message, run, false, nil
	} else if err != nil {
		return message, run, false, err
	}
	if existing.SessionID != run.SessionID || existing.Model != run.Model || existing.Context != run.Context { return message, run, true, ErrAgentToolResultConflict }
	var existingStep model.AgentStep
	if err := db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ?", run.OrganizationID, run.UserID, run.ID, model.AgentStepTypeCompletion).Order("created_at asc").First(&existingStep).Error; err != nil { return message, run, true, err }
	if existingStep.Input != message.Content { return message, run, true, ErrAgentToolResultConflict }
	var existingMessage model.AgentMessage
	if err := db.Where("organization_id = ? AND user_id = ? AND session_id = ? AND id = ?", run.OrganizationID, run.UserID, run.SessionID, existing.MessageID).First(&existingMessage).Error; err != nil { return message, run, true, err }
	return existingMessage, existing, true, nil
}

func ListRecentAgentMessages(organizationID, userID, sessionID string, limit int) ([]model.AgentMessage, error) {
	db, err := DB()
	if err != nil { return nil, err }
	if limit <= 0 { limit = 30 }
	var messages []model.AgentMessage
	err = db.Where("organization_id = ? AND user_id = ? AND session_id = ?", organizationID, userID, sessionID).Order("sequence desc").Limit(limit).Find(&messages).Error
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 { messages[left], messages[right] = messages[right], messages[left] }
	return messages, err
}

func SaveAgentMemory(memory model.AgentMemory) (model.AgentMemory, error) {
	db, err := DB()
	if err != nil { return memory, err }
	var existing model.AgentMemory
	lookup := db.Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", memory.OrganizationID, memory.UserID, memory.ProjectID, memory.Key).First(&existing)
	if lookup.Error == nil {
		memory.ID = existing.ID
		if memory.CreatedAt == "" { memory.CreatedAt = existing.CreatedAt }
		if err := db.Model(&existing).Updates(map[string]any{
			"kind": memory.Kind, "content": memory.Content, "source_run_id": memory.SourceRunID,
			"source_message_id": memory.SourceMessageID, "confidence": memory.Confidence,
			"status": memory.Status, "expires_at": memory.ExpiresAt, "forgotten_at": memory.ForgottenAt,
			"updated_at": memory.UpdatedAt,
		}).Error; err != nil { return memory, err }
		return memory, nil
	}
	if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) { return memory, lookup.Error }
	if err := transactionWithSQLiteRetry(db, func(tx *gorm.DB) error { return tx.Create(&memory).Error }); err != nil {
		if retryErr := db.Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", memory.OrganizationID, memory.UserID, memory.ProjectID, memory.Key).First(&existing).Error; retryErr == nil {
			memory.ID = existing.ID
			return memory, db.Model(&existing).Updates(map[string]any{
				"kind": memory.Kind, "content": memory.Content, "source_run_id": memory.SourceRunID,
				"source_message_id": memory.SourceMessageID, "confidence": memory.Confidence,
				"status": memory.Status, "expires_at": memory.ExpiresAt, "forgotten_at": memory.ForgottenAt,
				"updated_at": memory.UpdatedAt,
			}).Error
		}
		return memory, err
	}
	return memory, nil
}

func ListActiveAgentMemories(organizationID, userID, projectID string, limit int) ([]model.AgentMemory, error) {
	db, err := DB()
	if err != nil { return nil, err }
	if limit <= 0 || limit > agentMemoryMaxLimit { limit = agentMemoryMaxLimit }
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000000000Z")
	var memories []model.AgentMemory
	err = db.Where(
		"organization_id = ? AND user_id = ? AND status = ? AND (project_id = ? OR project_id = '') AND (expires_at = '' OR expires_at > ?)",
		organizationID, userID, model.AgentMemoryStatusActive, projectID, now,
	).Order("confidence desc, updated_at desc").Limit(limit).Find(&memories).Error
	return memories, err
}

func ForgetAgentMemory(organizationID, userID, projectID, key, timestamp string) (model.AgentMemory, error) {
	db, err := DB()
	if err != nil { return model.AgentMemory{}, err }
	var memory model.AgentMemory
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Where("organization_id = ? AND user_id = ? AND project_id = ? AND key = ?", organizationID, userID, projectID, key).First(&memory).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
			return err
		}
		if memory.Status != model.AgentMemoryStatusActive { return nil }
		return tx.Model(&memory).Updates(map[string]any{"status": model.AgentMemoryStatusForgotten, "forgotten_at": timestamp, "updated_at": timestamp}).Error
	})
	return memory, err
}

func CompleteAgentRun(organizationID, userID, runID, messageID, deltaEventID, completedEventID, content, deltaPayload, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, run.SessionID).First(&model.AgentSession{}).Error; err != nil { return err }
		var sequence int64
		if err := tx.Model(&model.AgentMessage{}).Where("organization_id = ? AND user_id = ? AND session_id = ?", organizationID, userID, run.SessionID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil { return err }
		message := model.AgentMessage{ID: messageID, OrganizationID: organizationID, UserID: userID, SessionID: run.SessionID, Role: model.AgentMessageRoleAssistant, Content: content, Sequence: sequence + 1, CreatedAt: timestamp}
		if err := tx.Create(&message).Error; err != nil { return err }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": content, "error": "", "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: deltaEventID, Type: model.AgentEventMessageDelta, Payload: deltaPayload, CreatedAt: timestamp}); err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: completedEventID, Type: model.AgentEventRunCompleted, Payload: "{}", CreatedAt: timestamp}); err != nil { return err }
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusCompleted, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		return nil
	})
}

func WaitAgentRunForTool(organizationID, userID, runID, completionOutput string, toolStep model.AgentStep, timestamp string, events ...model.AgentEvent) error {
	db, err := DB()
	if err != nil { return err }
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil { return err }
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": completionOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := tx.Create(&toolStep).Error; err != nil { return err }
		for _, event := range events {
			if err := appendAgentEvent(tx, run, event); err != nil { return err }
		}
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusWaitingTool, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		return nil
	})
}

func WaitAgentRunForConfirmation(organizationID, userID, runID, completionOutput string, toolStep model.AgentStep, event model.AgentEvent, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if err != nil { return err }
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "output": completionOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := tx.Create(&toolStep).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, event); err != nil { return err }
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusRunning).Updates(map[string]any{"status": model.AgentRunStatusWaitingConfirmation, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		return nil
	})
}

func ConfirmAgentTool(organizationID, userID, runID, callID, decision, rejectedOutput, timestamp string, approvedEvent, rejectedEvent model.AgentEvent, completionStep model.AgentStep) (model.AgentRun, model.AgentStep, bool, bool, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, model.AgentStep{}, false, false, err }
	var run model.AgentRun
	var toolStep model.AgentStep
	approved, resume := false, false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil { return err }
		if run.Status != model.AgentRunStatusWaitingConfirmation {
			if toolStep.Confirmation == decision { return nil }
			return gorm.ErrRecordNotFound
		}
		if toolStep.Status != model.AgentStepStatusRunning { return gorm.ErrRecordNotFound }
		if decision == "approved" {
			result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Update("confirmation", decision)
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
			if err := appendAgentEvent(tx, run, approvedEvent); err != nil { return err }
			result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusWaitingTool, "updated_at": timestamp})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
			toolStep.Confirmation = decision
			run.Status, run.UpdatedAt, approved = model.AgentRunStatusWaitingTool, timestamp, true
			return nil
		}
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "confirmation": decision, "output": rejectedOutput, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := appendAgentEvent(tx, run, rejectedEvent); err != nil { return err }
		if err := tx.Create(&completionStep).Error; err != nil { return err }
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Confirmation, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = model.AgentStepStatusCompleted, decision, rejectedOutput, "", timestamp, timestamp
		resume = true
		return nil
	})
	return run, toolStep, approved, resume, err
}

func AnswerAgentAskUser(organizationID, userID, runID, callID, decision, output, timestamp string, completedEvent model.AgentEvent, completionStep model.AgentStep) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, false, err }
	var run model.AgentRun
	var toolStep model.AgentStep
	resume := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil { return err }
		if run.Status != model.AgentRunStatusWaitingConfirmation {
			if toolStep.Confirmation == decision && toolStep.Output == output { return nil }
			return ErrAgentToolResultConflict
		}
		if toolStep.Status != model.AgentStepStatusRunning { return gorm.ErrRecordNotFound }
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND confirmation = ''", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCompleted, "confirmation": decision, "output": output, "error": "", "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := appendAgentEvent(tx, run, completedEvent); err != nil { return err }
		if err := tx.Create(&completionStep).Error; err != nil { return err }
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingConfirmation).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Confirmation, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = model.AgentStepStatusCompleted, decision, output, "", timestamp, timestamp
		resume = true
		return nil
	})
	return run, resume, err
}

func SubmitAgentToolResult(organizationID, userID, runID, callID, executionToken, output, toolError, timestamp string, succeeded, continueAfterFailure bool, completedEvent, failedEvent model.AgentEvent, completionStep model.AgentStep) (model.AgentRun, model.AgentStep, bool, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, model.AgentStep{}, false, err }
	var run model.AgentRun
	var toolStep model.AgentStep
	resume := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil { return err }
		if toolStep.Status != model.AgentStepStatusRunning {
			if toolStep.Output == output && toolStep.Error == toolError { return nil }
			return ErrAgentToolResultConflict
		}
		if toolStep.ExecutionToken != executionToken { return ErrAgentToolExecutionClaimed }
		if run.Status != model.AgentRunStatusWaitingTool { return gorm.ErrRecordNotFound }
		stepStatus := model.AgentStepStatusCompleted
		if !succeeded { stepStatus = model.AgentStepStatusFailed }
		result := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND execution_token = ?", organizationID, userID, toolStep.ID, model.AgentStepStatusRunning, executionToken).Updates(map[string]any{"status": stepStatus, "output": output, "error": toolError, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := appendAgentEvent(tx, run, completedEvent); err != nil { return err }
		if !succeeded && !continueAfterFailure {
			if err := appendAgentEvent(tx, run, failedEvent); err != nil { return err }
			result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": toolError, "completed_at": timestamp, "updated_at": timestamp})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
			run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusFailed, toolError, timestamp, timestamp
			toolStep.Status, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = stepStatus, output, toolError, timestamp, timestamp
			return nil
		}
		if err := tx.Create(&completionStep).Error; err != nil { return err }
		result = tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool).Updates(map[string]any{"status": model.AgentRunStatusRunning, "error": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		run.Status, run.Error, run.UpdatedAt = model.AgentRunStatusRunning, "", timestamp
		toolStep.Status, toolStep.Output, toolStep.Error, toolStep.CompletedAt, toolStep.UpdatedAt = stepStatus, output, toolError, timestamp, timestamp
		resume = true
		return nil
	})
	return run, toolStep, resume, err
}

func ExpireRunningAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp, deadline string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, false, err }
	var run model.AgentRun
	expired := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if run.Status != model.AgentRunStatusRunning || run.UpdatedAt > deadline { return nil }
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND updated_at <= ?", organizationID, userID, runID, model.AgentRunStatusRunning, deadline).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return nil }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunFailed, Payload: payload, CreatedAt: timestamp}); err != nil { return err }
		run.Status, run.Error, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusFailed, message, timestamp, timestamp
		expired = true
		return nil
	})
	return run, expired, err
}

func ExpireWaitingAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp, deadline string) (model.AgentRun, bool, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, false, err }
	var run model.AgentRun
	expired := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if run.Status != model.AgentRunStatusWaitingTool || run.UpdatedAt > deadline { return nil }
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status = ? AND updated_at <= ?", organizationID, userID, runID, model.AgentRunStatusWaitingTool, deadline).Updates(map[string]any{"status": model.AgentRunStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return nil }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ?", organizationID, userID, runID, model.AgentStepTypeTool, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusFailed, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunFailed, Payload: payload, CreatedAt: timestamp}); err != nil { return err }
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
	if err != nil { return model.AgentRun{}, false, err }
	var run model.AgentRun
	cancelled := false
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		if run.Status != model.AgentRunStatusRunning && run.Status != model.AgentRunStatusWaitingConfirmation && run.Status != model.AgentRunStatusWaitingTool { return nil }
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status IN ?", organizationID, userID, runID, []model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingConfirmation, model.AgentRunStatusWaitingTool}).Updates(map[string]any{"status": model.AgentRunStatusCancelled, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return nil }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCancelled, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: model.AgentEventRunCancelled, Payload: "{}", CreatedAt: timestamp}); err != nil { return err }
		run.Status, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusCancelled, timestamp, timestamp
		cancelled = true
		return nil
	})
	return run, cancelled, err
}

func RevertAgentTool(organizationID, userID, runID, callID, timestamp string, revertedEvent, cancelledEvent model.AgentEvent) (model.AgentRun, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, err }
	var run model.AgentRun
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error; err != nil { return err }
		var toolStep model.AgentStep
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&toolStep).Error; err != nil { return err }
		if toolStep.Confirmation == "rejected" || (toolStep.Status != model.AgentStepStatusRunning && toolStep.Status != model.AgentStepStatusCompleted) { return ErrAgentToolNotRevertible }
		var eventCount int64
		if err := tx.Model(&model.AgentEvent{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND payload = ?", organizationID, userID, runID, model.AgentEventToolReverted, revertedEvent.Payload).Count(&eventCount).Error; err != nil { return err }
		if eventCount > 0 { return nil }
		if err := appendAgentEvent(tx, run, revertedEvent); err != nil { return err }
		if run.Terminal() { return nil }
		result := tx.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND status IN ?", organizationID, userID, runID, []model.AgentRunStatus{model.AgentRunStatusRunning, model.AgentRunStatusWaitingConfirmation, model.AgentRunStatusWaitingTool}).Updates(map[string]any{"status": model.AgentRunStatusCancelled, "completed_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": model.AgentStepStatusCancelled, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, cancelledEvent); err != nil { return err }
		run.Status, run.CompletedAt, run.UpdatedAt = model.AgentRunStatusCancelled, timestamp, timestamp
		return nil
	})
	return run, err
}

func GetAgentRun(organizationID, userID, runID string) (model.AgentRun, error) {
	db, err := DB()
	if err != nil { return model.AgentRun{}, err }
	var run model.AgentRun
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).First(&run).Error
	return run, err
}

func GetAgentToolStep(organizationID, userID, runID, callID string) (model.AgentStep, error) {
	db, err := DB()
	if err != nil { return model.AgentStep{}, err }
	var step model.AgentStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool).First(&step).Error
	return step, err
}

func ListCompletedAgentToolSteps(organizationID, userID, runID string) ([]model.AgentStep, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var steps []model.AgentStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status IN ?", organizationID, userID, runID, model.AgentStepTypeTool, []model.AgentStepStatus{model.AgentStepStatusCompleted, model.AgentStepStatusFailed}).Order("created_at asc").Find(&steps).Error
	return steps, err
}

func ClaimAgentRunExecution(organizationID, userID, runID, token, timestamp, deadline string) error {
	db, err := DB()
	if err != nil { return err }
	result := db.Model(&model.AgentStep{}).Where(
		"organization_id = ? AND user_id = ? AND run_id = ? AND type = ? AND status = ? AND (execution_token = '' OR execution_at <= ?)",
		organizationID, userID, runID, model.AgentStepTypeCompletion, model.AgentStepStatusRunning, deadline,
	).Updates(map[string]any{"execution_token": token, "execution_at": timestamp, "updated_at": timestamp})
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return ErrAgentToolExecutionClaimed }
	return nil
}

func ClaimAgentToolExecution(organizationID, userID, runID, callID, token, timestamp, deadline string) error {
	db, err := DB()
	if err != nil { return err }
	result := db.Model(&model.AgentStep{}).Where(
		"organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ? AND status = ? AND (execution_token = '' OR execution_token = ? OR execution_at <= ?)",
		organizationID, userID, runID, callID, model.AgentStepTypeTool, model.AgentStepStatusRunning, token, deadline,
	).Updates(map[string]any{"execution_token": token, "execution_at": timestamp, "updated_at": timestamp})
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return ErrAgentToolExecutionClaimed }
	return nil
}

func GetAgentToolOutput(organizationID, userID, runID, callID string) (string, error) {
	db, err := DB()
	if err != nil { return "", err }
	var step model.AgentStep
	err = db.Select("output").Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND type = ? AND status = ?", organizationID, userID, runID, callID, model.AgentStepTypeTool, model.AgentStepStatusCompleted).First(&step).Error
	return step.Output, err
}

func ListAgentEvents(organizationID, userID, runID string, after int64, limit int) ([]model.AgentEvent, error) {
	db, err := DB()
	if err != nil { return nil, err }
	if limit <= 0 || limit > 100 { limit = 100 }
	var events []model.AgentEvent
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ? AND sequence > ?", organizationID, userID, runID, after).Order("sequence asc").Limit(limit).Find(&events).Error
	return events, err
}

func finishAgentRun(organizationID, userID, runID, eventID, message, payload, timestamp string, runStatus model.AgentRunStatus, stepStatus model.AgentStepStatus, eventType model.AgentEventType) error {
	db, err := DB()
	if err != nil { return err }
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		run, err := lockRunningAgentRun(tx, organizationID, userID, runID)
		if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
		if err != nil { return err }
		if err := tx.Model(&model.AgentStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status = ?", organizationID, userID, runID, model.AgentStepStatusRunning).Updates(map[string]any{"status": stepStatus, "error": message, "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		if err := appendAgentEvent(tx, run, model.AgentEvent{ID: eventID, Type: eventType, Payload: payload, CreatedAt: timestamp}); err != nil { return err }
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
	if err := tx.Model(&model.AgentEvent{}).Where("organization_id = ? AND user_id = ? AND run_id = ?", run.OrganizationID, run.UserID, run.ID).Select("COALESCE(MAX(sequence), 0)").Scan(&sequence).Error; err != nil { return err }
	event.OrganizationID, event.UserID, event.RunID, event.Sequence = run.OrganizationID, run.UserID, run.ID, sequence+1
	return tx.Create(&event).Error
}
