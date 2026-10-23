package repository

import (
	"github.com/yypyyd/infinite-canvas/model"
)

// GenerationOutcomeRow is a minimal generation-task row for public model health.
type GenerationOutcomeRow struct {
	Model        string                     `json:"model"`
	Status       model.GenerationTaskStatus `json:"status"`
	ErrorMessage string                     `json:"errorMessage"`
	DurationMs   int64                      `json:"durationMs"`
	CreatedAt    string                     `json:"createdAt"`
}

// ListRecentGenerationOutcomes returns recent finished/running tasks for the
// given logical models. Channel and account fields are intentionally omitted.
func ListRecentGenerationOutcomes(since string, models []string, limit int) ([]GenerationOutcomeRow, error) {
	if len(models) == 0 {
		return nil, nil
	}
	if limit <= 0 {
		limit = 500
	}
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var rows []GenerationOutcomeRow
	err = db.Model(&model.GenerationTask{}).
		Select("model, status, error_message, duration_ms, created_at").
		Where("created_at >= ? AND model IN ?", since, models).
		Order("created_at DESC").
		Limit(limit).
		Scan(&rows).Error
	return rows, err
}