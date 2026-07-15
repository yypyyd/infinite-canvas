package repository

import (
	"errors"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

var ErrEmailVerificationUnavailable = errors.New("email verification is unavailable")

func CreateVerifiedUserWithCreditLog(user model.User, log *model.CreditLog, verificationID string, codeHash string, now string, maxAttempts int) (model.User, error) {
	db, err := DB()
	if err != nil {
		return user, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.EmailVerification{}).
			Where("id = ? AND code_hash = ? AND used_at = ? AND expires_at > ? AND attempts < ?", verificationID, codeHash, "", now, maxAttempts).
			Update("used_at", now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrEmailVerificationUnavailable
		}
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		if log == nil {
			return nil
		}
		log.Balance = user.Credits
		return tx.Create(log).Error
	})
	return user, err
}

func GetEmailVerification(id string) (model.EmailVerification, bool, error) {
	db, err := DB()
	if err != nil {
		return model.EmailVerification{}, false, err
	}
	item := model.EmailVerification{}
	err = db.Where("id = ?", id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.EmailVerification{}, false, nil
	}
	return item, err == nil, err
}

func SaveEmailVerification(item model.EmailVerification) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Save(&item).Error
}

func DeleteExpiredEmailVerifications(before string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("expires_at <= ?", before).Delete(&model.EmailVerification{}).Error
}

func DeleteEmailVerification(id string, codeHash string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Where("id = ? AND code_hash = ?", id, codeHash).Delete(&model.EmailVerification{}).Error
}

func IncrementEmailVerificationAttempts(id string, codeHash string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Model(&model.EmailVerification{}).
		Where("id = ? AND code_hash = ? AND used_at = ?", id, codeHash, "").
		UpdateColumn("attempts", gorm.Expr("attempts + 1")).Error
}

func CountRecentEmailVerificationsByIP(ip string, since string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.EmailVerification{}).Where("request_ip = ? AND sent_at >= ?", ip, since).Count(&total).Error
	return total, err
}
