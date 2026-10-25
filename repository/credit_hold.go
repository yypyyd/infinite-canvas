package repository

import (
	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Credit hold / settle / release is the shared envelope used by batch production
// jobs and video-render jobs. Model generation (interactive / agent / batch item /
// protocol) still debits through BeginGenerationTask -> Create*GenerationTaskWithCharge;
// FinishGenerationTask is the sole refund point for those tasks.
//
// Hold: increase reserved_credits (does not spend yet).
// Settle: move amount from reserved into spent credits; release leftover hold.
// Release: return outstanding reserved without spending.

// HoldCredits reserves amount against the organization shared pool or the
// personal user (createdBy), matching organization.CreditMode. It also rotates
// the monthly budget window when needed. On success it returns the CreditSource
// that must be stored on the job for later settle/release.
func HoldCredits(tx *gorm.DB, organization *model.Organization, createdBy string, amount int, timestamp string) (model.CreditSource, error) {
	if amount < 0 {
		return "", ErrInsufficientUserCredits
	}
	if amount == 0 {
		if organization.CreditMode == model.OrganizationCreditModeShared {
			return model.CreditSourceOrganization, nil
		}
		return model.CreditSourcePersonal, nil
	}
	if organization.CreditMode == model.OrganizationCreditModeShared {
		budgetMonth := creditBudgetMonth(timestamp)
		if organization.CreditBudgetMonth != budgetMonth {
			if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{
				"monthly_credits_used": 0,
				"credit_budget_month":  budgetMonth,
			}).Error; err != nil {
				return "", err
			}
			organization.MonthlyCreditsUsed, organization.CreditBudgetMonth = 0, budgetMonth
		}
		if organization.Credits-organization.ReservedCredits < amount {
			return "", ErrInsufficientOrganizationCredits
		}
		if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+organization.ReservedCredits+amount > organization.MonthlyCreditBudget {
			return "", ErrOrganizationCreditBudgetExceeded
		}
		result := tx.Model(&model.Organization{}).Where(
			"id = ? AND credits - reserved_credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + reserved_credits + ? <= monthly_credit_budget)",
			organization.ID, amount, amount,
		).Updates(map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits + ?", amount),
			"updated_at":       timestamp,
		})
		if result.Error != nil {
			return "", result.Error
		}
		if result.RowsAffected != 1 {
			return "", ErrOrganizationCreditBudgetExceeded
		}
		organization.ReservedCredits += amount
		return model.CreditSourceOrganization, nil
	}

	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", createdBy).First(&user).Error; err != nil {
		return "", err
	}
	if user.Credits-user.ReservedCredits < amount {
		return "", ErrInsufficientUserCredits
	}
	result := tx.Model(&model.User{}).Where("id = ? AND credits - reserved_credits >= ?", user.ID, amount).Updates(map[string]any{
		"reserved_credits": gorm.Expr("reserved_credits + ?", amount),
		"updated_at":       timestamp,
	})
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected != 1 {
		return "", ErrInsufficientUserCredits
	}
	return model.CreditSourcePersonal, nil
}

// ReleaseHeldCredits returns outstanding reserved credits without spending them.
func ReleaseHeldCredits(tx *gorm.DB, source model.CreditSource, organizationID, userID string, amount int, timestamp string) error {
	if amount < 0 {
		return ErrInsufficientUserCredits
	}
	if amount == 0 {
		return nil
	}
	switch source {
	case model.CreditSourceOrganization:
		result := tx.Model(&model.Organization{}).Where("id = ? AND reserved_credits >= ?", organizationID, amount).Updates(map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits - ?", amount),
			"updated_at":       timestamp,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientOrganizationCredits
		}
		return nil
	case model.CreditSourcePersonal:
		result := tx.Model(&model.User{}).Where("id = ? AND reserved_credits >= ?", userID, amount).Updates(map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits - ?", amount),
			"updated_at":       timestamp,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientUserCredits
		}
		return nil
	default:
		return ErrInsufficientUserCredits
	}
}

// SettleHeldCredits spends settleAmount from the hold and releases releaseAmount
// of unused hold in one update. settleAmount + releaseAmount must equal the
// outstanding reserved amount being closed.
func SettleHeldCredits(tx *gorm.DB, source model.CreditSource, organizationID, userID string, settleAmount, releaseAmount int, timestamp string) error {
	if settleAmount < 0 || releaseAmount < 0 {
		return ErrInsufficientUserCredits
	}
	total := settleAmount + releaseAmount
	if total == 0 {
		return nil
	}
	switch source {
	case model.CreditSourceOrganization:
		updates := map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits - ?", total),
			"updated_at":       timestamp,
		}
		if settleAmount > 0 {
			updates["credits"] = gorm.Expr("credits - ?", settleAmount)
			updates["monthly_credits_used"] = gorm.Expr("monthly_credits_used + ?", settleAmount)
		}
		result := tx.Model(&model.Organization{}).Where("id = ? AND credits >= ? AND reserved_credits >= ?", organizationID, settleAmount, total).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientOrganizationCredits
		}
		return nil
	case model.CreditSourcePersonal:
		updates := map[string]any{
			"reserved_credits": gorm.Expr("reserved_credits - ?", total),
			"updated_at":       timestamp,
		}
		if settleAmount > 0 {
			updates["credits"] = gorm.Expr("credits - ?", settleAmount)
		}
		result := tx.Model(&model.User{}).Where("id = ? AND credits >= ? AND reserved_credits >= ?", userID, settleAmount, total).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientUserCredits
		}
		return nil
	default:
		return ErrInsufficientUserCredits
	}
}
