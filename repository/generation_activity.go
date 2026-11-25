package repository

import (
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

var generationCreditTypes = []model.CreditLogType{model.CreditLogTypeAIConsume, model.CreditLogTypeAIRefund, model.CreditLogTypeAILateRecharge}

const generationCreditLink = "linked_task.id = credit.related_id AND linked_task.user_id = credit.user_id"

func ListAdminGenerationActivity(q model.Query) (model.AdminGenerationActivityList, error) {
	result := model.AdminGenerationActivityList{Items: []model.AdminGenerationActivity{}}
	db, err := DB()
	if err != nil {
		return result, err
	}
	q.Normalize()
	err = db.Transaction(func(tx *gorm.DB) error {
		linkedTask := tx.Table("generation_tasks AS linked_task").Select("1").Where(generationCreditLink).Where("credit.type IN ?", generationCreditTypes)
		linkedCredits := tx.Table("credit_logs AS credit").Select("credit.related_id").
			Joins("JOIN generation_tasks AS linked_task ON "+generationCreditLink).Where("credit.type IN ?", generationCreditTypes)
		tasks := tx.Table("generation_tasks AS task").Select("task.id, 'task' AS kind, task.created_at, task.user_id, users.username, users.public_id AS public_user_id").Joins("LEFT JOIN users ON users.id = task.user_id")
		standalone := tx.Table("credit_logs AS credit").Select("credit.id, 'credit' AS kind, credit.created_at, credit.user_id, users.username, users.public_id AS public_user_id").
			Joins("LEFT JOIN users ON users.id = credit.user_id").Where("NOT EXISTS (?)", linkedTask)

		if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
			if number := searchUserNumber(keyword); number >= 0 {
				tasks = tasks.Where("(users.public_id = ? OR users.username = ?)", number, keyword)
				standalone = standalone.Where("(users.public_id = ? OR users.username = ?)", number, keyword)
			} else {
				like := "%" + keyword + "%"
				const creditText = "(credit.type LIKE ? OR credit.remark LIKE ? OR credit.related_id LIKE ?)"
				creditMatches := linkedCredits.Session(&gorm.Session{}).Where(creditText, like, like, like)
				tasks = tasks.Where(`(task.user_id LIKE ? OR users.username LIKE ? OR task.model LIKE ? OR task.upstream_model LIKE ?
					OR task.channel_name LIKE ? OR task.error_message LIKE ? OR task.request_id = ? OR task.id = ? OR task.id IN (?))`,
					like, like, like, like, like, like, keyword, keyword, creditMatches)
				standalone = standalone.Where("(credit.user_id LIKE ? OR users.username LIKE ? OR "+creditText+")", like, like, like, like, like)
			}
		}
		statusFilter := q.Type != "" && q.Type != "all"
		modalityFilter := q.Category != "" && q.Category != "all" && q.Category != "credits"
		if statusFilter {
			tasks = tasks.Where("task.status = ?", q.Type)
		}
		if modalityFilter {
			tasks = tasks.Where("task.modality = ?", q.Category)
		} else if q.Category == "credits" {
			tasks = tasks.Where("task.id IN (?)", linkedCredits)
		}

		activity := tx.Table("(?) AS activity", tasks)
		if !statusFilter && !modalityFilter {
			activity = tx.Table("(? UNION ALL ?) AS activity", tasks, standalone)
		}
		if err := activity.Count(&result.Total).Error; err != nil || result.Total == 0 {
			return err
		}
		if err := activity.Order("created_at DESC, id DESC, kind ASC").Offset(q.Offset()).Limit(q.PageSize).Scan(&result.Items).Error; err != nil || len(result.Items) == 0 {
			return err
		}

		var taskIDs, creditIDs []string
		taskRows := make(map[string]*model.AdminGenerationActivity)
		creditRows := make(map[string]*model.AdminGenerationActivity)
		for i := range result.Items {
			row := &result.Items[i]
			row.CreditLogs = []model.CreditLog{}
			if row.Kind == "task" {
				taskIDs = append(taskIDs, row.ID)
				taskRows[row.ID] = row
			} else {
				creditIDs = append(creditIDs, row.ID)
				creditRows[row.ID] = row
			}
		}
		if len(taskIDs) > 0 {
			var pageTasks []model.GenerationTask
			if err := tx.Select(`id, organization_id, user_id, request_id, batch_job_id, batch_item_id, model, upstream_model, channel_name, channel_id,
				path, modality, operation, resolution_tier, quantity, credits, credit_source, status, upstream_task_id, error_message, duration_ms, created_at, updated_at`).
				Where("id IN ?", taskIDs).Find(&pageTasks).Error; err != nil {
				return err
			}
			for i := range pageTasks {
				taskRows[pageTasks[i].ID].Task = &pageTasks[i]
			}
		}
		var logs []model.CreditLog
		if err := tx.Table("credit_logs AS credit").Where("credit.id IN ? OR (credit.related_id IN ? AND EXISTS (?))", creditIDs, taskIDs, linkedTask).
			Order("credit.created_at ASC, credit.id ASC").Find(&logs).Error; err != nil {
			return err
		}
		for _, log := range logs {
			row := creditRows[log.ID]
			if row == nil {
				row = taskRows[log.RelatedID]
			}
			log.PublicUserID, log.Username = row.PublicUserID, row.Username
			row.CreditLogs = append(row.CreditLogs, log)
		}
		return nil
	})
	return result, err
}
