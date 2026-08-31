package repository

import (
	"sort"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
)

// ListUserPricingDiscounts returns every exact-spec pricing ratio configured for a user.
func ListUserPricingDiscounts(userID string) ([]model.UserPricingDiscount, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	var items []model.UserPricingDiscount
	err = db.Where("user_id = ?", userID).
		Order("model asc, modality asc, operation asc, unit asc, resolution_tier asc").
		Find(&items).Error
	return items, err
}

// ReplaceUserPricingDiscounts atomically replaces a user's complete discount set.
func ReplaceUserPricingDiscounts(userID string, items []model.UserPricingDiscount) ([]model.UserPricingDiscount, error) {
	db, err := DB()
	if err != nil {
		return nil, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&model.User{}, "id = ?", userID).Error; err != nil {
			return err
		}
		var existing []model.UserPricingDiscount
		if err := tx.Where("user_id = ?", userID).Find(&existing).Error; err != nil {
			return err
		}
		existingBySpec := make(map[string]model.UserPricingDiscount, len(existing))
		for _, item := range existing {
			existingBySpec[userPricingDiscountKey(item)] = item
		}
		if err := tx.Where("user_id = ?", userID).Delete(&model.UserPricingDiscount{}).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].UserID = userID
			if previous, ok := existingBySpec[userPricingDiscountKey(items[index])]; ok {
				items[index].ID = previous.ID
				items[index].CreatedAt = previous.CreatedAt
			}
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool {
		return userPricingDiscountKey(items[i]) < userPricingDiscountKey(items[j])
	})
	return items, nil
}

func userPricingDiscountKey(item model.UserPricingDiscount) string {
	return item.Model + "\x00" + item.Modality + "\x00" + item.Operation + "\x00" + item.Unit + "\x00" + item.ResolutionTier
}
