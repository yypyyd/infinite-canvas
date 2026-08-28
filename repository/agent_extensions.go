package repository

import (
	"errors"
	"math"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AgentMetricsData struct {
	Runs     []model.AgentRun
	Steps    []model.AgentStep
	Feedback []model.AgentFeedback
	Credits  int64
}

func GetAgentRunSnapshot(organizationID, userID, runID string) (model.AgentRunSnapshot, error) {
	db, err := DB()
	if err != nil {
		return model.AgentRunSnapshot{}, err
	}
	var snapshot model.AgentRunSnapshot
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ?", organizationID, userID, runID).First(&snapshot).Error
	return snapshot, err
}

func MarkAgentRunSnapshotRestored(organizationID, userID, runID, checksum, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.AgentRunSnapshot{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND checksum = ?", organizationID, userID, runID, checksum).Update("restored_at", timestamp)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func MarkAgentRunBudgetReason(organizationID, userID, runID, reason, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ? AND budget_reason = ''", organizationID, userID, runID).Updates(map[string]any{"budget_reason": reason, "updated_at": timestamp}).Error
}

func RecordAgentStreamReconnect(organizationID, userID, runID string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.AgentRun{}).Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, runID).UpdateColumn("stream_reconnects", gorm.Expr("stream_reconnects + 1")).Error
}

func GetAgentRunCredits(organizationID, userID, runID string) (int, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var credits int
	err = db.Model(&model.GenerationTask{}).Where(
		"organization_id = ? AND user_id = ? AND status != ? AND (request_id LIKE ? OR request_id LIKE ? OR request_id LIKE ?)",
		organizationID, userID, model.GenerationTaskStatusFailed, runID+"-completion-%", "agent:"+runID+":%", "agent-inspect:"+runID+":%",
	).Select("COALESCE(SUM(credits), 0)").Scan(&credits).Error
	return credits, err
}

func ReplaceAgentPlan(organizationID, userID, runID, planCallID string, titles []string, timestamp string) ([]model.AgentPlanStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	steps := make([]model.AgentPlanStep, 0, len(titles))
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		steps = steps[:0]
		if err := tx.Where("organization_id = ? AND user_id = ? AND run_id = ? AND plan_call_id = ?", organizationID, userID, runID, planCallID).Order("position asc").Find(&steps).Error; err != nil {
			return err
		}
		if len(steps) > 0 {
			return nil
		}
		var revision int
		if err := tx.Model(&model.AgentPlanStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ?", organizationID, userID, runID).Select("COALESCE(MAX(revision), 0)").Scan(&revision).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.AgentPlanStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status IN ?", organizationID, userID, runID, []model.AgentPlanStepStatus{model.AgentPlanStepStatusPending, model.AgentPlanStepStatusRunning}).Updates(map[string]any{"status": model.AgentPlanStepStatusSkipped, "reason": "计划已重新调整", "completed_at": timestamp, "updated_at": timestamp}).Error; err != nil {
			return err
		}
		revision++
		for index, title := range titles {
			dependsOn := 0
			if index > 0 {
				dependsOn = index
			}
			steps = append(steps, model.AgentPlanStep{
				ID: newRepositoryID("agent-plan-step"), OrganizationID: organizationID, UserID: userID, RunID: runID,
				Revision: revision, PlanCallID: planCallID, Position: index + 1, Title: title, CompletionCriteria: "关联工具返回成功", DependsOnPosition: dependsOn,
				Status: model.AgentPlanStepStatusPending, CreatedAt: timestamp, UpdatedAt: timestamp,
			})
		}
		return tx.Create(&steps).Error
	})
	return steps, err
}

func StartNextAgentPlanStep(organizationID, userID, runID, callID, toolName, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var revision int
		if err := tx.Model(&model.AgentPlanStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ?", organizationID, userID, runID).Select("COALESCE(MAX(revision), 0)").Scan(&revision).Error; err != nil || revision == 0 {
			return err
		}
		var step model.AgentPlanStep
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND revision = ? AND status = ?", organizationID, userID, runID, revision, model.AgentPlanStepStatusPending).Order("position asc").First(&step).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return tx.Model(&step).Updates(map[string]any{"status": model.AgentPlanStepStatusRunning, "tool_call_id": callID, "tool_name": toolName, "started_at": timestamp, "updated_at": timestamp}).Error
	})
}

func FinishAgentPlanStep(organizationID, userID, runID, callID string, succeeded bool, reason, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	status := model.AgentPlanStepStatusCompleted
	if !succeeded {
		status = model.AgentPlanStepStatusFailed
	}
	return db.Model(&model.AgentPlanStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND tool_call_id = ? AND status = ?", organizationID, userID, runID, callID, model.AgentPlanStepStatusRunning).Updates(map[string]any{"status": status, "reason": reason, "completed_at": timestamp, "updated_at": timestamp}).Error
}

func FinalizeAgentPlan(organizationID, userID, runID, reason, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.AgentPlanStep{}).Where("organization_id = ? AND user_id = ? AND run_id = ? AND status IN ?", organizationID, userID, runID, []model.AgentPlanStepStatus{model.AgentPlanStepStatusPending, model.AgentPlanStepStatusRunning}).Updates(map[string]any{"status": model.AgentPlanStepStatusSkipped, "reason": reason, "completed_at": timestamp, "updated_at": timestamp}).Error
}

func ListAgentPlanSteps(organizationID, userID, runID string) ([]model.AgentPlanStep, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var steps []model.AgentPlanStep
	err = db.Where("organization_id = ? AND user_id = ? AND run_id = ?", organizationID, userID, runID).Order("revision asc, position asc").Find(&steps).Error
	return steps, err
}

func SaveAgentFeedback(feedback model.AgentFeedback) (model.AgentFeedback, int64, error) {
	db, err := DB()
	if err != nil {
		return feedback, 0, err
	}
	var adjusted int64
	err = transactionWithSQLiteRetry(db, func(tx *gorm.DB) error {
		var run model.AgentRun
		if err := tx.Where("organization_id = ? AND user_id = ? AND id = ?", feedback.OrganizationID, feedback.UserID, feedback.RunID).First(&run).Error; err != nil {
			return err
		}
		var existing model.AgentFeedback
		lookup := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND user_id = ? AND run_id = ?", feedback.OrganizationID, feedback.UserID, feedback.RunID).First(&existing)
		previousScore := 0.0
		if lookup.Error == nil {
			feedback.ID, feedback.CreatedAt = existing.ID, existing.CreatedAt
			previousScore = agentFeedbackScore(existing.Signal)
			if err := tx.Model(&existing).Updates(map[string]any{"signal": feedback.Signal, "note": feedback.Note, "updated_at": feedback.UpdatedAt}).Error; err != nil {
				return err
			}
		} else if !errors.Is(lookup.Error, gorm.ErrRecordNotFound) {
			return lookup.Error
		} else if err := tx.Create(&feedback).Error; err != nil {
			return err
		}
		delta := agentFeedbackScore(feedback.Signal) - previousScore
		if delta == 0 {
			return nil
		}
		var memories []model.AgentMemory
		if err := tx.Where("organization_id = ? AND user_id = ? AND source_run_id = ? AND kind = ? AND status = ?", feedback.OrganizationID, feedback.UserID, feedback.RunID, model.AgentMemoryKindExperience, model.AgentMemoryStatusActive).Find(&memories).Error; err != nil {
			return err
		}
		for _, memory := range memories {
			confidence := math.Max(0.1, math.Min(0.9, memory.Confidence+delta))
			if err := tx.Model(&memory).Updates(map[string]any{"confidence": confidence, "updated_at": feedback.UpdatedAt}).Error; err != nil {
				return err
			}
			adjusted++
		}
		return nil
	})
	return feedback, adjusted, err
}

func agentFeedbackScore(signal model.AgentFeedbackSignal) float64 {
	switch signal {
	case model.AgentFeedbackSignalAccepted, model.AgentFeedbackSignalHelpful:
		return 0.05
	case model.AgentFeedbackSignalUnhelpful, model.AgentFeedbackSignalDeleted:
		return -0.1
	case model.AgentFeedbackSignalCorrected:
		return -0.06
	default:
		return 0
	}
}

func GetAgentMetricsData(organizationID, userID, since string) (AgentMetricsData, error) {
	db, err := DB()
	if err != nil {
		return AgentMetricsData{}, err
	}
	result := AgentMetricsData{}
	if err := db.Where("organization_id = ? AND user_id = ? AND created_at >= ?", organizationID, userID, since).Find(&result.Runs).Error; err != nil {
		return result, err
	}
	if err := db.Where("organization_id = ? AND user_id = ? AND created_at >= ?", organizationID, userID, since).Find(&result.Steps).Error; err != nil {
		return result, err
	}
	if err := db.Where("organization_id = ? AND user_id = ? AND created_at >= ?", organizationID, userID, since).Find(&result.Feedback).Error; err != nil {
		return result, err
	}
	err = db.Model(&model.GenerationTask{}).Where("organization_id = ? AND user_id = ? AND created_at >= ? AND status != ? AND (request_id LIKE ? OR request_id LIKE ? OR request_id LIKE ?)", organizationID, userID, since, model.GenerationTaskStatusFailed, "agent-run-%-completion-%", "agent:agent-run-%", "agent-inspect:agent-run-%").Select("COALESCE(SUM(credits), 0)").Scan(&result.Credits).Error
	return result, err
}
