package repository

import (
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

func GetUserWorkspaceState(userID string) (model.UserWorkspaceState, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserWorkspaceState{}, false, err
	}
	var state model.UserWorkspaceState
	err = db.First(&state, "user_id = ?", userID).Error
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

func ListUserProjects(userID string) ([]model.UserProject, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserProject
	err = db.Where("user_id = ? AND deleted_at = ''", userID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func ListUserAssets(userID string) ([]model.UserAsset, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserAsset
	err = db.Where("user_id = ? AND deleted_at = ''", userID).Order("updated_at desc").Find(&items).Error
	return items, err
}

func ApplyUserWorkspaceMutations(userID string, mutations []model.UserWorkspaceMutation, updatedAt string) ([]model.UserProject, []model.UserAsset, error) {
	db, err := DB()
	if err != nil {
		return nil, nil, err
	}
	projects := make([]model.UserProject, 0, len(mutations))
	assets := make([]model.UserAsset, 0, len(mutations))
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, mutation := range mutations {
			if mutation.Domain == "canvas_project" {
				item, err := saveUserProject(tx, userID, mutation, updatedAt)
				if err != nil {
					return err
				}
				projects = append(projects, item)
				continue
			}
			item, err := saveUserAsset(tx, userID, mutation, updatedAt)
			if err != nil {
				return err
			}
			assets = append(assets, item)
		}
		return nil
	})
	return projects, assets, err
}

func saveUserProject(tx *gorm.DB, userID string, mutation model.UserWorkspaceMutation, updatedAt string) (model.UserProject, error) {
	var item model.UserProject
	result := tx.Where("user_id = ? AND id = ?", userID, mutation.ObjectID).First(&item)
	found := result.Error == nil
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return item, result.Error
	}
	if found && item.Data == mutation.Data && (item.DeletedAt != "") == mutation.Deleted {
		return item, nil
	}
	if found && item.Data != mutation.Data {
		version := model.UserProjectVersion{ID: mutation.RecordID, UserID: userID, ProjectID: item.ID, Version: item.Version, Data: item.Data, CreatedAt: updatedAt}
		if err := tx.Save(&version).Error; err != nil {
			return item, err
		}
	}
	if !found {
		item = model.UserProject{ID: mutation.ObjectID, UserID: userID, CreatedAt: updatedAt}
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
	if err := tx.Save(&item).Error; err != nil {
		return item, err
	}
	if item.Version > 50 {
		if err := tx.Where("user_id = ? AND project_id = ? AND version < ?", userID, item.ID, item.Version-50).Delete(&model.UserProjectVersion{}).Error; err != nil {
			return item, err
		}
	}
	return item, nil
}

func saveUserAsset(tx *gorm.DB, userID string, mutation model.UserWorkspaceMutation, updatedAt string) (model.UserAsset, error) {
	var item model.UserAsset
	result := tx.Where("user_id = ? AND id = ?", userID, mutation.ObjectID).First(&item)
	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return item, result.Error
	}
	if result.Error == nil && item.Data == mutation.Data && (item.DeletedAt != "") == mutation.Deleted {
		return item, nil
	}
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		item = model.UserAsset{ID: mutation.ObjectID, UserID: userID, CreatedAt: updatedAt}
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
	return item, tx.Save(&item).Error
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

func UserStorageUsage(userID string) (int64, int64, error) {
	db, err := DB()
	if err != nil {
		return 0, 0, err
	}
	var usage int64
	if err := db.Model(&model.UserFile{}).Where("user_id = ?", userID).Select("COALESCE(SUM(size), 0)").Scan(&usage).Error; err != nil {
		return 0, 0, err
	}
	var count int64
	err = db.Model(&model.UserFile{}).Where("user_id = ?", userID).Count(&count).Error
	return usage, count, err
}
