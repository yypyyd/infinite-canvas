package repository

import (
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

func GetUserPreference(userID string) (model.UserPreference, bool, error) {
	db, err := DB()
	if err != nil {
		return model.UserPreference{}, false, err
	}
	var item model.UserPreference
	err = db.First(&item, "user_id = ?", userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.UserPreference{}, false, nil
	}
	return item, err == nil, err
}

func SaveUserPreference(item model.UserPreference) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Save(&item).Error
}
