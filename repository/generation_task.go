package repository

import (
	"errors"
	"fmt"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
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
			if organization.Credits-organization.ReservedCredits < task.Credits { return ErrInsufficientOrganizationCredits }
			if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+organization.ReservedCredits+task.Credits > organization.MonthlyCreditBudget { return ErrOrganizationCreditBudgetExceeded }
			result := tx.Model(&model.Organization{}).Where("id = ? AND credits - reserved_credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + reserved_credits + ? <= monthly_credit_budget)", organization.ID, task.Credits, task.Credits).Updates(map[string]any{
				"credits": gorm.Expr("credits - ?", task.Credits), "monthly_credits_used": gorm.Expr("monthly_credits_used + ?", task.Credits), "updated_at": task.UpdatedAt,
			})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrOrganizationCreditBudgetExceeded }
			organization.Credits -= task.Credits
			log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = task.ID, task.OrganizationID, model.CreditSourceOrganization, organization.Credits
			return tx.Create(log).Error
		}
		result := tx.Model(&model.User{}).Where("id = ? AND credits - reserved_credits >= ?", task.UserID, task.Credits).Updates(map[string]any{
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

func CreateBatchGenerationTaskWithCharge(task model.GenerationTask, log *model.CreditLog) (model.GenerationTask, error) {
	db, err := DB()
	if err != nil { return task, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var existing model.GenerationTask
		if err := tx.Where("organization_id = ? AND user_id = ? AND request_id = ?", task.OrganizationID, task.UserID, task.RequestID).First(&existing).Error; err == nil { return ErrGenerationTaskRequestConflict } else if !errors.Is(err, gorm.ErrRecordNotFound) { return err }
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND created_by = ? AND status NOT IN ?", task.OrganizationID, task.BatchJobID, task.UserID, []model.BatchProductionStatus{model.BatchProductionStatusCompleted, model.BatchProductionStatusCancelled}).First(&job).Error; err != nil { return err }
		if err := tx.Where("organization_id = ? AND job_id = ? AND id = ? AND estimated_credits = ?", task.OrganizationID, task.BatchJobID, task.BatchItemID, task.Credits).First(&model.BatchProductionItem{}).Error; err != nil { return err }
		if err := tx.Where("organization_id = ? AND user_id = ? AND request_id = ?", task.OrganizationID, task.UserID, task.RequestID).First(&existing).Error; err == nil { return ErrGenerationTaskRequestConflict } else if !errors.Is(err, gorm.ErrRecordNotFound) { return err }
		task.CreditSource = job.CreditSource
		if log != nil { log.CreditSource = job.CreditSource }
		if task.Credits < 0 || job.ReservedCredits-job.SettledCredits < task.Credits { return errors.New("batch production reserved credits are insufficient") }
		if task.CreditSource == model.CreditSourceOrganization {
			var organization model.Organization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", task.OrganizationID, "active").First(&organization).Error; err != nil { return err }
			budgetMonth := creditBudgetMonth(task.CreatedAt)
			if organization.CreditBudgetMonth != budgetMonth {
				if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{"monthly_credits_used": 0, "credit_budget_month": budgetMonth}).Error; err != nil { return err }
				organization.MonthlyCreditsUsed, organization.CreditBudgetMonth = 0, budgetMonth
			}
			if organization.Credits < task.Credits || organization.ReservedCredits < task.Credits { return ErrInsufficientOrganizationCredits }
			if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+organization.ReservedCredits > organization.MonthlyCreditBudget { return ErrOrganizationCreditBudgetExceeded }
			result := tx.Model(&model.Organization{}).Where("id = ? AND credits >= ? AND reserved_credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + reserved_credits <= monthly_credit_budget)", organization.ID, task.Credits, task.Credits).Updates(map[string]any{"credits": gorm.Expr("credits - ?", task.Credits), "reserved_credits": gorm.Expr("reserved_credits - ?", task.Credits), "monthly_credits_used": gorm.Expr("monthly_credits_used + ?", task.Credits), "updated_at": task.UpdatedAt})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrOrganizationCreditBudgetExceeded }
			if log != nil { log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = task.ID, task.OrganizationID, model.CreditSourceOrganization, organization.Credits-task.Credits }
		} else if task.CreditSource == model.CreditSourcePersonal {
			var user model.User
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", task.UserID).First(&user).Error; err != nil { return err }
			if user.Credits < task.Credits || user.ReservedCredits < task.Credits { return ErrInsufficientUserCredits }
			result := tx.Model(&model.User{}).Where("id = ? AND credits >= ? AND reserved_credits >= ?", user.ID, task.Credits, task.Credits).Updates(map[string]any{"credits": gorm.Expr("credits - ?", task.Credits), "reserved_credits": gorm.Expr("reserved_credits - ?", task.Credits), "updated_at": task.UpdatedAt})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrInsufficientUserCredits }
			if log != nil { log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = task.ID, task.OrganizationID, model.CreditSourcePersonal, user.Credits-task.Credits }
		} else {
			return errors.New("batch production credit source is invalid")
		}
		result := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND id = ? AND settled_credits + ? <= reserved_credits", task.OrganizationID, task.BatchJobID, task.Credits).Updates(map[string]any{"settled_credits": gorm.Expr("settled_credits + ?", task.Credits), "actual_credits": gorm.Expr("actual_credits + ?", task.Credits), "updated_at": task.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return errors.New("batch production reservation settlement conflict") }
		if err := tx.Create(&task).Error; err != nil { if isDuplicateKeyError(err) { return ErrGenerationTaskRequestConflict }; return err }
		if log != nil { return tx.Create(log).Error }
		return nil
	})
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

func GetGenerationTaskByUpstreamID(organizationID, userID, upstreamTaskID string) (model.GenerationTask, bool, error) {
	db, err := DB()
	if err != nil {
		return model.GenerationTask{}, false, err
	}
	var task model.GenerationTask
	err = db.Where("organization_id = ? AND user_id = ? AND upstream_task_id = ?", organizationID, userID, upstreamTaskID).Order("created_at desc").First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.GenerationTask{}, false, nil
	}
	return task, err == nil, err
}

func ListStaleRunningGenerationTasks(createdBefore string, limit int) ([]model.GenerationTask, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var tasks []model.GenerationTask
	err = db.Where("status = ? AND batch_job_id = '' AND batch_item_id = '' AND created_at <= ?", model.GenerationTaskStatusRunning, createdBefore).
		Order("created_at asc").
		Limit(limit).
		Find(&tasks).Error
	return tasks, err
}

func UpdateGenerationTaskRecovery(taskID, upstreamTaskID, resultJSON string, storageKeys []string, updatedAt string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var task model.GenerationTask
		if err := tx.Where("id = ? AND status IN ?", taskID, []model.GenerationTaskStatus{model.GenerationTaskStatusRunning, model.GenerationTaskStatusSuccess}).First(&task).Error; err != nil { return err }
		result := tx.Model(&task).Select("UpstreamTaskID", "ResultJSON", "StorageKeys", "UpdatedAt").Updates(model.GenerationTask{UpstreamTaskID: upstreamTaskID, ResultJSON: resultJSON, StorageKeys: storageKeys, UpdatedAt: updatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return errors.New("generation task recovery is unavailable") }
		return replaceUserFileReferences(tx, task.OrganizationID, "generation_task", task.ID, task.ID+"-file", storageKeys, false, updatedAt)
	})
}

func ClearGenerationTaskRecoveryResults(organizationID, userID string, requestIDs []string, updatedAt string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var tasks []model.GenerationTask
		if err := tx.Where("organization_id = ? AND user_id = ? AND request_id IN ? AND status = ?", organizationID, userID, requestIDs, model.GenerationTaskStatusSuccess).Find(&tasks).Error; err != nil { return err }
		for _, task := range tasks {
			if err := replaceUserFileReferences(tx, organizationID, "generation_task", task.ID, task.ID+"-file", nil, true, updatedAt); err != nil { return err }
		}
		return tx.Model(&model.GenerationTask{}).Where("organization_id = ? AND user_id = ? AND request_id IN ? AND status = ?", organizationID, userID, requestIDs, model.GenerationTaskStatusSuccess).Updates(map[string]any{"result_json": "", "storage_keys": nil}).Error
	})
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

// UpdateRunningGenerationTaskChannel records the channel currently handling a running task.
func UpdateRunningGenerationTaskChannel(taskID string, channelName string, upstreamModel string, updatedAt string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.GenerationTask{}).Where("id = ? AND status = ?", taskID, model.GenerationTaskStatusRunning).Updates(map[string]any{
		"channel_name":   channelName,
		"upstream_model": upstreamModel,
		"updated_at":     updatedAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("generation task is not running")
	}
	return nil
}

func ReconcileCompletedBatchGenerationTasks(timestamp string, limit int) (scanned int, repaired int, rejected int, err error) {
	db, err := DB()
	if err != nil { return 0, 0, 0, err }
	if limit <= 0 { limit = 100 }
	if limit > 100 { limit = 100 }
	type candidate struct {
		TaskID        string `gorm:"column:task_id"`
		OrganizationID string `gorm:"column:organization_id"`
		BatchItemID   string `gorm:"column:batch_item_id"`
		BatchJobID    string `gorm:"column:batch_job_id"`
		RequestID     string `gorm:"column:request_id"`
		RunNumber     int    `gorm:"column:run_number"`
	}
	var candidates []candidate
	err = db.Table("generation_tasks AS tasks").
		Select("tasks.id AS task_id, tasks.organization_id AS organization_id, tasks.batch_item_id AS batch_item_id, tasks.batch_job_id AS batch_job_id, tasks.request_id AS request_id, items.run_number AS run_number").
		Joins("JOIN batch_production_items AS items ON items.id = tasks.batch_item_id AND items.organization_id = tasks.organization_id AND items.job_id = tasks.batch_job_id").
		Joins("JOIN batch_production_jobs AS jobs ON jobs.id = items.job_id AND jobs.organization_id = items.organization_id").
		Where("tasks.status = ? AND tasks.batch_item_id <> '' AND tasks.batch_job_id <> ''", model.GenerationTaskStatusRunning).
		Where("items.status = ? AND items.result_storage_key <> ''", model.BatchProductionStatusCompleted).
		Order("items.finished_at asc, tasks.id asc").
		Limit(limit).
		Scan(&candidates).Error
	if err != nil { return 0, 0, 0, err }
	scanned = len(candidates)
	for _, candidate := range candidates {
		if candidate.RequestID != batchGenerationTaskRequestID(candidate.BatchItemID, candidate.RunNumber) {
			rejected++
			continue
		}
		result := db.Model(&model.GenerationTask{}).
			Where("id = ? AND status = ? AND organization_id = ? AND batch_item_id = ? AND batch_job_id = ? AND request_id = ?", candidate.TaskID, model.GenerationTaskStatusRunning, candidate.OrganizationID, candidate.BatchItemID, candidate.BatchJobID, candidate.RequestID).
			Where("EXISTS (SELECT 1 FROM batch_production_items AS items JOIN batch_production_jobs AS jobs ON jobs.id = items.job_id AND jobs.organization_id = items.organization_id WHERE items.id = generation_tasks.batch_item_id AND items.organization_id = generation_tasks.organization_id AND items.job_id = generation_tasks.batch_job_id AND items.status = ? AND items.result_storage_key <> '' AND items.run_number = ?)", model.BatchProductionStatusCompleted, candidate.RunNumber).
			Updates(map[string]any{"status": model.GenerationTaskStatusSuccess, "error_message": "", "updated_at": timestamp})
		if result.Error != nil {
			rejected++
			if err == nil { err = result.Error }
			continue
		}
		if result.RowsAffected == 1 { repaired++ }
	}
	return scanned, repaired, rejected, err
}

func batchGenerationTaskRequestID(itemID string, runNumber int) string {
	return fmt.Sprintf("batch:%s:%d", itemID, runNumber)
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
		if saved.BatchJobID != "" {
			result := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND id = ? AND actual_credits >= ?", saved.OrganizationID, saved.BatchJobID, saved.Credits).Update("actual_credits", gorm.Expr("actual_credits - ?", saved.Credits))
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return errors.New("batch production actual credits refund conflict") }
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
			if log != nil {
				log.RelatedID, log.OrganizationID, log.CreditSource, log.Balance = saved.ID, saved.OrganizationID, model.CreditSourceOrganization, organization.Credits+saved.Credits
				if err := tx.Create(log).Error; err != nil { return err }
			}
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
		if log != nil {
			log.RelatedID = saved.ID
			log.OrganizationID = saved.OrganizationID
			log.CreditSource = model.CreditSourcePersonal
			log.Balance = user.Credits
			if err := tx.Create(log).Error; err != nil { return err }
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
