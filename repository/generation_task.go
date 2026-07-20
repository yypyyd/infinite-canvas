package repository

import (
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

func ListGenerationTasks(q model.Query) ([]model.GenerationTask, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := applyGenerationTaskFilters(db.Model(&model.GenerationTask{}), q)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GenerationTask
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func ListUserGenerationTasks(organizationID string, userID string, q model.Query) ([]model.GenerationTask, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := applyGenerationTaskFilters(db.Model(&model.GenerationTask{}).Where("organization_id = ? AND user_id = ?", organizationID, userID), q)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.GenerationTask
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func SaveGenerationTask(task model.GenerationTask) (model.GenerationTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	return task, db.Save(&task).Error
}

func CountUsersSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.User{}).Where("created_at >= ?", since).Count(&total).Error
	return total, err
}

func CountActiveUsersSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.User{}).Where("last_login_at >= ?", since).Count(&total).Error
	return total, err
}

func CountGenerationTasksSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.GenerationTask{}).Where("created_at >= ?", since).Count(&total).Error
	return total, err
}

func CountFailedGenerationTasksSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.GenerationTask{}).Where("created_at >= ? AND status = ?", since, model.GenerationTaskStatusFailed).Count(&total).Error
	return total, err
}

func SumConsumedCreditsSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.CreditLog{}).Where("created_at >= ? AND type = ?", since, model.CreditLogTypeAIConsume).Select("COALESCE(SUM(-amount), 0)").Scan(&total).Error
	return total, err
}

func SumRechargeCreditsSince(since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.CreditLog{}).Where("created_at >= ? AND type = ?", since, model.CreditLogTypeRedeem).Select("COALESCE(SUM(amount), 0)").Scan(&total).Error
	return total, err
}

func TopGenerationTaskModelsSince(since string, limit int) ([]model.DashboardNameValue, error) {
	return groupedGenerationTaskCounts(since, "model", "model <> ''", limit)
}

func ChannelGenerationErrorsSince(since string, limit int) ([]model.DashboardNameValue, error) {
	return groupedGenerationTaskCounts(since, "channel_name", "status = 'failed' AND channel_name <> ''", limit)
}

func RecentGenerationTasks(limit int) ([]model.GenerationTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.GenerationTask
	err = db.Order("created_at desc").Limit(limit).Find(&items).Error
	return items, err
}

func RecentFailedGenerationTasks(limit int) ([]model.GenerationTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.GenerationTask
	err = db.Where("status = ?", model.GenerationTaskStatusFailed).Order("created_at desc").Limit(limit).Find(&items).Error
	return items, err
}

func groupedGenerationTaskCounts(since string, column string, extraWhere string, limit int) ([]model.DashboardNameValue, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	type row struct {
		Name  string
		Value int64
	}
	var rows []row
	tx := db.Model(&model.GenerationTask{}).Where("created_at >= ?", since)
	if extraWhere != "" {
		tx = tx.Where(extraWhere)
	}
	err = tx.Select(column+" AS name, COUNT(*) AS value").Group(column).Order("value desc").Limit(limit).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	items := make([]model.DashboardNameValue, 0, len(rows))
	for _, item := range rows {
		items = append(items, model.DashboardNameValue{Name: item.Name, Value: item.Value})
	}
	return items, nil
}

func applyGenerationTaskFilters(tx *gorm.DB, q model.Query) *gorm.DB {
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("model LIKE ? OR upstream_model LIKE ? OR channel_name LIKE ? OR error_message LIKE ? OR user_id LIKE ?", like, like, like, like, like)
	}
	if q.Type != "" && q.Type != "all" {
		tx = tx.Where("status = ?", q.Type)
	}
	if q.Category != "" && q.Category != "all" {
		tx = tx.Where("modality = ?", q.Category)
	}
	return tx
}

func DayStartRFC3339() string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format(time.RFC3339)
}
