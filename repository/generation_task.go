package repository

import (
	"errors"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInsufficientUserCredits       = errors.New("insufficient user credits")
	ErrInsufficientOrganizationCredits = errors.New("insufficient organization credits")
	ErrOrganizationCreditBudgetExceeded = errors.New("organization credit budget exceeded")
	ErrGenerationTaskRequestConflict = errors.New("generation task request conflict")
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

func CreateGenerationTaskWithCharge(task model.GenerationTask, log *model.CreditLog) (model.GenerationTask, error) {
	db, err := DB()
	if err != nil {
		return task, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&task).Error; err != nil {
			return err
		}
		if task.Credits <= 0 {
			return nil
		}
		if task.CreditSource == model.CreditSourceOrganization {
			var organization model.Organization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", task.OrganizationID, "active").First(&organization).Error; err != nil { return err }
			budgetMonth := creditBudgetMonth(task.CreatedAt)
			if organization.CreditBudgetMonth != budgetMonth {
				if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{"monthly_credits_used": 0, "credit_budget_month": budgetMonth}).Error; err != nil { return err }
				organization.MonthlyCreditsUsed, organization.CreditBudgetMonth = 0, budgetMonth
			}
			if organization.Credits < task.Credits { return ErrInsufficientOrganizationCredits }
			if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+task.Credits > organization.MonthlyCreditBudget { return ErrOrganizationCreditBudgetExceeded }
			result := tx.Model(&model.Organization{}).Where("id = ? AND credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + ? <= monthly_credit_budget)", organization.ID, task.Credits, task.Credits).Updates(map[string]any{
				"credits": gorm.Expr("credits - ?", task.Credits), "monthly_credits_used": gorm.Expr("monthly_credits_used + ?", task.Credits), "updated_at": task.UpdatedAt,
			})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrOrganizationCreditBudgetExceeded }
			organization.Credits -= task.Credits
			log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = task.ID, task.OrganizationID, model.CreditSourceOrganization, organization.Credits
			return tx.Create(log).Error
		}
		result := tx.Model(&model.User{}).Where("id = ? AND credits >= ?", task.UserID, task.Credits).Updates(map[string]any{
			"credits":    gorm.Expr("credits - ?", task.Credits),
			"updated_at": task.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientUserCredits
		}
		var user model.User
		if err := tx.First(&user, "id = ?", task.UserID).Error; err != nil {
			return err
		}
		log.RelatedID = task.ID
		log.OrganizationID = task.OrganizationID
		log.CreditSource = model.CreditSourcePersonal
		log.Balance = user.Credits
		return tx.Create(log).Error
	})
	if err != nil && task.RequestID != "" {
		var total int64
		if countErr := db.Model(&model.GenerationTask{}).Where("organization_id = ? AND user_id = ? AND request_id = ?", task.OrganizationID, task.UserID, task.RequestID).Count(&total).Error; countErr == nil && total > 0 {
			err = ErrGenerationTaskRequestConflict
		}
	}
	return task, err
}

func GetGenerationTaskByRequest(organizationID, userID, requestID string) (model.GenerationTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.GenerationTask{}, false, err
	}
	var task model.GenerationTask
	err = db.Where("organization_id = ? AND user_id = ? AND request_id = ?", organizationID, userID, requestID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.GenerationTask{}, false, nil
	}
	return task, err == nil, err
}

func CompleteGenerationTask(task model.GenerationTask) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.GenerationTask{}).Where("id = ? AND status = ?", task.ID, model.GenerationTaskStatusRunning).Updates(map[string]any{
		"status":        model.GenerationTaskStatusSuccess,
		"error_message": "",
		"duration_ms":   task.DurationMs,
		"updated_at":    task.UpdatedAt,
	}).Error
}

func FailGenerationTaskAndRefund(task model.GenerationTask, log *model.CreditLog) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	refunded := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var saved model.GenerationTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, "id = ?", task.ID).Error; err != nil {
			return err
		}
		if saved.Status != model.GenerationTaskStatusRunning {
			return nil
		}
		result := tx.Model(&model.GenerationTask{}).Where("id = ? AND status = ?", saved.ID, model.GenerationTaskStatusRunning).Updates(map[string]any{
			"status":        model.GenerationTaskStatusFailed,
			"error_message": task.ErrorMessage,
			"duration_ms":   task.DurationMs,
			"updated_at":    task.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return nil
		}
		if saved.Credits <= 0 {
			return nil
		}
		if saved.CreditSource == model.CreditSourceOrganization {
			var organization model.Organization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", saved.OrganizationID).Error; err != nil { return err }
			monthlyUsed := organization.MonthlyCreditsUsed
			budgetMonth := creditBudgetMonth(task.UpdatedAt)
			if organization.CreditBudgetMonth != budgetMonth { monthlyUsed = 0 }
			if creditBudgetMonth(saved.CreatedAt) == budgetMonth {
				monthlyUsed -= saved.Credits
				if monthlyUsed < 0 { monthlyUsed = 0 }
			}
			result := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{"credits": gorm.Expr("credits + ?", saved.Credits), "monthly_credits_used": monthlyUsed, "credit_budget_month": budgetMonth, "updated_at": task.UpdatedAt})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
			log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = saved.ID, saved.OrganizationID, model.CreditSourceOrganization, organization.Credits+saved.Credits
			if err := tx.Create(log).Error; err != nil { return err }
			refunded = true
			return nil
		}
		result = tx.Model(&model.User{}).Where("id = ?", saved.UserID).Updates(map[string]any{
			"credits":    gorm.Expr("credits + ?", saved.Credits),
			"updated_at": task.UpdatedAt,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		var user model.User
		if err := tx.First(&user, "id = ?", saved.UserID).Error; err != nil {
			return err
		}
		log.RelatedID = saved.ID
		log.OrganizationID = saved.OrganizationID
		log.CreditSource = model.CreditSourcePersonal
		log.Balance = user.Credits
		if err := tx.Create(log).Error; err != nil {
			return err
		}
		refunded = true
		return nil
	})
	return refunded, err
}

func creditBudgetMonth(timestamp string) string {
	if len(timestamp) >= 7 { return timestamp[:7] }
	return time.Now().UTC().Format("2006-01")
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
