package service

import (
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	modelStatusWindow       = 30 * time.Minute
	modelStatusBucketCount  = 15
	modelStatusOutcomeLimit = 800
	modelStatusHighLatency  = int64(90_000)
)

// listRecentGenerationOutcomes is replaceable in tests.
var listRecentGenerationOutcomes = repository.ListRecentGenerationOutcomes

// PublicModelStatus builds per-logical-model health for the plaza.
// It aggregates generation outcomes and capacity without exposing channels.
func PublicModelStatus() (model.ModelStatusResponse, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ModelStatusResponse{}, err
	}
	normalized := normalizeSettings(settings)
	routes, routesKnown := poolRouteDetails()
	overview := buildModelOverview(normalized, routes, routesKnown)

	plazaIDs := plazaModelIDs(normalized.Public.ModelChannel)
	if len(plazaIDs) == 0 {
		return model.ModelStatusResponse{UpdatedAt: now(), Models: []model.ModelStatusItem{}}, nil
	}

	since := time.Now().UTC().Add(-modelStatusWindow).Format(timestampLayout)
	outcomes, err := listRecentGenerationOutcomes(since, plazaIDs, modelStatusOutcomeLimit)
	if err != nil {
		return model.ModelStatusResponse{}, err
	}

	byModel := map[string][]repository.GenerationOutcomeRow{}
	for _, row := range outcomes {
		byModel[row.Model] = append(byModel[row.Model], row)
	}
	available := map[string]bool{}
	for _, item := range overview.Models {
		available[item.ID] = item.Available
	}

	nowTime := time.Now().UTC()
	items := make([]model.ModelStatusItem, 0, len(plazaIDs))
	for _, id := range plazaIDs {
		items = append(items, buildModelStatusItem(id, available[id], routesKnown, byModel[id], nowTime))
	}
	return model.ModelStatusResponse{UpdatedAt: nowTime.Format(timestampLayout), Models: items}, nil
}

func plazaModelIDs(channel model.PublicModelChannelSetting) []string {
	available := map[string]bool{}
	for _, id := range channel.AvailableModels {
		available[id] = true
	}
	ids := make([]string, 0, len(channel.Models))
	for _, item := range channel.Models {
		if !item.Enabled || !available[item.ID] || item.Modality == "text" || item.Modality == "" {
			continue
		}
		if item.Modality != "image" && item.Modality != "video" && item.Modality != "audio" {
			continue
		}
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

func buildModelStatusItem(id string, capacityOK, routesKnown bool, rows []repository.GenerationOutcomeRow, nowTime time.Time) model.ModelStatusItem {
	timeline := buildModelStatusTimeline(rows, nowTime)
	latency := averageSuccessLatency(rows)
	status := deriveModelStatus(capacityOK, routesKnown, rows, latency)
	item := model.ModelStatusItem{
		ID:       id,
		Status:   status,
		Label:    modelStatusLabel(status),
		Timeline: timeline,
	}
	if latency != nil {
		item.LatencyMs = latency
	}
	return item
}

func deriveModelStatus(capacityOK, routesKnown bool, rows []repository.GenerationOutcomeRow, latency *int64) string {
	if !capacityOK {
		if !routesKnown && len(rows) == 0 {
			return model.ModelStatusUnknown
		}
		return model.ModelStatusUnavailable
	}

	success, fail, busy := countOutcomeClasses(rows)
	scored := success + fail + busy
	if scored == 0 {
		return model.ModelStatusOperational
	}

	failRate := float64(fail) / float64(scored)
	busyRate := float64(busy) / float64(scored)
	highLatency := latency != nil && *latency >= modelStatusHighLatency

	switch {
	case failRate >= 0.8 && busyRate < 0.2 && scored >= 2:
		return model.ModelStatusUnavailable
	case busyRate >= 0.15 || failRate >= 0.25 || highLatency:
		return model.ModelStatusDegraded
	default:
		return model.ModelStatusOperational
	}
}

func countOutcomeClasses(rows []repository.GenerationOutcomeRow) (success, fail, busy int) {
	for _, row := range rows {
		switch classifyGenerationOutcome(row) {
		case "success":
			success++
		case "busy":
			busy++
		case "fail":
			fail++
		}
	}
	return
}

func classifyGenerationOutcome(row repository.GenerationOutcomeRow) string {
	switch row.Status {
	case model.GenerationTaskStatusSuccess:
		return "success"
	case model.GenerationTaskStatusFailed:
		message := strings.ToLower(strings.TrimSpace(row.ErrorMessage))
		if isUserFaultGenerationError(message) {
			return "ignore"
		}
		if isBusyGenerationError(message) {
			return "busy"
		}
		return "fail"
	default:
		return "ignore"
	}
}

func isBusyGenerationError(message string) bool {
	if message == "" {
		return false
	}
	needles := []string{
		"繁忙", "busy", "temporary", "暂时", "overload", "under load",
		"rate limit", "ratelimit", "429", "503", "concurrency", "并发",
		"try again shortly", "provider temporary", "系统繁忙", "稍后重试",
	}
	for _, needle := range needles {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func isUserFaultGenerationError(message string) bool {
	if message == "" {
		return false
	}
	needles := []string{
		"content", "banned", "内容", "敏感", "参数", "unsupported",
		"insufficient", "余额", "算力不足", "reference too large", "参考",
		"idempotency", "unknown model", "未知模型",
	}
	for _, needle := range needles {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func averageSuccessLatency(rows []repository.GenerationOutcomeRow) *int64 {
	var sum, count int64
	for _, row := range rows {
		if row.Status != model.GenerationTaskStatusSuccess || row.DurationMs <= 0 {
			continue
		}
		sum += row.DurationMs
		count++
	}
	if count == 0 {
		return nil
	}
	avg := sum / count
	return &avg
}

func buildModelStatusTimeline(rows []repository.GenerationOutcomeRow, nowTime time.Time) []model.ModelStatusBucket {
	bucketDur := modelStatusWindow / time.Duration(modelStatusBucketCount)
	buckets := make([]model.ModelStatusBucket, modelStatusBucketCount)
	type stats struct {
		success, fail, busy int
		latencySum, latencyN int64
	}
	statsByIndex := make([]stats, modelStatusBucketCount)

	for i := 0; i < modelStatusBucketCount; i++ {
		end := nowTime.Add(-time.Duration(i) * bucketDur)
		buckets[i] = model.ModelStatusBucket{
			Status: model.ModelStatusUnknown,
			At:     end.Format(timestampLayout),
		}
	}

	for _, row := range rows {
		created, err := time.Parse(timestampLayout, row.CreatedAt)
		if err != nil {
			continue
		}
		age := nowTime.Sub(created)
		if age < 0 {
			age = 0
		}
		index := int(age / bucketDur)
		if index < 0 {
			index = 0
		}
		if index >= modelStatusBucketCount {
			continue
		}
		switch classifyGenerationOutcome(row) {
		case "success":
			statsByIndex[index].success++
			if row.DurationMs > 0 {
				statsByIndex[index].latencySum += row.DurationMs
				statsByIndex[index].latencyN++
			}
		case "busy":
			statsByIndex[index].busy++
		case "fail":
			statsByIndex[index].fail++
		}
	}

	for i := range buckets {
		s := statsByIndex[i]
		scored := s.success + s.fail + s.busy
		if scored == 0 {
			continue
		}
		var latency *int64
		if s.latencyN > 0 {
			avg := s.latencySum / s.latencyN
			latency = &avg
		}
		status := model.ModelStatusOperational
		failRate := float64(s.fail) / float64(scored)
		busyRate := float64(s.busy) / float64(scored)
		highLatency := latency != nil && *latency >= modelStatusHighLatency
		switch {
		case failRate >= 0.8 && busyRate < 0.2:
			status = model.ModelStatusUnavailable
		case busyRate >= 0.15 || failRate >= 0.25 || highLatency:
			status = model.ModelStatusDegraded
		}
		buckets[i].Status = status
		buckets[i].LatencyMs = latency
	}
	return buckets
}

func modelStatusLabel(status string) string {
	switch status {
	case model.ModelStatusOperational:
		return "正常"
	case model.ModelStatusDegraded:
		return "繁忙"
	case model.ModelStatusUnavailable:
		return "不可用"
	default:
		return "暂无数据"
	}
}