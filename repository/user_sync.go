package repository

import (
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func UserSyncCursor(userID string) (int64, error) {
	state, ok, err := GetUserSyncState(userID)
	if !ok {
		return 0, err
	}
	return state.Cursor, err
}

func GetUserSyncState(userID string) (model.UserSyncState, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserSyncState{}, false, err
	}
	var state model.UserSyncState
	err = db.First(&state, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserSyncState{}, false, nil
	}
	return state, err == nil, err
}

func ListUserSyncRecords(userID string, after int64, includeDeleted bool) ([]model.UserSyncRecord, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	tx := db.Where("user_id = ?", userID)
	if after > 0 {
		tx = tx.Where("change_seq > ?", after)
	}
	if !includeDeleted {
		tx = tx.Where("deleted_at = ''")
	}
	var items []model.UserSyncRecord
	err = tx.Order("change_seq asc, updated_at asc").Find(&items).Error
	return items, err
}

func ApplyUserSyncMutations(userID string, mutations []model.UserSyncMutation, updatedAt string) ([]model.UserSyncRecord, []model.UserSyncRecord, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, nil, 0, err
	}
	var saved []model.UserSyncRecord
	var conflicts []model.UserSyncRecord
	var cursor int64
	err = db.Transaction(func(tx *gorm.DB) error {
		var state model.UserSyncState
		result := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&state, "user_id = ?", userID)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			state = model.UserSyncState{UserID: userID}
		} else if result.Error != nil {
			return result.Error
		}

		pending := make([]model.UserSyncRecord, 0, len(mutations))
		for _, mutation := range mutations {
			var current model.UserSyncRecord
			result = tx.Where("user_id = ? AND domain = ? AND object_id = ?", userID, mutation.Domain, mutation.ObjectID).First(&current)
			if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
				return result.Error
			}
			found := result.Error == nil
			if (found && current.Revision != mutation.BaseRevision) || (!found && mutation.BaseRevision != 0) {
				if found {
					conflicts = append(conflicts, current)
				}
				continue
			}
			record := current
			if !found {
				record = model.UserSyncRecord{ID: mutation.RecordID, UserID: userID, Domain: mutation.Domain, ObjectID: mutation.ObjectID, CreatedAt: updatedAt}
			}
			record.Data = mutation.Data
			record.Revision++
			record.UpdatedAt = updatedAt
			if mutation.Deleted {
				record.DeletedAt = updatedAt
			} else {
				record.DeletedAt = ""
			}
			pending = append(pending, record)
		}

		if len(pending) > 0 {
			state.Cursor++
			state.UpdatedAt = updatedAt
			if err := tx.Save(&state).Error; err != nil {
				return err
			}
			for _, record := range pending {
				record.ChangeSeq = state.Cursor
				if err := tx.Save(&record).Error; err != nil {
					return err
				}
				saved = append(saved, record)
			}
		}
		cursor = state.Cursor
		return nil
	})
	return saved, conflicts, cursor, err
}

func GetUserFile(userID string, storageKey string) (model.UserFile, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserFile{}, false, err
	}
	var item model.UserFile
	err = db.Where("user_id = ? AND storage_key = ?", userID, storageKey).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserFile{}, false, nil
	}
	return item, err == nil, err
}

func GetUserFileByHash(userID string, hash string) (model.UserFile, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserFile{}, false, err
	}
	var item model.UserFile
	err = db.Where("user_id = ? AND sha256 = ?", userID, hash).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserFile{}, false, nil
	}
	return item, err == nil, err
}

func SaveUserFile(item model.UserFile) (model.UserFile, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	return item, db.Save(&item).Error
}

func ListUserFiles(userID string) ([]model.UserFile, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserFile
	err = db.Where("user_id = ?", userID).Find(&items).Error
	return items, err
}

func DeleteUserFile(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.UserFile{}, "id = ?", id).Error
}

func CountUserFileHash(userID string, hash string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.UserFile{}).Where("user_id = ? AND sha256 = ?", userID, hash).Count(&total).Error
	return total, err
}

func UserStorageUsage(userID string) (int64, int64, error) {
	db, err := DB()
	if err != nil {
		return 0, 0, err
	}
	type fileSize struct {
		SHA256 string
		Size   int64
	}
	var items []fileSize
	if err := db.Model(&model.UserFile{}).Where("user_id = ?", userID).Select("sha256, MAX(size) AS size").Group("sha256").Scan(&items).Error; err != nil {
		return 0, 0, err
	}
	var usage int64
	for _, item := range items {
		usage += item.Size
	}
	var count int64
	err = db.Model(&model.UserFile{}).Where("user_id = ?", userID).Count(&count).Error
	return usage, count, err
}
