package service

import (
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const defaultPromptSyncCron = "*/5 * * * *"

var (
	promptSyncCron *cron.Cron
	promptSyncOnce sync.Once
	promptSyncMu   sync.Mutex
)

func StartPromptSyncScheduler() {
	promptSyncOnce.Do(func() {
		promptSyncCron = cron.New()
		promptSyncCron.Start()
		logWorkerInfo("prompt_sync", "scheduler_started")
	})
	RefreshPromptSyncScheduler()
}

func RefreshPromptSyncScheduler() {
	promptSyncMu.Lock()
	defer promptSyncMu.Unlock()
	if promptSyncCron == nil {
		return
	}
	for _, entry := range promptSyncCron.Entries() {
		promptSyncCron.Remove(entry.ID)
	}
	settings, err := repository.GetSettings()
	if err != nil {
		logWorkerError("prompt_sync", "settings_load_failed", err)
		return
	}
	setting := normalizePromptSyncSetting(settings.Private.PromptSync)
	if setting.Enabled == nil || !*setting.Enabled {
		return
	}
	if _, err := promptSyncCron.AddFunc(setting.Cron, SyncRemotePromptCategories); err != nil {
		logWorkerError("prompt_sync", "schedule_update_failed", err, "cron", setting.Cron)
		return
	}
	logWorkerInfo("prompt_sync", "schedule_updated", "cron", setting.Cron)
}

func SyncRemotePromptCategories() {
	for _, category := range repository.PromptCategories() {
		if !category.Remote {
			continue
		}
		startedAt := time.Now()
		logWorkerInfo("prompt_sync", "category_sync_started", "category", category.Category)
		if _, err := SyncPromptCategory(category.Category); err != nil {
			logWorkerError("prompt_sync", "category_sync_failed", err, "category", category.Category, "duration_ms", time.Since(startedAt).Milliseconds())
			continue
		}
		logWorkerInfo("prompt_sync", "category_sync_completed", "category", category.Category, "duration_ms", time.Since(startedAt).Milliseconds())
	}
}

func normalizePromptSyncSetting(setting model.PromptSyncSetting) model.PromptSyncSetting {
	if setting.Cron == "" {
		setting.Cron = defaultPromptSyncCron
	}
	if setting.Enabled == nil {
		enabled := true
		setting.Enabled = &enabled
	}
	return setting
}
