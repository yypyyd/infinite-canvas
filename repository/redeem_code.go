package repository

import (
	"errors"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
)

var (
	ErrRedemptionCodeNotFound    = errors.New("redemption code not found")
	ErrRedemptionCodeUnavailable = errors.New("redemption code unavailable")
	ErrRedemptionUserNotFound    = errors.New("redemption user not found")
)

func ListRedemptionCodes(q model.Query) ([]model.RedemptionCode, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.RedemptionCode{})
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("code LIKE ? OR status LIKE ? OR used_by LIKE ? OR remark LIKE ?", like, like, like, like)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var codes []model.RedemptionCode
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&codes).Error
	return codes, total, err
}

func CreateRedemptionCodes(codes []model.RedemptionCode) ([]model.RedemptionCode, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	if len(codes) == 0 {
		return codes, nil
	}
	return codes, db.Create(&codes).Error
}

func DeleteRedemptionCode(id string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Delete(&model.RedemptionCode{}, "id = ?", id).Error
}

func RedeemRedemptionCode(userID string, codeText string, redeemedAt string, log model.CreditLog) (model.RedemptionCode, model.User, error) {
	db, err := DB()
	if err != nil {
		return model.RedemptionCode{}, model.User{}, err
	}

	var redeemedCode model.RedemptionCode
	var redeemedUser model.User
	err = db.Transaction(func(tx *gorm.DB) error {
		var code model.RedemptionCode
		err := tx.Where("code = ?", codeText).First(&code).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRedemptionCodeNotFound
		}
		if err != nil {
			return err
		}
		if code.Status != model.RedemptionCodeStatusActive || code.UsedBy != "" || code.UsedAt != "" || code.Credits <= 0 {
			return ErrRedemptionCodeUnavailable
		}

		codeUpdate := tx.Model(&model.RedemptionCode{}).
			Where("id = ? AND status = ? AND used_by = ? AND used_at = ?", code.ID, model.RedemptionCodeStatusActive, "", "").
			Updates(map[string]any{
				"status":     model.RedemptionCodeStatusUsed,
				"used_by":    userID,
				"used_at":    redeemedAt,
				"updated_at": redeemedAt,
			})
		if codeUpdate.Error != nil {
			return codeUpdate.Error
		}
		if codeUpdate.RowsAffected == 0 {
			return ErrRedemptionCodeUnavailable
		}

		update := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
			"credits":    gorm.Expr("credits + ?", code.Credits),
			"updated_at": redeemedAt,
		})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected == 0 {
			return ErrRedemptionUserNotFound
		}
		if err := tx.Where("id = ?", userID).First(&redeemedUser).Error; err != nil {
			return err
		}

		code.Status = model.RedemptionCodeStatusUsed
		code.UsedBy = userID
		code.UsedAt = redeemedAt
		code.UpdatedAt = redeemedAt

		log.UserID = userID
		log.RelatedID = code.ID
		log.Amount = code.Credits
		log.Balance = redeemedUser.Credits
		log.CreatedAt = redeemedAt
		if err := tx.Save(&log).Error; err != nil {
			return err
		}

		redeemedCode = code
		return nil
	})
	return redeemedCode, redeemedUser, err
}
