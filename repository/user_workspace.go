package repository

import (
	"errors"
	"strconv"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrWorkspaceVersionConflict = errors.New("workspace version conflict")
var ErrWorkspaceFileMissing = errors.New("workspace file missing")
var ErrWorkspaceStorageQuotaExceeded = errors.New("workspace storage quota exceeded")
var ErrWorkspaceUploadReservationUnavailable = errors.New("workspace upload reservation unavailable")

func GetUserWorkspaceState(organizationID string) (model.UserWorkspaceState, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserWorkspaceState{}, false, err
	}
	var state model.UserWorkspaceState
	err = db.First(&state, "organization_id = ?", organizationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserWorkspaceState{}, false, nil
	}
	return state, err == nil, err
}

func SaveUserWorkspaceState(state model.UserWorkspaceState) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Save(&state).Error
}

func ListUserProjects(organizationID string) ([]model.UserProject, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserProject
	err = db.Where("organization_id = ? AND deleted_at = ''", organizationID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func ListUserAssets(organizationID string) ([]model.UserAsset, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserAsset
	err = db.Where("organization_id = ? AND deleted_at = ''", organizationID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func ListUserGenerationRecords(organizationID string) ([]model.UserGenerationRecord, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserGenerationRecord
	err = db.Where("organization_id = ?", organizationID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func ApplyUserWorkspaceMutations(organizationID string, userID string, mutations []model.UserWorkspaceMutation, updatedAt string, auditLogs ...model.OrganizationAuditLog) ([]model.UserProject, []model.UserAsset, []model.UserGenerationRecord, error) {
	db, err := DB()
	if err != nil {
		return nil, nil, nil, err
	}
	projects := make([]model.UserProject, 0, len(mutations))
	assets := make([]model.UserAsset, 0, len(mutations))
	records := make([]model.UserGenerationRecord, 0, len(mutations))
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		for _, mutation := range mutations {
			if mutation.Domain == "canvas_project" {
				item, err := saveUserProject(tx, organizationID, userID, mutation, updatedAt)
				if err != nil {
					return err
				}
				projects = append(projects, item)
				if err := replaceUserFileReferences(tx, organizationID, mutation, updatedAt); err != nil { return err }
				continue
			}
			if mutation.Domain == "generation_record" {
				item, err := saveUserGenerationRecord(tx, organizationID, userID, mutation, updatedAt)
				if err != nil {
					return err
				}
				records = append(records, item)
				if err := replaceUserFileReferences(tx, organizationID, mutation, updatedAt); err != nil { return err }
				continue
			}
			item, err := saveUserAsset(tx, organizationID, userID, mutation, updatedAt)
			if err != nil {
				return err
			}
			assets = append(assets, item)
			if err := replaceUserFileReferences(tx, organizationID, mutation, updatedAt); err != nil { return err }
		}
		if err := tx.Save(&model.UserWorkspaceState{OrganizationID: organizationID, UpdatedAt: updatedAt}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return projects, assets, records, err
}

func replaceUserFileReferences(tx *gorm.DB, organizationID string, mutation model.UserWorkspaceMutation, timestamp string) error {
	if err := tx.Where("organization_id = ? AND domain = ? AND object_id = ?", organizationID, mutation.Domain, mutation.ObjectID).Delete(&model.UserFileReference{}).Error; err != nil { return err }
	if mutation.Deleted || len(mutation.StorageKeys) == 0 { return nil }
	var total int64
	if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND storage_key IN ?", organizationID, mutation.StorageKeys).Count(&total).Error; err != nil { return err }
	if total != int64(len(mutation.StorageKeys)) { return ErrWorkspaceFileMissing }
	items := make([]model.UserFileReference, 0, len(mutation.StorageKeys))
	for index, storageKey := range mutation.StorageKeys {
		items = append(items, model.UserFileReference{ID: mutation.RecordID + "-" + strconv.Itoa(index), OrganizationID: organizationID, Domain: mutation.Domain, ObjectID: mutation.ObjectID, StorageKey: storageKey, CreatedAt: timestamp})
	}
	return tx.Create(&items).Error
}

func saveUserProject(tx *gorm.DB, organizationID string, userID string, mutation model.UserWorkspaceMutation, updatedAt string) (model.UserProject, error) {
	var item model.UserProject
	result := tx.Where("organization_id = ? AND id = ?", organizationID, mutation.ObjectID).First(&item)
	found := result.Error == nil
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return item, result.Error
	}
	if found && item.Data == mutation.Data && (item.DeletedAt != "") == mutation.Deleted {
		return item, nil
	}
	if found && item.Version != mutation.ExpectedVersion { return item, ErrWorkspaceVersionConflict }
	if !found && mutation.ExpectedVersion != 0 { return item, ErrWorkspaceVersionConflict }
	if found && item.Data != mutation.Data {
		version := model.UserProjectVersion{ID: mutation.RecordID, OrganizationID: organizationID, UserID: userID, ProjectID: item.ID, Version: item.Version, Data: item.Data, CreatedAt: updatedAt}
		if err := tx.Save(&version).Error; err != nil {
			return item, err
		}
	}
	if !found {
		item = model.UserProject{ID: mutation.ObjectID, OrganizationID: organizationID, UserID: userID, CreatedAt: updatedAt}
	}
	item.Title = mutation.Title
	item.Data = mutation.Data
	item.Version++
	item.UpdatedAt = updatedAt
	if mutation.Deleted {
		item.DeletedAt = updatedAt
	} else {
		item.DeletedAt = ""
	}
	if !found {
		if err := tx.Create(&item).Error; err != nil { return item, err }
	} else {
		result := tx.Model(&model.UserProject{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, item.ID, mutation.ExpectedVersion).Updates(map[string]any{"user_id": userID, "title": item.Title, "data": item.Data, "version": item.Version, "deleted_at": item.DeletedAt, "updated_at": item.UpdatedAt})
		if result.Error != nil { return item, result.Error }
		if result.RowsAffected != 1 { return item, ErrWorkspaceVersionConflict }
	}
	if item.Version > 50 {
		if err := tx.Where("organization_id = ? AND project_id = ? AND version < ?", organizationID, item.ID, item.Version-50).Delete(&model.UserProjectVersion{}).Error; err != nil {
			return item, err
		}
	}
	return item, nil
}

func saveUserAsset(tx *gorm.DB, organizationID string, userID string, mutation model.UserWorkspaceMutation, updatedAt string) (model.UserAsset, error) {
	var item model.UserAsset
	result := tx.Where("organization_id = ? AND id = ?", organizationID, mutation.ObjectID).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return item, result.Error
	}
	if result.Error == nil && item.Data == mutation.Data && (item.DeletedAt != "") == mutation.Deleted {
		return item, nil
	}
	if result.Error == nil && item.Version != mutation.ExpectedVersion { return item, ErrWorkspaceVersionConflict }
	if errors.Is(result.Error, gorm.ErrRecordNotFound) && mutation.ExpectedVersion != 0 { return item, ErrWorkspaceVersionConflict }
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		item = model.UserAsset{ID: mutation.ObjectID, OrganizationID: organizationID, UserID: userID, CreatedAt: updatedAt}
	}
	item.Title = mutation.Title
	item.Kind = mutation.Kind
	item.Data = mutation.Data
	item.Version++
	item.UpdatedAt = updatedAt
	if mutation.Deleted {
		item.DeletedAt = updatedAt
	} else {
		item.DeletedAt = ""
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) { return item, tx.Create(&item).Error }
	update := tx.Model(&model.UserAsset{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, item.ID, mutation.ExpectedVersion).Updates(map[string]any{"user_id": userID, "title": item.Title, "kind": item.Kind, "data": item.Data, "version": item.Version, "deleted_at": item.DeletedAt, "updated_at": item.UpdatedAt})
	if update.Error != nil { return item, update.Error }
	if update.RowsAffected != 1 { return item, ErrWorkspaceVersionConflict }
	return item, nil
}

func saveUserGenerationRecord(tx *gorm.DB, organizationID string, userID string, mutation model.UserWorkspaceMutation, updatedAt string) (model.UserGenerationRecord, error) {
	var item model.UserGenerationRecord
	result := tx.Where("organization_id = ? AND id = ?", organizationID, mutation.ObjectID).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return item, result.Error
	}
	if result.Error == nil && item.Data == mutation.Data && (item.DeletedAt != "") == mutation.Deleted {
		return item, nil
	}
	if result.Error == nil && item.Version != mutation.ExpectedVersion { return item, ErrWorkspaceVersionConflict }
	if errors.Is(result.Error, gorm.ErrRecordNotFound) && mutation.ExpectedVersion != 0 { return item, ErrWorkspaceVersionConflict }
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		item = model.UserGenerationRecord{ID: mutation.ObjectID, OrganizationID: organizationID, UserID: userID, CreatedAt: updatedAt}
	}
	item.Kind = mutation.Kind
	item.Data = mutation.Data
	item.Version++
	item.UpdatedAt = updatedAt
	if mutation.Deleted {
		item.DeletedAt = updatedAt
	} else {
		item.DeletedAt = ""
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) { return item, tx.Create(&item).Error }
	update := tx.Model(&model.UserGenerationRecord{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, item.ID, mutation.ExpectedVersion).Updates(map[string]any{"user_id": userID, "kind": item.Kind, "data": item.Data, "version": item.Version, "deleted_at": item.DeletedAt, "updated_at": item.UpdatedAt})
	if update.Error != nil { return item, update.Error }
	if update.RowsAffected != 1 { return item, ErrWorkspaceVersionConflict }
	return item, nil
}

func GetUserFile(organizationID string, storageKey string) (model.UserFile, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserFile{}, false, err
	}
	var item model.UserFile
	err = db.Where("organization_id = ? AND storage_key = ?", organizationID, storageKey).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserFile{}, false, nil
	}
	return item, err == nil, err
}

func ReserveUserFileUpload(item model.UserFileUploadReservation, quota int64, timestamp string) (model.UserFileUploadReservation, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		if err := expireUserFileUploadReservations(tx, item.OrganizationID, timestamp); err != nil { return err }
		var previous model.UserFileUploadReservation
		if err := tx.Where("organization_id = ? AND storage_key = ?", item.OrganizationID, item.StorageKey).First(&previous).Error; err == nil {
			if err := queueUserObjectDeletion(tx, previous.ObjectKey, previous.ExpiresAt, timestamp); err != nil { return err }
			if err := tx.Delete(&previous).Error; err != nil { return err }
		} else if !errors.Is(err, gorm.ErrRecordNotFound) { return err }
		var used, reserved int64
		if err := tx.Model(&model.UserFile{}).Where("organization_id = ?", item.OrganizationID).Select("COALESCE(SUM(size), 0)").Scan(&used).Error; err != nil { return err }
		if err := tx.Model(&model.UserFileUploadReservation{}).Where("organization_id = ?", item.OrganizationID).Select("COALESCE(SUM(reserved_bytes), 0)").Scan(&reserved).Error; err != nil { return err }
		var existing model.UserFile
		if err := tx.Where("organization_id = ? AND storage_key = ?", item.OrganizationID, item.StorageKey).First(&existing).Error; err == nil {
			item.ReservedBytes = item.Size - existing.Size
			if item.ReservedBytes < 0 { item.ReservedBytes = 0 }
		} else if errors.Is(err, gorm.ErrRecordNotFound) { item.ReservedBytes = item.Size } else { return err }
		if used+reserved+item.ReservedBytes > quota { return ErrWorkspaceStorageQuotaExceeded }
		return tx.Create(&item).Error
	})
	return item, err
}

func ConfirmUserFileUpload(organizationID string, userID string, reservationID string, fileID string, hash string, mimeType string, size int64, timestamp string) (model.UserFile, error) {
	db, err := DB()
	if err != nil { return model.UserFile{}, err }
	var item model.UserFile
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var reservation model.UserFileUploadReservation
		if err := tx.Where("organization_id = ? AND id = ? AND user_id = ? AND expires_at > ?", organizationID, reservationID, userID, timestamp).First(&reservation).Error; err != nil { if errors.Is(err, gorm.ErrRecordNotFound) { return ErrWorkspaceUploadReservationUnavailable }; return err }
		if reservation.Size != size { return ErrWorkspaceUploadReservationUnavailable }
		var existing model.UserFile
		result := tx.Where("organization_id = ? AND storage_key = ?", organizationID, reservation.StorageKey).First(&existing)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) { return result.Error }
		item = model.UserFile{ID: fileID, OrganizationID: organizationID, UserID: userID, StorageKey: reservation.StorageKey, ObjectKey: reservation.ObjectKey, Hash: hash, MimeType: mimeType, Size: size, CreatedAt: timestamp, UpdatedAt: timestamp}
		if result.Error == nil {
			item.ID, item.CreatedAt = existing.ID, existing.CreatedAt
			if existing.ObjectKey != reservation.ObjectKey { if err := queueUserObjectDeletion(tx, existing.ObjectKey, timestamp, timestamp); err != nil { return err } }
			if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND id = ?", organizationID, existing.ID).Updates(map[string]any{"user_id": userID, "object_key": item.ObjectKey, "hash": hash, "mime_type": mimeType, "size": size, "unreferenced_at": "", "updated_at": timestamp}).Error; err != nil { return err }
		} else if err := tx.Create(&item).Error; err != nil { return err }
		return tx.Delete(&reservation).Error
	})
	return item, err
}

func GetUserFileUploadReservation(organizationID string, userID string, id string) (model.UserFileUploadReservation, bool, error) {
	db, err := DB()
	if err != nil { return model.UserFileUploadReservation{}, false, err }
	var item model.UserFileUploadReservation
	err = db.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.UserFileUploadReservation{}, false, nil }
	return item, err == nil, err
}

func ListUserFiles(organizationID string) ([]model.UserFile, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserFile
	err = db.Where("organization_id = ?", organizationID).Find(&items).Error
	return items, err
}

func CollectUserFileGarbage(organizationID string, timestamp string, cutoff string) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		if err := expireUserFileUploadReservations(tx, organizationID, timestamp); err != nil { return err }
		var referenced []string
		if err := tx.Model(&model.UserFileReference{}).Where("organization_id = ?", organizationID).Distinct("storage_key").Pluck("storage_key", &referenced).Error; err != nil { return err }
		if len(referenced) > 0 {
			if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND storage_key IN ?", organizationID, referenced).Update("unreferenced_at", "").Error; err != nil { return err }
			if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND storage_key NOT IN ? AND unreferenced_at = ''", organizationID, referenced).Update("unreferenced_at", timestamp).Error; err != nil { return err }
		} else if err := tx.Model(&model.UserFile{}).Where("organization_id = ? AND unreferenced_at = ''", organizationID).Update("unreferenced_at", timestamp).Error; err != nil { return err }
		query := tx.Where("organization_id = ? AND unreferenced_at <> '' AND unreferenced_at <= ?", organizationID, cutoff)
		if len(referenced) > 0 { query = query.Where("storage_key NOT IN ?", referenced) }
		var files []model.UserFile
		if err := query.Find(&files).Error; err != nil { return err }
		for _, file := range files {
			if err := queueUserObjectDeletion(tx, file.ObjectKey, timestamp, timestamp); err != nil { return err }
			if err := tx.Where("organization_id = ? AND id = ? AND unreferenced_at <= ?", organizationID, file.ID, cutoff).Delete(&model.UserFile{}).Error; err != nil { return err }
		}
		return nil
	})
}

func expireUserFileUploadReservations(tx *gorm.DB, organizationID string, timestamp string) error {
	var items []model.UserFileUploadReservation
	if err := tx.Where("organization_id = ? AND expires_at <= ?", organizationID, timestamp).Find(&items).Error; err != nil { return err }
	for _, item := range items { if err := queueUserObjectDeletion(tx, item.ObjectKey, timestamp, timestamp); err != nil { return err } }
	return tx.Where("organization_id = ? AND expires_at <= ?", organizationID, timestamp).Delete(&model.UserFileUploadReservation{}).Error
}

func queueUserObjectDeletion(tx *gorm.DB, objectKey string, nextAttemptAt string, timestamp string) error {
	if objectKey == "" { return nil }
	item := model.UserObjectDeletion{ID: objectKey, ObjectKey: objectKey, Status: "pending", NextAttemptAt: nextAttemptAt, CreatedAt: timestamp, UpdatedAt: timestamp}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&item).Error
}

func ClaimUserObjectDeletion(timestamp string, leaseExpiresAt string, leaseToken string) (model.UserObjectDeletion, bool, error) {
	db, err := DB()
	if err != nil { return model.UserObjectDeletion{}, false, err }
	var item model.UserObjectDeletion
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("(status IN ? AND next_attempt_at <= ?) OR (status = ? AND lease_expires_at <= ?)", []string{"pending", "failed"}, timestamp, "processing", timestamp).Order("created_at asc").First(&item).Error; err != nil { return err }
		return tx.Model(&model.UserObjectDeletion{}).Where("id = ?", item.ID).Updates(map[string]any{"status": "processing", "attempts": gorm.Expr("attempts + 1"), "lease_token": leaseToken, "lease_expires_at": leaseExpiresAt, "updated_at": timestamp}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.UserObjectDeletion{}, false, nil }
	if err != nil { return model.UserObjectDeletion{}, false, err }
	item.Status, item.LeaseToken, item.LeaseExpiresAt = "processing", leaseToken, leaseExpiresAt
	item.Attempts++
	return item, true, nil
}

func FinishUserObjectDeletion(item model.UserObjectDeletion, succeeded bool, message string, nextAttemptAt string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	query := db.Where("id = ? AND status = ? AND lease_token = ?", item.ID, "processing", item.LeaseToken)
	var result *gorm.DB
	if succeeded { result = query.Delete(&model.UserObjectDeletion{}) } else { result = query.Model(&model.UserObjectDeletion{}).Updates(map[string]any{"status": "failed", "lease_token": "", "lease_expires_at": "", "next_attempt_at": nextAttemptAt, "last_error": message, "updated_at": timestamp}) }
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
	return nil
}

func ListUserFileMaintenanceOrganizations(timestamp string, cutoff string) ([]string, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var uploadIDs, fileIDs []string
	if err := db.Model(&model.UserFileUploadReservation{}).Where("expires_at <= ?", timestamp).Distinct("organization_id").Pluck("organization_id", &uploadIDs).Error; err != nil { return nil, err }
	if err := db.Model(&model.UserFile{}).Where("unreferenced_at <> '' AND unreferenced_at <= ?", cutoff).Distinct("organization_id").Pluck("organization_id", &fileIDs).Error; err != nil { return nil, err }
	seen := make(map[string]bool, len(uploadIDs)+len(fileIDs))
	ids := make([]string, 0, len(uploadIDs)+len(fileIDs))
	for _, id := range append(uploadIDs, fileIDs...) { if id != "" && !seen[id] { seen[id] = true; ids = append(ids, id) } }
	return ids, nil
}

func UserStorageUsage(organizationID string) (int64, int64, error) {
	db, err := DB()
	if err != nil {
		return 0, 0, err
	}
	var usage int64
	if err := db.Model(&model.UserFile{}).Where("organization_id = ?", organizationID).Select("COALESCE(SUM(size), 0)").Scan(&usage).Error; err != nil {
		return 0, 0, err
	}
	var count int64
	err = db.Model(&model.UserFile{}).Where("organization_id = ?", organizationID).Count(&count).Error
	return usage, count, err
}
