package repository

import (
	"errors"

	"github.com/yypyyd/infinite-canvas/model"
)

// SaveGenerationProtocol runs before a potentially billable upstream submission.
// A claimed task cannot be submitted again by a retrying caller.
func SaveGenerationProtocol(task model.GenerationTask, snapshot *model.GenerationProtocolSnapshot, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := generationTaskReconcileScope(db.Model(&model.GenerationTask{}), task.ReconcileLeaseToken, timestamp).
		Where("id = ? AND status = ? AND (protocol_snapshot IS NULL OR protocol_snapshot = '' OR protocol_snapshot = 'null')", task.ID, model.GenerationTaskStatusRunning).
		Select("ProtocolSnapshot", "UpdatedAt").Updates(model.GenerationTask{ProtocolSnapshot: snapshot, UpdatedAt: timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("任务已经提交或已结束，不能再次创建上游任务")
	}
	return nil
}
