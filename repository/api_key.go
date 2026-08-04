package repository

import (
	"errors"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUserAPIKeyLimit    = errors.New("user api key limit reached")
	ErrUserAPIKeyNotFound = errors.New("user api key not found")
)

func ListUserAPIKeys(organizationID string, userID string) ([]model.UserAPIKey, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserAPIKey
	err = db.Where("organization_id = ? AND user_id = ?", organizationID, userID).Order("created_at desc").Find(&items).Error
	return items, err
}

func CreateUserAPIKey(item model.UserAPIKey, activeLimit int, auditLogs ...model.OrganizationAuditLog) (model.UserAPIKey, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", item.OrganizationID).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&model.UserAPIKey{}).Where("organization_id = ? AND user_id = ? AND status = ?", item.OrganizationID, item.UserID, model.UserAPIKeyStatusActive).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(activeLimit) {
			return ErrUserAPIKeyLimit
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func GetActiveUserAPIKeyByHash(keyHash string) (model.UserAPIKey, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserAPIKey{}, false, err
	}
	var item model.UserAPIKey
	err = db.Where("key_hash = ? AND status = ?", keyHash, model.UserAPIKeyStatusActive).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserAPIKey{}, false, nil
	}
	return item, err == nil, err
}

func DeleteUserAPIKey(organizationID string, userID string, id string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", organizationID).Error; err != nil {
			return err
		}
		result := tx.Where("organization_id = ? AND user_id = ? AND id = ?", organizationID, userID, id).Delete(&model.UserAPIKey{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrUserAPIKeyNotFound
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func TouchUserAPIKey(id string, cutoff string, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.UserAPIKey{}).Where("id = ? AND status = ? AND (last_used_at = '' OR last_used_at < ?)", id, model.UserAPIKeyStatusActive, cutoff).Update("last_used_at", timestamp).Error
}
