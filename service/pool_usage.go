package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// poolEventSource reads pool events; replaced in tests.
type poolEventSource interface {
	UsageEvents(ctx context.Context, since time.Time) ([]gateway.PoolEvent, error)
	EventsFor(ctx context.Context, requestIDs, eventIDs []string) ([]gateway.PoolEvent, error)
}

var currentPoolEvents = func() poolEventSource {
	if m := gateway.Current(); m != nil {
		return m
	}
	return nil
}

const poolUsageTaskLimit = 50000

// PoolUsageRow aggregates pool generations for one account or one model.
// Credits come from 幻图's own generation tasks (the only billing), linked to
// pool events by request ID.
type PoolUsageRow struct {
	Key             string `json:"key"`
	Label           string `json:"label"`
	Provider        string `json:"provider,omitempty"`
	Requests        int    `json:"requests"`
	Succeeded       int    `json:"succeeded"`
	Failed          int    `json:"failed"`
	Pending         int    `json:"pending"`
	ChargedCredits  int    `json:"chargedCredits"`
	RefundedCredits int    `json:"refundedCredits"`
	HeldCredits     int    `json:"heldCredits"`
	// Providers breaks a model row down per provider route (model rows only).
	Providers []PoolUsageRow `json:"providers,omitempty"`
}

type PoolUsageReport struct {
	Since        string         `json:"since"`
	Days         int            `json:"days"`
	Accounts     []PoolUsageRow `json:"accounts"`
	Models       []PoolUsageRow `json:"models"`
	Unlinked     int            `json:"unlinked"`     // pool events with no 幻图 task (e.g. admin tests)
	Unattributed PoolUsageRow   `json:"unattributed"` // 幻图 pool tasks the pool has no record of
}

func poolTaskCredits(row *PoolUsageRow, task model.GenerationTask) {
	switch task.Status {
	case model.GenerationTaskStatusSuccess:
		row.ChargedCredits += task.Credits
	case model.GenerationTaskStatusFailed:
		row.RefundedCredits += task.Credits
	default:
		row.HeldCredits += task.Credits
	}
}

func poolEventCount(row *PoolUsageRow, status string) {
	row.Requests++
	switch status {
	case "success":
		row.Succeeded++
	case "pending":
		row.Pending++
	default:
		row.Failed++
	}
}

// BuildPoolUsage reports per-account and per-model pool usage for the last
// days days, joined with 幻图 credits by request ID.
func BuildPoolUsage(ctx context.Context, days int) (PoolUsageReport, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	report := PoolUsageReport{Since: since.Format(timestampLayout), Days: days, Accounts: []PoolUsageRow{}, Models: []PoolUsageRow{}}
	source := currentPoolEvents()
	if source == nil {
		return report, nil
	}
	events, err := source.UsageEvents(ctx, since)
	if err != nil {
		return report, err
	}
	tasks, err := repository.ListGenerationTasksByChannelSince(BuiltinPoolChannelName, report.Since, poolUsageTaskLimit)
	if err != nil {
		return report, err
	}
	byRequest := make(map[string]model.GenerationTask, len(tasks))
	byUpstream := make(map[string]model.GenerationTask)
	for _, task := range tasks {
		byRequest[task.RequestID] = task
		if task.UpstreamTaskID != "" {
			byUpstream[task.UpstreamTaskID] = task
		}
	}
	accounts := map[string]*PoolUsageRow{}
	models := map[string]*PoolUsageRow{}
	providerRows := map[string]map[string]*PoolUsageRow{}
	linked := map[string]bool{}
	for _, event := range events {
		task, ok := byRequest[event.RequestID]
		if !ok || event.RequestID == "" {
			task, ok = byUpstream[event.ID]
		}
		// A request retried inside the pool can leave several events; credit the
		// 幻图 task once, on its first (newest) event.
		creditable := ok && !linked[task.ID]
		if ok {
			linked[task.ID] = true
		} else {
			report.Unlinked++
		}
		accountKey := event.AccountID
		if accountKey == "" {
			accountKey = "-"
		}
		account := accounts[accountKey]
		if account == nil {
			label := event.AccountEmail
			if label == "" {
				label = event.AccountID
			}
			if label == "" {
				label = "未分配账号"
			}
			account = &PoolUsageRow{Key: accountKey, Label: label, Provider: event.Provider}
			accounts[accountKey] = account
		}
		modelID := event.Model
		if unified, legacy := unifiedPoolModelID(modelID); legacy {
			modelID = unified
		}
		// A request made under a public alias is reported under that name.
		if ok && task.Model != "" && task.UpstreamModel != "" && task.Model != task.UpstreamModel {
			modelID = task.Model
		}
		modelRow := models[modelID]
		if modelRow == nil {
			modelRow = &PoolUsageRow{Key: modelID, Label: modelID}
			models[modelID] = modelRow
			providerRows[modelID] = map[string]*PoolUsageRow{}
		}
		providerKey := event.Provider
		if providerKey == "" {
			providerKey = "-"
		}
		providerRow := providerRows[modelID][providerKey]
		if providerRow == nil {
			providerRow = &PoolUsageRow{Key: poolRouteKey(event.Kind, modelID, providerKey), Label: providerKey, Provider: event.Provider}
			providerRows[modelID][providerKey] = providerRow
		}
		poolEventCount(account, event.Status)
		poolEventCount(modelRow, event.Status)
		poolEventCount(providerRow, event.Status)
		if creditable {
			poolTaskCredits(account, task)
			poolTaskCredits(modelRow, task)
			poolTaskCredits(providerRow, task)
		}
	}
	report.Unattributed = PoolUsageRow{Key: "unattributed", Label: "池内无记录"}
	for _, task := range tasks {
		if !linked[task.ID] {
			report.Unattributed.Requests++
			poolTaskCredits(&report.Unattributed, task)
		}
	}
	report.Accounts = sortedUsageRows(accounts)
	for id, row := range models {
		row.Providers = sortedUsageRows(providerRows[id])
	}
	report.Models = sortedUsageRows(models)
	return report, nil
}

// poolRouteKey mirrors the gateway route ID convention (<kind>.<model>.<provider>).
func poolRouteKey(kind, modelID, provider string) string {
	if kind == "" {
		return modelID + "." + provider
	}
	return kind + "." + modelID + "." + provider
}

func sortedUsageRows(rows map[string]*PoolUsageRow) []PoolUsageRow {
	items := make([]PoolUsageRow, 0, len(rows))
	for _, row := range rows {
		items = append(items, *row)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ChargedCredits != items[j].ChargedCredits {
			return items[i].ChargedCredits > items[j].ChargedCredits
		}
		if items[i].Requests != items[j].Requests {
			return items[i].Requests > items[j].Requests
		}
		return items[i].Key < items[j].Key
	})
	return items
}

// attachPoolAccounts fills PoolAccount on pool-served tasks for admin views.
func attachPoolAccounts(ctx context.Context, tasks []model.GenerationTask) {
	source := currentPoolEvents()
	if source == nil {
		return
	}
	var requestIDs, eventIDs []string
	for _, task := range tasks {
		if IsBuiltinPoolTask(task) {
			requestIDs = append(requestIDs, task.RequestID)
			if task.UpstreamTaskID != "" {
				eventIDs = append(eventIDs, task.UpstreamTaskID)
			}
		}
	}
	if len(requestIDs) == 0 {
		return
	}
	events, err := source.EventsFor(ctx, requestIDs, eventIDs)
	if err != nil {
		return
	}
	byRequest := map[string]gateway.PoolEvent{}
	byID := map[string]gateway.PoolEvent{}
	for _, event := range events {
		// Prefer the event that actually used an account (latest attempt).
		if current, ok := byRequest[event.RequestID]; !ok || poolEventPreferred(event, current) {
			byRequest[event.RequestID] = event
		}
		byID[event.ID] = event
	}
	for i := range tasks {
		if !IsBuiltinPoolTask(tasks[i]) {
			continue
		}
		event, ok := byRequest[tasks[i].RequestID]
		if !ok {
			event, ok = byID[tasks[i].UpstreamTaskID]
		}
		if ok {
			tasks[i].PoolAccount = poolAccountLabel(event)
		}
	}
}

func poolAccountLabel(event gateway.PoolEvent) string {
	label := strings.TrimSpace(event.AccountEmail)
	if label == "" {
		label = strings.TrimSpace(event.AccountID)
	}
	if label == "" {
		return "未分配账号"
	}
	if event.Provider != "" {
		return event.Provider + " · " + label
	}
	return label
}

// poolEventPreferred picks the attempt that used an account, newest first.
func poolEventPreferred(candidate, current gateway.PoolEvent) bool {
	if (candidate.AccountID != "") != (current.AccountID != "") {
		return candidate.AccountID != ""
	}
	return candidate.TS.After(current.TS)
}
