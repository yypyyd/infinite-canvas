package repository

import (
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
)

func ListUserReferralCommissions(userID string, q model.Query) ([]model.ReferralCommission, int64, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.ReferralCommission{}).
		Joins("LEFT JOIN users ON users.id = referral_commissions.invitee_id").
		Where("referral_commissions.inviter_id = ?", userID)
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		tx = tx.Where("referral_commissions.order_no LIKE ? OR referral_commissions.invitee_id LIKE ? OR users.username LIKE ?", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, 0, err
	}
	var commissionTotal int64
	if err := db.Model(&model.ReferralCommission{}).Where("inviter_id = ?", userID).Select("COALESCE(SUM(commission_cents), 0)").Scan(&commissionTotal).Error; err != nil {
		return nil, 0, 0, err
	}
	var items []model.ReferralCommission
	err = tx.Select("referral_commissions.*, users.username AS invitee_username").Order("referral_commissions.created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, commissionTotal, err
}

func CountUserReferrals(userID string) (int64, error) {
	db, err := DB()
	if err != nil {
		return 0, err
	}
	var total int64
	err = db.Model(&model.User{}).Where("inviter_id = ?", userID).Count(&total).Error
	return total, err
}
