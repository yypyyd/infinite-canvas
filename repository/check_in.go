package repository

import (
	"errors"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

func HasCheckIn(userID string, date string) (bool, error) {
	db, err := DB()
	if err != nil {
		return false, err
	}
	var total int64
	err = db.Model(&model.CheckIn{}).Where("user_id = ? AND check_in_date = ?", userID, date).Count(&total).Error
	return total > 0, err
}

// CreateCheckIn 原子写入签到记录、增加余额并记录额度流水。
func CreateCheckIn(checkIn model.CheckIn, log *model.CreditLog, updatedAt string) (model.User, error) {
	db, err := DB()
	if err != nil {
		return model.User{}, err
	}
	user := model.User{}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&checkIn).Error; err != nil {
			return err
		}
		if checkIn.RewardCredits > 0 {
			result := tx.Model(&model.User{}).Where("id = ?", checkIn.UserID).Updates(map[string]any{
				"credits":    gorm.Expr("credits + ?", checkIn.RewardCredits),
				"updated_at": updatedAt,
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return errors.New("user not found")
			}
		}
		if err := tx.First(&user, "id = ?", checkIn.UserID).Error; err != nil {
			return err
		}
		if log != nil {
			log.Balance = user.Credits
			return tx.Create(log).Error
		}
		return nil
	})
	return user, err
}
