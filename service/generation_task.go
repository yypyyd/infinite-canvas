package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

type GenerationTaskInput struct {
	UserID         string
	OrganizationID string
	RequestID      string
	BatchJobID     string
	BatchItemID    string
	Model          string
	UpstreamModel  string
	ChannelName    string
	Path           string
	Modality       string
	Operation      string
	ResolutionTier string
	Quantity       int
	Credits        int
}

func ListGenerationTasks(q model.Query) (model.GenerationTaskList, error) {
	items, total, err := repository.ListGenerationTasks(q)
	if err != nil {
		return model.GenerationTaskList{}, err
	}
	return model.GenerationTaskList{Items: items, Total: int(total)}, nil
}

func ListUserGenerationTasks(organizationID string, userID string, q model.Query) (model.GenerationTaskList, error) {
	items, total, err := repository.ListUserGenerationTasks(organizationID, userID, q)
	if err != nil {
		return model.GenerationTaskList{}, err
	}
	return model.GenerationTaskList{Items: items, Total: int(total)}, nil
}

func BeginGenerationTask(input GenerationTaskInput) (model.GenerationTask, error) {
	nowText := now()
	creditSource := model.CreditSourcePersonal
	organization, exists, err := repository.GetOrganization(input.OrganizationID)
	if err != nil { return model.GenerationTask{}, err }
	if !exists || organization.Status != "active" { return model.GenerationTask{}, safeMessageError{message: "企业不存在或已停用"} }
	if organization.CreditMode == model.OrganizationCreditModeShared { creditSource = model.CreditSourceOrganization }
	input.RequestID = strings.TrimSpace(input.RequestID)
	if len(input.RequestID) > 191 {
		return model.GenerationTask{}, safeMessageError{message: "请求编号过长"}
	}
	if input.RequestID == "" {
		input.RequestID = newID("request")
	}
	task := model.GenerationTask{
		ID:             newID("task"),
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
		RequestID:      input.RequestID,
		BatchJobID:     strings.TrimSpace(input.BatchJobID),
		BatchItemID:    strings.TrimSpace(input.BatchItemID),
		Model:          input.Model,
		UpstreamModel:  input.UpstreamModel,
		ChannelName:    input.ChannelName,
		Path:           input.Path,
		Modality:       input.Modality,
		Operation:      input.Operation,
		ResolutionTier: input.ResolutionTier,
		Quantity:       input.Quantity,
		Credits:        input.Credits,
		CreditSource:   creditSource,
		Status:         model.GenerationTaskStatusRunning,
		CreatedAt:      nowText,
		UpdatedAt:      nowText,
	}
	var log *model.CreditLog
	if input.Credits > 0 {
		extra, _ := json.Marshal(map[string]string{"model": input.Model, "path": input.Path})
		log = &model.CreditLog{ID: newID("credit"), UserID: input.UserID, OrganizationID: input.OrganizationID, CreditSource: creditSource, Type: model.CreditLogTypeAIConsume, Amount: -input.Credits, Remark: "调用模型 " + input.Model, Extra: string(extra), CreatedAt: nowText}
	}
	if task.BatchJobID != "" || task.BatchItemID != "" {
		if task.BatchJobID == "" || task.BatchItemID == "" { return task, safeMessageError{message: "批量任务扣费关联无效"} }
		task, err = repository.CreateBatchGenerationTaskWithCharge(task, log)
	} else {
		task, err = repository.CreateGenerationTaskWithCharge(task, log)
	}
	if errors.Is(err, repository.ErrInsufficientUserCredits) {
		return task, safeMessageError{message: "个人算力余额不足"}
	}
	if errors.Is(err, repository.ErrInsufficientOrganizationCredits) {
		return task, safeMessageError{message: "企业共享算力余额不足"}
	}
	if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
		return task, safeMessageError{message: "企业本月算力预算不足"}
	}
	if errors.Is(err, repository.ErrGenerationTaskRequestConflict) {
		return task, safeMessageError{message: "请求已提交，请勿重复操作"}
	}
	return task, err
}

func FinishGenerationTask(task model.GenerationTask, status model.GenerationTaskStatus, errMessage string) error {
	if task.ID == "" {
		return nil
	}
	task.ErrorMessage = errMessage
	task.UpdatedAt = now()
	if startedAt, err := time.Parse(time.RFC3339, task.CreatedAt); err == nil {
		task.DurationMs = time.Since(startedAt).Milliseconds()
	}
	if status == model.GenerationTaskStatusFailed {
		extra, _ := json.Marshal(map[string]string{"model": task.Model, "path": task.Path})
		var log *model.CreditLog
		if task.Credits > 0 {
			log = &model.CreditLog{ID: newID("credit"), UserID: task.UserID, OrganizationID: task.OrganizationID, CreditSource: task.CreditSource, Type: model.CreditLogTypeAIRefund, Amount: task.Credits, Remark: "模型调用失败返还 " + task.Model, Extra: string(extra), CreatedAt: task.UpdatedAt}
		}
		return retryGenerationTaskWrite(func() error {
			_, err := repository.FailGenerationTaskAndRefund(task, log)
			return err
		})
	}
	return retryGenerationTaskWrite(func() error { return repository.CompleteGenerationTask(task) })
}

// UpdateGenerationTaskChannel records a failover channel before the next upstream request starts.
func UpdateGenerationTaskChannel(task *model.GenerationTask, channelName string, upstreamModel string) error {
	updatedAt := now()
	err := retryGenerationTaskWrite(func() error { return repository.UpdateRunningGenerationTaskChannel(task.ID, channelName, upstreamModel, updatedAt) })
	if err == nil {
		task.ChannelName = channelName
		task.UpstreamModel = upstreamModel
		task.UpdatedAt = updatedAt
	}
	return err
}

func retryGenerationTaskWrite(operation func() error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = operation()
		if !isGenerationTaskWriteConflict(err) || attempt == 3 {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
	}
	return err
}

func isGenerationTaskWriteConflict(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") || strings.Contains(message, "database table is locked") || strings.Contains(message, "sqlite_busy") || strings.Contains(message, "sqlite_locked")
}

func AdminDashboard() (model.AdminDashboard, error) {
	since := repository.DayStartRFC3339()
	registrations, err := repository.CountUsersSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	activeUsers, err := repository.CountActiveUsersSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	taskCount, err := repository.CountGenerationTasksSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	failedCount, err := repository.CountFailedGenerationTasksSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	consumedCredits, err := repository.SumConsumedCreditsSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	rechargedCredits, err := repository.SumRechargeCreditsSince(since)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	topModels, err := repository.TopGenerationTaskModelsSince(since, 8)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	channelErrors, err := repository.ChannelGenerationErrorsSince(since, 8)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	recentTasks, err := repository.RecentGenerationTasks(8)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	recentFailures, err := repository.RecentFailedGenerationTasks(6)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	failureRate := int64(0)
	if taskCount > 0 {
		failureRate = failedCount * 100 / taskCount
	}
	return model.AdminDashboard{
		Metrics: []model.DashboardMetric{
			{Key: "registrations", Label: "今日注册", Value: registrations},
			{Key: "activeUsers", Label: "今日活跃", Value: activeUsers},
			{Key: "tasks", Label: "生成次数", Value: taskCount},
			{Key: "consumedCredits", Label: "算力消耗", Value: consumedCredits},
			{Key: "failureRate", Label: "失败率", Value: failureRate},
			{Key: "rechargedCredits", Label: "兑换充值", Value: rechargedCredits},
		},
		RecentTasks:    recentTasks,
		TopModels:      topModels,
		ChannelErrors:  channelErrors,
		RecentFailures: recentFailures,
	}, nil
}
