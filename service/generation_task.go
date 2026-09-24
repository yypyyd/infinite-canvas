package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

type GenerationTaskInput struct {
	UserID          string
	OrganizationID  string
	RequestID       string
	BatchJobID      string
	BatchItemID     string
	Model           string
	UpstreamModel   string
	ChannelName     string
	Path            string
	Modality        string
	Operation       string
	ResolutionTier  string
	Quantity        int
	Credits         int
	PricingSnapshot model.PricingSnapshot
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

func RecoverGenerationTask(organizationID, userID, requestID string) (model.GenerationTaskRecovery, error) {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return model.GenerationTaskRecovery{}, safeMessageError{message: "请求编号不能为空"}
	}
	task, exists, err := repository.GetGenerationTaskByRequest(organizationID, userID, requestID)
	if err != nil {
		return model.GenerationTaskRecovery{}, err
	}
	if !exists {
		return model.GenerationTaskRecovery{}, safeMessageError{message: "生成任务不存在"}
	}
	result := model.GenerationTaskRecovery{
		RequestID: task.RequestID, Model: task.Model, Modality: task.Modality, Path: task.Path,
		Status: task.Status, UpstreamTaskID: task.UpstreamTaskID, ErrorMessage: task.ErrorMessage,
		StorageKeys: task.StorageKeys,
		CreatedAt:   task.CreatedAt, UpdatedAt: task.UpdatedAt,
	}
	if json.Valid([]byte(task.ResultJSON)) {
		result.Result = refreshRecoveredImageURLs(model.AuthUser{ID: userID, OrganizationID: organizationID}, task)
	}
	return result, nil
}

func refreshRecoveredImageURLs(user model.AuthUser, task model.GenerationTask) json.RawMessage {
	result := json.RawMessage(task.ResultJSON)
	if task.Modality != "image" || len(task.StorageKeys) == 0 {
		return result
	}
	var payload map[string]any
	if json.Unmarshal(result, &payload) != nil {
		return result
	}
	items, ok := payload["data"].([]any)
	if !ok {
		return result
	}
	changed := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		storageKey, _ := item["storage_key"].(string)
		if strings.TrimSpace(storageKey) == "" {
			continue
		}
		if encoded, _ := item["b64_json"].(string); strings.TrimSpace(encoded) != "" {
			if _, hasURL := item["url"]; hasURL {
				delete(item, "url")
				changed = true
			}
			continue
		}
		if fileURL, ok := UserWorkspaceFileURL(user, storageKey, ""); ok {
			item["url"] = fileURL
			changed = true
		}
	}
	if !changed {
		return result
	}
	refreshed, err := json.Marshal(payload)
	if err != nil {
		return result
	}
	return refreshed
}

func UserGenerationTaskByRequest(organizationID, userID, requestID string) (model.GenerationTask, error) {
	task, exists, err := repository.GetGenerationTaskByRequest(organizationID, userID, strings.TrimSpace(requestID))
	if err != nil {
		return model.GenerationTask{}, err
	}
	if !exists {
		return model.GenerationTask{}, safeMessageError{message: "生成任务不存在"}
	}
	return task, nil
}

func UserGenerationTaskByUpstreamID(organizationID, userID, upstreamTaskID string) (model.GenerationTask, bool, error) {
	return repository.GetGenerationTaskByUpstreamID(organizationID, userID, strings.TrimSpace(upstreamTaskID))
}

func AcknowledgeGenerationTaskRecoveries(organizationID, userID string, requestIDs []string) error {
	unique := make([]string, 0, len(requestIDs))
	seen := map[string]bool{}
	for _, requestID := range requestIDs {
		requestID = strings.TrimSpace(requestID)
		if requestID == "" || seen[requestID] {
			continue
		}
		if len(requestID) > 191 || len(unique) >= 50 {
			return safeMessageError{message: "恢复确认参数无效"}
		}
		seen[requestID] = true
		unique = append(unique, requestID)
	}
	if len(unique) == 0 {
		return nil
	}
	return repository.ClearGenerationTaskRecoveryResults(organizationID, userID, unique, now())
}

func BeginGenerationTask(input GenerationTaskInput) (model.GenerationTask, error) {
	nowText := now()
	creditSource := model.CreditSourcePersonal
	organization, exists, err := repository.GetOrganization(input.OrganizationID)
	if err != nil {
		return model.GenerationTask{}, err
	}
	if !exists || organization.Status != "active" {
		return model.GenerationTask{}, safeMessageError{message: "企业不存在或已停用"}
	}
	if organization.CreditMode == model.OrganizationCreditModeShared {
		creditSource = model.CreditSourceOrganization
	}
	input.RequestID = strings.TrimSpace(input.RequestID)
	if len(input.RequestID) > 191 {
		return model.GenerationTask{}, safeMessageError{message: "请求编号过长"}
	}
	if input.RequestID == "" {
		input.RequestID = newID("request")
	}
	task := model.GenerationTask{
		ID:              newID("task"),
		UserID:          input.UserID,
		OrganizationID:  input.OrganizationID,
		RequestID:       input.RequestID,
		BatchJobID:      strings.TrimSpace(input.BatchJobID),
		BatchItemID:     strings.TrimSpace(input.BatchItemID),
		Model:           input.Model,
		UpstreamModel:   input.UpstreamModel,
		ChannelName:     input.ChannelName,
		Path:            input.Path,
		Modality:        input.Modality,
		Operation:       input.Operation,
		ResolutionTier:  input.ResolutionTier,
		Quantity:        input.Quantity,
		Credits:         input.Credits,
		PricingSnapshot: input.PricingSnapshot,
		CreditSource:    creditSource,
		Status:          model.GenerationTaskStatusRunning,
		CreatedAt:       nowText,
		UpdatedAt:       nowText,
	}
	var log *model.CreditLog
	if input.Credits > 0 {
		log = &model.CreditLog{ID: newID("credit"), UserID: input.UserID, OrganizationID: input.OrganizationID, CreditSource: creditSource, Type: model.CreditLogTypeAIConsume, Amount: -input.Credits, Remark: "调用模型 " + input.Model, Extra: generationTaskCreditExtra(input.Model, input.Path, input.PricingSnapshot), CreatedAt: nowText}
	}
	if task.BatchJobID != "" || task.BatchItemID != "" {
		if task.BatchJobID == "" || task.BatchItemID == "" {
			return task, safeMessageError{message: "批量任务扣费关联无效"}
		}
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
		var log *model.CreditLog
		if task.Credits > 0 {
			log = &model.CreditLog{ID: newID("credit"), UserID: task.UserID, OrganizationID: task.OrganizationID, CreditSource: task.CreditSource, Type: model.CreditLogTypeAIRefund, Amount: task.Credits, Remark: "模型调用失败返还 " + task.Model, Extra: generationTaskCreditExtra(task.Model, task.Path, task.PricingSnapshot), CreatedAt: task.UpdatedAt}
		}
		return retryGenerationTaskWrite(func() error {
			_, err := repository.FailGenerationTaskAndRefund(task, log)
			return err
		})
	}
	return retryGenerationTaskWrite(func() error { return repository.CompleteGenerationTask(task) })
}

func generationTaskCreditExtra(modelName string, path string, snapshot model.PricingSnapshot) string {
	extra := map[string]any{"model": modelName, "path": path}
	if snapshot.Source != "" {
		extra["pricingSource"] = snapshot.Source
		extra["effectiveRatio"] = snapshot.EffectiveRatio
	}
	value, _ := json.Marshal(extra)
	return string(value)
}

// UpdateGenerationTaskChannel records a failover channel before the next upstream request starts.
func UpdateGenerationTaskChannel(task *model.GenerationTask, channelName string, upstreamModel string) error {
	updatedAt := now()
	err := retryGenerationTaskWrite(func() error {
		return repository.UpdateRunningGenerationTaskChannel(task.ID, channelName, upstreamModel, updatedAt)
	})
	if err == nil {
		task.ChannelName = channelName
		task.UpstreamModel = upstreamModel
		task.UpdatedAt = updatedAt
	}
	return err
}

func UpdateGenerationTaskRecovery(task *model.GenerationTask, upstreamTaskID string, result []byte, storageKeys []string) error {
	if task == nil || task.ID == "" {
		return nil
	}
	updatedAt := now()
	err := retryGenerationTaskWrite(func() error {
		return repository.UpdateGenerationTaskRecovery(task.ID, strings.TrimSpace(upstreamTaskID), string(result), storageKeys, updatedAt)
	})
	if err == nil {
		task.UpstreamTaskID = strings.TrimSpace(upstreamTaskID)
		task.ResultJSON = string(result)
		task.StorageKeys = storageKeys
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
	current := time.Now()
	today := time.Date(current.Year(), current.Month(), current.Day(), 0, 0, 0, 0, current.Location())
	sinceToday := repository.LocalDayStartUTC(0)
	sincePeriod := repository.LocalDayStartUTC(29)
	registrations, err := repository.CountUsersSince(sinceToday)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	activeUsers, err := repository.CountActiveUsersSince(sinceToday)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	consumedCredits, err := repository.SumConsumedCreditsSince(sinceToday)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	rechargedCredits, err := repository.SumRechargeCreditsSince(sinceToday)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	rows, err := repository.ListDashboardTaskRowsSince(sincePeriod)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	hourly, daily, statuses, modalities, topModels, channelErrors := aggregateDashboardSeries(rows, today, 30)
	todayTasks := int64(0)
	todayFailed := int64(0)
	for _, point := range hourly {
		todayTasks += point.Tasks
		todayFailed += point.Failed
	}
	failureRate := int64(0)
	if todayTasks > 0 {
		failureRate = todayFailed * 100 / todayTasks
	}
	recentTasks, err := repository.RecentGenerationTasks(20)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	recentFailures, err := repository.RecentFailedGenerationTasks(8)
	if err != nil {
		return model.AdminDashboard{}, err
	}
	return model.AdminDashboard{
		GeneratedAt: now(),
		Metrics: []model.DashboardMetric{
			{Key: "registrations", Label: "今日注册", Value: registrations},
			{Key: "activeUsers", Label: "今日活跃", Value: activeUsers},
			{Key: "tasks", Label: "今日生成", Value: todayTasks},
			{Key: "consumedCredits", Label: "算力消耗", Value: consumedCredits},
			{Key: "failureRate", Label: "失败率", Value: failureRate},
			{Key: "rechargedCredits", Label: "兑换充值", Value: rechargedCredits},
		},
		Hourly:         hourly,
		Daily:          daily,
		Statuses:       statuses,
		Modalities:     modalities,
		RecentTasks:    recentTasks,
		TopModels:      topModels,
		ChannelErrors:  channelErrors,
		RecentFailures: recentFailures,
	}, nil
}

func aggregateDashboardSeries(rows []repository.DashboardTaskRow, today time.Time, days int) ([]model.DashboardSeriesPoint, []model.DashboardSeriesPoint, []model.DashboardNameValue, []model.DashboardNameValue, []model.DashboardNameValue, []model.DashboardNameValue) {
	periodStart := today.AddDate(0, 0, -(days - 1))
	todayEnd := today.AddDate(0, 0, 1)
	hourly := make([]model.DashboardSeriesPoint, 24)
	for hour := 0; hour < 24; hour++ {
		hourly[hour] = model.DashboardSeriesPoint{Label: fmt.Sprintf("%02d:00", hour)}
	}
	daily := make([]model.DashboardSeriesPoint, days)
	for index := 0; index < days; index++ {
		daily[index] = model.DashboardSeriesPoint{Label: periodStart.AddDate(0, 0, index).Format("01-02")}
	}
	statusCounts := map[model.GenerationTaskStatus]int64{
		model.GenerationTaskStatusSuccess: 0,
		model.GenerationTaskStatusFailed:  0,
		model.GenerationTaskStatusRunning: 0,
	}
	modalityCounts := map[string]int64{"image": 0, "video": 0, "text": 0, "audio": 0}
	modelCounts := map[string]int64{}
	channelFails := map[string]int64{}
	for _, row := range rows {
		parsed, ok := parseDashboardTime(row.CreatedAt)
		if !ok {
			continue
		}
		local := parsed.In(today.Location())
		dayIndex := int(local.Sub(periodStart).Hours() / 24)
		if dayIndex >= 0 && dayIndex < days {
			applySeriesPoint(&daily[dayIndex], row)
			statusCounts[row.Status]++
			if _, exists := modalityCounts[row.Modality]; exists {
				modalityCounts[row.Modality]++
			}
			if row.Model != "" {
				modelCounts[row.Model]++
			}
			if row.Status == model.GenerationTaskStatusFailed && row.ChannelName != "" {
				channelFails[row.ChannelName]++
			}
		}
		if !local.Before(today) && local.Before(todayEnd) {
			applySeriesPoint(&hourly[local.Hour()], row)
		}
	}
	return hourly, daily,
		[]model.DashboardNameValue{
			{Name: "成功", Value: statusCounts[model.GenerationTaskStatusSuccess]},
			{Name: "失败", Value: statusCounts[model.GenerationTaskStatusFailed]},
			{Name: "运行中", Value: statusCounts[model.GenerationTaskStatusRunning]},
		},
		[]model.DashboardNameValue{
			{Name: "图片", Value: modalityCounts["image"]},
			{Name: "视频", Value: modalityCounts["video"]},
			{Name: "文本", Value: modalityCounts["text"]},
			{Name: "音频", Value: modalityCounts["audio"]},
		},
		topNameValues(modelCounts, 8),
		topNameValues(channelFails, 8)
}

func applySeriesPoint(point *model.DashboardSeriesPoint, row repository.DashboardTaskRow) {
	point.Tasks++
	point.Credits += int64(row.Credits)
	switch row.Status {
	case model.GenerationTaskStatusSuccess:
		point.Success++
	case model.GenerationTaskStatusFailed:
		point.Failed++
	}
}

func parseDashboardTime(value string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02T15:04:05.000000000Z", time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func topNameValues(counts map[string]int64, limit int) []model.DashboardNameValue {
	items := make([]model.DashboardNameValue, 0, len(counts))
	for name, value := range counts {
		items = append(items, model.DashboardNameValue{Name: name, Value: value})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Value > items[j].Value })
	if len(items) > limit {
		return items[:limit]
	}
	return items
}
