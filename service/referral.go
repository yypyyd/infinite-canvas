package service

import (
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func GetReferralDashboard(user model.AuthUser, q model.Query) (model.ReferralDashboard, error) {
	items, total, commissionTotal, err := repository.ListUserReferralCommissions(user.ID, q)
	if err != nil {
		return model.ReferralDashboard{}, err
	}
	invitedCount, err := repository.CountUserReferrals(user.ID)
	if err != nil {
		return model.ReferralDashboard{}, err
	}
	owner, ok, err := repository.GetUserByID(user.ID)
	if err != nil {
		return model.ReferralDashboard{}, err
	}
	if !ok {
		return model.ReferralDashboard{}, safeMessageError{message: "用户不存在"}
	}
	settings, err := repository.GetSettings()
	if err != nil {
		return model.ReferralDashboard{}, err
	}
	referral := normalizeReferralSetting(settings.Private.Referral)
	if items == nil {
		items = []model.ReferralCommission{}
	}
	return model.ReferralDashboard{
		AffCode: owner.AffCode, InvitedCount: int(invitedCount), TotalCommissionCents: commissionTotal,
		ReferralEnabled: referral.Enabled, CommissionRate: referral.CommissionRate, Items: items, Total: int(total),
	}, nil
}
