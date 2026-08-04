package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const maxDataConsistencyIssues = 1000

type dataConsistencyObject struct {
	Key, Hash, MimeType string
	Size                int64
}

type RuntimeHealth struct {
	Status   string `json:"status"`
	Database string `json:"database"`
}

type OperationsHealth struct {
	RuntimeHealth
	Queues     model.OperationsQueueHealth       `json:"queues"`
	Generation model.OperationsGenerationMetrics `json:"generation"`
	Alerts     []model.OperationsAlert           `json:"alerts"`
	CheckedAt  string                            `json:"checkedAt"`
}

func CheckReadiness(ctx context.Context) (RuntimeHealth, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := repository.CheckDatabase(ctx); err != nil {
		return RuntimeHealth{Status: "unavailable", Database: "unavailable"}, err
	}
	return RuntimeHealth{Status: "ok", Database: "ok"}, nil
}

func GetOperationsHealth(ctx context.Context) (OperationsHealth, error) {
	runtime, err := CheckReadiness(ctx)
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime}, err
	}
	timestamp := now()
	queues, err := repository.GetOperationsQueueHealth(timestamp)
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, CheckedAt: timestamp}, err
	}
	generation, durations, err := repository.GetOperationsGenerationMetrics(time.Now().UTC().Add(-24 * time.Hour).Format(timestampLayout))
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, Queues: queues, CheckedAt: timestamp}, err
	}
	generation = finalizeOperationsGenerationMetrics(generation, durations)
	settings, err := repository.GetSettings()
	if err != nil {
		return OperationsHealth{RuntimeHealth: runtime, Queues: queues, Generation: generation, CheckedAt: timestamp}, err
	}
	alerts := operationsAlerts(queues, normalizePrivateSetting(settings.Private).OperationsAlerts)
	status := "ok"
	if len(alerts) > 0 {
		status = "degraded"
	}
	return OperationsHealth{RuntimeHealth: RuntimeHealth{Status: status, Database: runtime.Database}, Queues: queues, Generation: generation, Alerts: alerts, CheckedAt: timestamp}, nil
}

func finalizeOperationsGenerationMetrics(metrics model.OperationsGenerationMetrics, durations []int64) model.OperationsGenerationMetrics {
	metrics.WindowHours = 24
	terminal := metrics.Success + metrics.Failed
	if terminal > 0 {
		metrics.SuccessRate = math.Round(float64(metrics.Success)*10000/float64(terminal)) / 100
	}
	if len(durations) == 0 {
		return metrics
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	var total int64
	for _, duration := range durations {
		total += duration
	}
	metrics.AverageDurationMs = int64(math.Round(float64(total) / float64(len(durations))))
	metrics.P95DurationMs = durations[int(math.Ceil(float64(len(durations))*0.95))-1]
	return metrics
}

func operationsAlerts(queues model.OperationsQueueHealth, setting model.OperationsAlertSetting) []model.OperationsAlert {
	if setting.Enabled != nil && !*setting.Enabled {
		return []model.OperationsAlert{}
	}
	checks := []struct {
		key       string
		value     int64
		threshold *int64
	}{
		{"batch_queue_backlog", queues.BatchQueued, setting.BatchQueuedThreshold},
		{"batch_expired_leases", queues.BatchExpiredLeases, setting.BatchExpiredLeasesThreshold},
		{"email_outbox_pending", queues.EmailPending, setting.EmailPendingThreshold},
		{"email_outbox_failed", queues.EmailFailed, setting.EmailFailedThreshold},
		{"email_outbox_expired_leases", queues.EmailExpiredLeases, setting.EmailExpiredLeasesThreshold},
		{"object_deletion_outbox_pending", queues.ObjectDeletionPending, setting.ObjectDeletionPendingThreshold},
		{"object_deletion_outbox_failed", queues.ObjectDeletionFailed, setting.ObjectDeletionFailedThreshold},
		{"object_deletion_outbox_expired_leases", queues.ObjectDeletionExpiredLeases, setting.ObjectDeletionExpiredLeasesThreshold},
	}
	alerts := make([]model.OperationsAlert, 0)
	for _, check := range checks {
		if check.threshold != nil && *check.threshold > 0 && check.value >= *check.threshold {
			alerts = append(alerts, model.OperationsAlert{Key: check.key, Value: check.value, Threshold: *check.threshold})
		}
	}
	return alerts
}

func InspectDataConsistency(ctx context.Context) (model.DataConsistencyReport, error) {
	snapshot, err := repository.GetDataConsistencySnapshot()
	if err != nil {
		return model.DataConsistencyReport{}, err
	}
	objects, storageStatus := listDataConsistencyObjects(ctx)
	return inspectDataConsistencySnapshot(snapshot, objects, storageStatus), nil
}

func listDataConsistencyObjects(ctx context.Context) ([]dataConsistencyObject, string) {
	if strings.TrimSpace(config.Cfg.QiniuAccessKey) == "" || strings.TrimSpace(config.Cfg.QiniuSecretKey) == "" || strings.TrimSpace(config.Cfg.QiniuBucket) == "" {
		return nil, "unconfigured"
	}
	manager, marker := qiniuBucketManager(), ""
	objects := []dataConsistencyObject{}
	for {
		if ctx.Err() != nil {
			return nil, "error"
		}
		items, _, nextMarker, hasNext, err := manager.ListFiles(config.Cfg.QiniuBucket, "organizations/", "", marker, 1000)
		if err != nil {
			return nil, "error"
		}
		for _, item := range items {
			objects = append(objects, dataConsistencyObject{Key: item.Key, Hash: item.Hash, MimeType: item.MimeType, Size: item.Fsize})
		}
		if !hasNext {
			return objects, "ok"
		}
		if nextMarker == "" || nextMarker == marker {
			return nil, "error"
		}
		marker = nextMarker
	}
}

func inspectDataConsistencySnapshot(snapshot repository.DataConsistencySnapshot, objects []dataConsistencyObject, storageStatus string) model.DataConsistencyReport {
	report := model.DataConsistencyReport{CheckedAt: now(), StorageStatus: storageStatus, Summary: map[string]int{"media_reference": 0, "object_storage": 0, "generation_record": 0, "batch_result": 0, "credit_ledger": 0}, Issues: []model.DataConsistencyIssue{}}
	add := func(issue model.DataConsistencyIssue) {
		issue.ID = dataConsistencyIssueID(issue.Code, issue.Target)
		report.TotalIssues++
		report.Summary[issue.Category]++
		if issue.RepairAction != "" {
			report.Repairable++
		}
		if len(report.Issues) < maxDataConsistencyIssues {
			report.Issues = append(report.Issues, issue)
		} else {
			report.Truncated = true
		}
	}
	organizations, organizationCredits, users := map[string]bool{}, map[string]int{}, map[string]model.User{}
	for _, item := range snapshot.Organizations {
		organizations[item.ID], organizationCredits[item.ID] = true, item.Credits
	}
	for _, item := range snapshot.Users {
		users[item.ID] = item
	}
	fileKey := func(organizationID, storageKey string) string { return organizationID + "\x00" + storageKey }
	files, filesByObject, referenceCount := map[string]model.UserFile{}, map[string]model.UserFile{}, map[string]int{}
	batchReferences := map[string][]model.UserFileReference{}
	for _, file := range snapshot.Files {
		files[fileKey(file.OrganizationID, file.StorageKey)] = file
		filesByObject[file.ObjectKey] = file
		if file.Size <= 0 {
			add(model.DataConsistencyIssue{Category: "media_reference", Code: "invalid_file_size", Severity: "error", OrganizationID: file.OrganizationID, ResourceType: "user_file", ResourceID: file.ID, Message: "数据库媒体文件大小无效", Target: file.ID})
		}
	}
	for _, reference := range snapshot.References {
		key := fileKey(reference.OrganizationID, reference.StorageKey)
		referenceCount[key]++
		if reference.Domain == "batch_result" {
			batchReferences[reference.ObjectID] = append(batchReferences[reference.ObjectID], reference)
		}
		if _, ok := files[key]; !ok {
			add(model.DataConsistencyIssue{Category: "media_reference", Code: "dangling_file_reference", Severity: "error", OrganizationID: reference.OrganizationID, ResourceType: reference.Domain, ResourceID: reference.ObjectID, Message: "媒体引用指向不存在的数据库文件", RepairAction: "delete_dangling_reference", Target: reference.ID})
		}
	}
	for key, file := range files {
		hasReferences := referenceCount[key] > 0
		if (hasReferences && file.UnreferencedAt != "") || (!hasReferences && file.UnreferencedAt == "") {
			message := "无引用文件尚未进入回收宽限期"
			if hasReferences {
				message = "仍被使用的文件被错误标记为待回收"
			}
			add(model.DataConsistencyIssue{Category: "media_reference", Code: "file_reference_state_mismatch", Severity: "warning", OrganizationID: file.OrganizationID, ResourceType: "user_file", ResourceID: file.ID, Message: message, RepairAction: "recalculate_file_reference_state", Target: file.ID})
		}
	}
	trackedObjects := map[string]bool{}
	for key := range filesByObject {
		trackedObjects[key] = true
	}
	for _, item := range snapshot.Reservations {
		trackedObjects[item.ObjectKey] = true
		if item.ReservedBytes < 0 || item.Size <= 0 {
			add(model.DataConsistencyIssue{Category: "media_reference", Code: "invalid_upload_reservation", Severity: "error", OrganizationID: item.OrganizationID, ResourceType: "upload_reservation", ResourceID: item.ID, Message: "上传预留的大小或占用字节无效", Target: item.ID})
		}
	}
	for _, item := range snapshot.Deletions {
		trackedObjects[item.ObjectKey] = true
	}
	if storageStatus == "error" {
		add(model.DataConsistencyIssue{Category: "object_storage", Code: "object_scan_failed", Severity: "error", ResourceType: "qiniu_bucket", ResourceID: "current", Message: "七牛对象列举失败，本次未完成对象一致性检查", Target: "qiniu"})
	}
	if storageStatus == "ok" {
		seen := map[string]bool{}
		for _, object := range objects {
			seen[object.Key] = true
			if file, ok := filesByObject[object.Key]; ok {
				if file.Size != object.Size || (file.Hash != "" && file.Hash != object.Hash) || assetTypeFromMime(file.MimeType) != assetTypeFromMime(object.MimeType) {
					add(model.DataConsistencyIssue{Category: "object_storage", Code: "object_metadata_mismatch", Severity: "error", OrganizationID: file.OrganizationID, ResourceType: "user_file", ResourceID: file.ID, Message: "七牛对象与数据库记录的大小、哈希或媒体类型不一致", Target: file.ID})
				}
				continue
			}
			if trackedObjects[object.Key] {
				continue
			}
			parts := strings.Split(object.Key, "/")
			organizationID := ""
			if len(parts) > 2 && parts[0] == "organizations" {
				organizationID = parts[1]
			}
			message := "企业目录中存在未被数据库登记的七牛对象"
			if !organizations[organizationID] {
				message = "七牛对象所属企业不存在"
			}
			add(model.DataConsistencyIssue{Category: "object_storage", Code: "orphan_object", Severity: "warning", OrganizationID: organizationID, ResourceType: "qiniu_object", ResourceID: dataConsistencyObjectID(object.Key), Message: message, Target: object.Key})
		}
		for objectKey, file := range filesByObject {
			if !seen[objectKey] {
				add(model.DataConsistencyIssue{Category: "object_storage", Code: "missing_object", Severity: "error", OrganizationID: file.OrganizationID, ResourceType: "user_file", ResourceID: file.ID, Message: "数据库媒体文件对应的七牛对象不存在", Target: file.ID})
			}
		}
	}
	logsByTask, latestBalance, latestOrganizationBalance := map[string][]model.CreditLog{}, map[string]int{}, map[string]int{}
	for _, log := range snapshot.CreditLogs {
		if log.RelatedID != "" {
			logsByTask[log.RelatedID] = append(logsByTask[log.RelatedID], log)
		}
		if log.CreditSource == model.CreditSourceOrganization {
			latestOrganizationBalance[log.OrganizationID] = log.Balance
			if !organizations[log.OrganizationID] {
				add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "credit_log_organization_missing", Severity: "error", OrganizationID: log.OrganizationID, ResourceType: "credit_log", ResourceID: log.ID, Message: "企业算力流水所属企业不存在", Target: log.ID})
			}
		} else {
			latestBalance[log.UserID] = log.Balance
		}
		if _, ok := users[log.UserID]; !ok {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "credit_log_user_missing", Severity: "error", ResourceType: "credit_log", ResourceID: log.ID, Message: "算力流水所属用户不存在", Target: log.ID})
		}
	}
	for _, user := range users {
		if balance, ok := latestBalance[user.ID]; ok && balance != user.Credits {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "credit_balance_mismatch", Severity: "error", ResourceType: "user", ResourceID: user.ID, Message: "用户当前算力与最后一笔账本余额不一致", Target: user.ID})
		}
	}
	for organizationID, credits := range organizationCredits {
		if balance, ok := latestOrganizationBalance[organizationID]; ok && balance != credits {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "organization_credit_balance_mismatch", Severity: "error", OrganizationID: organizationID, ResourceType: "organization", ResourceID: organizationID, Message: "企业共享算力与最后一笔企业账本余额不一致", Target: organizationID})
		}
	}
	for _, record := range snapshot.GenerationRecords {
		if !organizations[record.OrganizationID] || users[record.UserID].ID == "" {
			add(model.DataConsistencyIssue{Category: "generation_record", Code: "generation_record_owner_missing", Severity: "error", OrganizationID: record.OrganizationID, ResourceType: "generation_record", ResourceID: record.ID, Message: "生成记录所属企业或用户不存在", Target: record.OrganizationID + "\x00" + record.ID})
		}
	}
	for _, task := range snapshot.GenerationTasks {
		if !organizations[task.OrganizationID] || users[task.UserID].ID == "" {
			add(model.DataConsistencyIssue{Category: "generation_record", Code: "generation_owner_missing", Severity: "error", OrganizationID: task.OrganizationID, ResourceType: "generation_task", ResourceID: task.ID, Message: "生成任务所属企业或用户不存在", Target: task.ID})
		}
		if task.Credits <= 0 {
			continue
		}
		consumeCount, refundCount, consumed, refunded, sourceMismatch := 0, 0, 0, 0, false
		taskSource := task.CreditSource
		if taskSource == "" {
			taskSource = model.CreditSourcePersonal
		}
		for _, log := range logsByTask[task.ID] {
			logSource := log.CreditSource
			if logSource == "" {
				logSource = model.CreditSourcePersonal
			}
			if (log.Type == model.CreditLogTypeAIConsume || log.Type == model.CreditLogTypeAIRefund) && logSource != taskSource {
				sourceMismatch = true
			}
			switch log.Type {
			case model.CreditLogTypeAIConsume:
				consumeCount++
				consumed += -log.Amount
			case model.CreditLogTypeAIRefund:
				refundCount++
				refunded += log.Amount
			}
		}
		if sourceMismatch {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "generation_credit_source_mismatch", Severity: "error", OrganizationID: task.OrganizationID, ResourceType: "generation_task", ResourceID: task.ID, Message: "生成任务与消费或退款流水的扣费来源不一致", Target: task.ID})
		}
		if consumeCount != 1 || consumed != task.Credits {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "generation_charge_mismatch", Severity: "error", OrganizationID: task.OrganizationID, ResourceType: "generation_task", ResourceID: task.ID, Message: "生成任务的消费流水数量或金额不一致", Target: task.ID})
		}
		if task.Status == model.GenerationTaskStatusFailed && (refundCount != 1 || refunded != task.Credits) {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "generation_refund_mismatch", Severity: "error", OrganizationID: task.OrganizationID, ResourceType: "generation_task", ResourceID: task.ID, Message: "失败生成任务的退款流水数量或金额不一致", Target: task.ID})
		}
		if task.Status != model.GenerationTaskStatusFailed && refundCount > 0 {
			add(model.DataConsistencyIssue{Category: "credit_ledger", Code: "unexpected_generation_refund", Severity: "error", OrganizationID: task.OrganizationID, ResourceType: "generation_task", ResourceID: task.ID, Message: "未失败的生成任务存在退款流水", Target: task.ID})
		}
	}
	for _, item := range snapshot.BatchItems {
		if item.Status != model.BatchProductionStatusCompleted {
			if item.ResultStorageKey != "" {
				add(model.DataConsistencyIssue{Category: "batch_result", Code: "unfinished_batch_has_result", Severity: "error", OrganizationID: item.OrganizationID, ResourceType: "batch_item", ResourceID: item.ID, Message: "未完成的批量任务项仍保存结果文件编号", Target: item.ID})
			}
			continue
		}
		if item.ResultStorageKey == "" {
			add(model.DataConsistencyIssue{Category: "batch_result", Code: "completed_batch_result_missing", Severity: "error", OrganizationID: item.OrganizationID, ResourceType: "batch_item", ResourceID: item.ID, Message: "已完成的批量任务项缺少结果文件编号", Target: item.ID})
			continue
		}
		file, ok := files[fileKey(item.OrganizationID, item.ResultStorageKey)]
		if !ok {
			add(model.DataConsistencyIssue{Category: "batch_result", Code: "batch_result_file_missing", Severity: "error", OrganizationID: item.OrganizationID, ResourceType: "batch_item", ResourceID: item.ID, Message: "批量结果对应的数据库媒体文件不存在", Target: item.ID})
			continue
		}
		references := batchReferences[item.ID]
		validReferences := 0
		for _, reference := range references {
			if reference.OrganizationID == item.OrganizationID && reference.StorageKey == file.StorageKey {
				validReferences++
			}
		}
		if validReferences != 1 || len(references) != 1 {
			add(model.DataConsistencyIssue{Category: "batch_result", Code: "batch_result_reference_mismatch", Severity: "warning", OrganizationID: item.OrganizationID, ResourceType: "batch_item", ResourceID: item.ID, Message: "批量结果的事务型文件引用缺失或不唯一", RepairAction: "rebuild_batch_result_reference", Target: item.ID})
		}
	}
	return report
}

func RepairDataConsistencyIssue(ctx context.Context, input model.RepairDataConsistencyInput) (bool, error) {
	input.IssueID = strings.TrimSpace(input.IssueID)
	if input.IssueID == "" || len(input.IssueID) > 64 {
		return false, safeMessageError{message: "巡检问题编号无效"}
	}
	report, err := InspectDataConsistency(ctx)
	if err != nil {
		return false, err
	}
	var issue *model.DataConsistencyIssue
	for index := range report.Issues {
		if report.Issues[index].ID == input.IssueID {
			issue = &report.Issues[index]
			break
		}
	}
	if issue == nil || issue.RepairAction == "" {
		return false, safeMessageError{message: "问题已变化或不支持自动修复，请重新巡检"}
	}
	var repaired bool
	switch issue.RepairAction {
	case "delete_dangling_reference":
		repaired, err = repository.RepairDanglingFileReference(issue.Target)
	case "recalculate_file_reference_state":
		repaired, err = repository.RepairUserFileReferenceState(issue.Target, now())
	case "rebuild_batch_result_reference":
		repaired, err = repository.RepairBatchProductionResultReference(issue.Target, now())
	default:
		return false, safeMessageError{message: "该问题不支持自动修复"}
	}
	if err != nil {
		return false, err
	}
	if !repaired {
		return false, safeMessageError{message: "问题状态已变化，请重新巡检"}
	}
	return true, nil
}

func dataConsistencyIssueID(code, target string) string {
	sum := sha256.Sum256([]byte(code + "\x00" + target))
	return hex.EncodeToString(sum[:16])
}

func dataConsistencyObjectID(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:6])
}
