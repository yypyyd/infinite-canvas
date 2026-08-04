package repository

import (
	"context"
	"errors"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DataConsistencySnapshot struct {
	Organizations []model.Organization
	Users         []model.User
	Files         []model.UserFile
	References    []model.UserFileReference
	Reservations  []model.UserFileUploadReservation
	Deletions     []model.UserObjectDeletion
	GenerationRecords []model.UserGenerationRecord
	GenerationTasks []model.GenerationTask
	CreditLogs    []model.CreditLog
	BatchItems    []model.BatchProductionItem
}

func CheckDatabase(ctx context.Context) error {
	db, err := DB()
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func GetOperationsQueueHealth(timestamp string) (model.OperationsQueueHealth, error) {
	db, err := DB()
	if err != nil {
		return model.OperationsQueueHealth{}, err
	}
	var result model.OperationsQueueHealth
	counts := []struct {
		target *int64
		query  any
		where  string
		args   []any
	}{
		{&result.BatchQueued, &model.BatchProductionItem{}, "status = ?", []any{model.BatchProductionStatusQueued}},
		{&result.BatchRunning, &model.BatchProductionItem{}, "status = ?", []any{model.BatchProductionStatusRunning}},
		{&result.BatchExpiredLeases, &model.BatchProductionItem{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{model.BatchProductionStatusRunning, timestamp}},
		{&result.EmailPending, &model.OrganizationEmailOutbox{}, "status = ?", []any{"pending"}},
		{&result.EmailFailed, &model.OrganizationEmailOutbox{}, "status = ?", []any{"failed"}},
		{&result.EmailExpiredLeases, &model.OrganizationEmailOutbox{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{"processing", timestamp}},
		{&result.ObjectDeletionPending, &model.UserObjectDeletion{}, "status = ?", []any{"pending"}},
		{&result.ObjectDeletionFailed, &model.UserObjectDeletion{}, "status = ?", []any{"failed"}},
		{&result.ObjectDeletionExpiredLeases, &model.UserObjectDeletion{}, "status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?", []any{"processing", timestamp}},
	}
	for _, count := range counts {
		if err := db.Model(count.query).Where(count.where, count.args...).Count(count.target).Error; err != nil {
			return model.OperationsQueueHealth{}, err
		}
	}
	return result, nil
}

func GetOperationsGenerationMetrics(since string) (model.OperationsGenerationMetrics, []int64, error) {
	db, err := DB()
	if err != nil {
		return model.OperationsGenerationMetrics{}, nil, err
	}
	var counts []struct {
		Status model.GenerationTaskStatus
		Value  int64
	}
	if err := db.Model(&model.GenerationTask{}).Where("created_at >= ?", since).Select("status, COUNT(*) AS value").Group("status").Scan(&counts).Error; err != nil {
		return model.OperationsGenerationMetrics{}, nil, err
	}
	var metrics model.OperationsGenerationMetrics
	for _, count := range counts {
		metrics.Total += count.Value
		switch count.Status {
		case model.GenerationTaskStatusRunning:
			metrics.Running = count.Value
		case model.GenerationTaskStatusSuccess:
			metrics.Success = count.Value
		case model.GenerationTaskStatusFailed:
			metrics.Failed = count.Value
		}
	}
	var durations []int64
	err = db.Model(&model.GenerationTask{}).
		Where("created_at >= ? AND status IN ?", since, []model.GenerationTaskStatus{model.GenerationTaskStatusSuccess, model.GenerationTaskStatusFailed}).
		Pluck("duration_ms", &durations).Error
	return metrics, durations, err
}

func GetDataConsistencySnapshot() (DataConsistencySnapshot, error) {
	db, err := DB()
	if err != nil { return DataConsistencySnapshot{}, err }
	snapshot := DataConsistencySnapshot{}
	queries := []*gorm.DB{
		db.Select("id, credits").Find(&snapshot.Organizations),
		db.Select("id, credits").Find(&snapshot.Users),
		db.Find(&snapshot.Files),
		db.Find(&snapshot.References),
		db.Select("id, organization_id, user_id, storage_key, object_key, mime_type, size, reserved_bytes, expires_at, created_at").Find(&snapshot.Reservations),
		db.Select("id, organization_id, object_key, size, status").Find(&snapshot.Deletions),
		db.Select("id, organization_id, user_id, deleted_at").Find(&snapshot.GenerationRecords),
		db.Select("id, organization_id, user_id, credits, credit_source, status, created_at").Find(&snapshot.GenerationTasks),
		db.Select("id, organization_id, user_id, credit_source, type, amount, balance, related_id, created_at").Order("created_at asc, id asc").Find(&snapshot.CreditLogs),
		db.Where("status = ? OR result_storage_key <> ''", model.BatchProductionStatusCompleted).Find(&snapshot.BatchItems),
	}
	for _, query := range queries { if query.Error != nil { return DataConsistencySnapshot{}, query.Error } }
	return snapshot, nil
}

func RepairDanglingFileReference(id string) (bool, error) {
	db, err := DB()
	if err != nil { return false, err }
	repaired := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var reference model.UserFileReference
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&reference, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
			return err
		}
		var files int64
		if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND storage_key = ?", reference.OrganizationID, reference.StorageKey).Count(&files).Error; err != nil { return err }
		if files > 0 { return nil }
		result := tx.Where("id = ? AND organization_id = ? AND storage_key = ?", reference.ID, reference.OrganizationID, reference.StorageKey).Delete(&model.UserFileReference{})
		if result.Error != nil { return result.Error }
		repaired = result.RowsAffected == 1
		return nil
	})
	return repaired, err
}

func RepairUserFileReferenceState(id string, timestamp string) (bool, error) {
	db, err := DB()
	if err != nil { return false, err }
	repaired := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var file model.UserFile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&file, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
			return err
		}
		var references int64
		if err := tx.Model(&model.UserFileReference{}).Where("organization_id = ? AND storage_key = ?", file.OrganizationID, file.StorageKey).Count(&references).Error; err != nil { return err }
		expected := timestamp
		if references > 0 { expected = "" }
		if file.UnreferencedAt == expected || (references == 0 && file.UnreferencedAt != "") { return nil }
		result := tx.Model(&model.UserFile{}).Where("id = ? AND organization_id = ?", file.ID, file.OrganizationID).Updates(map[string]any{"unreferenced_at": expected, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		repaired = result.RowsAffected == 1
		return nil
	})
	return repaired, err
}

func RepairBatchProductionResultReference(id string, timestamp string) (bool, error) {
	db, err := DB()
	if err != nil { return false, err }
	repaired := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var item model.BatchProductionItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
			return err
		}
		if item.Status != model.BatchProductionStatusCompleted || item.ResultStorageKey == "" { return nil }
		var file model.UserFile
		if err := tx.Where("organization_id = ? AND storage_key = ?", item.OrganizationID, item.ResultStorageKey).First(&file).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) { return nil }
			return err
		}
		var references, validReferences int64
		if err := tx.Model(&model.UserFileReference{}).Where("organization_id = ? AND domain = ? AND object_id = ?", item.OrganizationID, "batch_result", item.ID).Count(&references).Error; err != nil { return err }
		if err := tx.Model(&model.UserFileReference{}).Where("organization_id = ? AND domain = ? AND object_id = ? AND storage_key = ?", item.OrganizationID, "batch_result", item.ID, item.ResultStorageKey).Count(&validReferences).Error; err != nil { return err }
		if references == 1 && validReferences == 1 { return nil }
		if err := replaceUserFileReferences(tx, item.OrganizationID, "batch_result", item.ID, "batch-result-"+item.ID, []string{item.ResultStorageKey}, false, timestamp); err != nil { return err }
		repaired = true
		return nil
	})
	return repaired, err
}
