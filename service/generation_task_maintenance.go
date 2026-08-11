package service

import (
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const generationTaskTimeout = 30 * time.Minute

func StartGenerationTaskMaintenanceWorker() {
	logWorkerInfo("generation_task", "worker_started", "timeout_minutes", int(generationTaskTimeout/time.Minute))
	go func() {
		for {
			expireStaleGenerationTasks(time.Now().UTC())
			time.Sleep(time.Minute)
		}
	}()
}

func expireStaleGenerationTasks(timestamp time.Time) {
	tasks, err := repository.ListStaleRunningGenerationTasks(timestamp.Add(-generationTaskTimeout).Format(timestampLayout), 100)
	if err != nil {
		logWorkerError("generation_task", "stale_list_failed", err)
		return
	}
	for _, task := range tasks {
		if err := FinishGenerationTask(task, model.GenerationTaskStatusFailed, "生成任务超过 30 分钟未完成，已自动结束"); err != nil {
			logWorkerError("generation_task", "stale_finalize_failed", err, "task_id", task.ID)
			continue
		}
		logWorkerInfo("generation_task", "stale_finalized", "task_id", task.ID, "modality", task.Modality)
	}
}
