package service

import (
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

type GenerationTaskInput struct {
	UserID         string
	OrganizationID string
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
	task := model.GenerationTask{
		ID:             newID("task"),
		UserID:         input.UserID,
		OrganizationID: input.OrganizationID,
		Model:          input.Model,
		UpstreamModel:  input.UpstreamModel,
		ChannelName:    input.ChannelName,
		Path:           input.Path,
		Modality:       input.Modality,
		Operation:      input.Operation,
		ResolutionTier: input.ResolutionTier,
		Quantity:       input.Quantity,
		Credits:        input.Credits,
		Status:         model.GenerationTaskStatusRunning,
		CreatedAt:      nowText,
		UpdatedAt:      nowText,
	}
	return repository.SaveGenerationTask(task)
}

func FinishGenerationTask(task model.GenerationTask, status model.GenerationTaskStatus, errMessage string) {
	if task.ID == "" {
		return
	}
	task.Status = status
	task.ErrorMessage = errMessage
	task.UpdatedAt = now()
	if startedAt, err := time.Parse(time.RFC3339, task.CreatedAt); err == nil {
		task.DurationMs = time.Since(startedAt).Milliseconds()
	}
	_, _ = repository.SaveGenerationTask(task)
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
