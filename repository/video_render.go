package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrVideoRenderRequestConflict = errors.New("video render request conflict")
	ErrVideoRenderStateConflict   = errors.New("video render state conflict")
	ErrVideoRenderQueueFull       = errors.New("video render organization queue full")
)

const maxVideoRenderPendingPerOrganization = 50

func ListVideoRenderJobs(organizationID string, q model.Query) ([]model.VideoRenderJob, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	tx := db.Model(&model.VideoRenderJob{}).Where("organization_id = ?", organizationID)
	if projectID := strings.TrimSpace(q.Type); projectID != "" {
		tx = tx.Where("project_id = ?", projectID)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.VideoRenderJob
	if err := tx.Order("created_at desc").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func GetVideoRenderJob(organizationID, id string) (model.VideoRenderJob, bool, error) {
	db, err := DB()
	if err != nil {
		return model.VideoRenderJob{}, false, err
	}
	var job model.VideoRenderJob
	if err := db.Where("organization_id = ? AND id = ?", organizationID, id).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return job, false, nil
	} else if err != nil {
		return job, false, err
	}
	return job, true, nil
}

func CreateVideoRenderJob(job model.VideoRenderJob) (model.VideoRenderJob, error) {
	db, err := DB()
	if err != nil {
		return job, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", job.OrganizationID).Error; err != nil {
			return err
		}
		var existing model.VideoRenderJob
		if err := tx.Where("organization_id = ? AND request_id = ?", job.OrganizationID, job.RequestID).First(&existing).Error; err == nil {
			if existing.RequestHash != job.RequestHash {
				return ErrVideoRenderRequestConflict
			}
			job = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var pending int64
		if err := tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND status IN ?", job.OrganizationID, []model.VideoRenderJobStatus{
			model.VideoRenderStatusQueued, model.VideoRenderStatusMaterializing, model.VideoRenderStatusRendering, model.VideoRenderStatusArchiving,
		}).Count(&pending).Error; err != nil {
			return err
		}
		if pending >= maxVideoRenderPendingPerOrganization {
			return ErrVideoRenderQueueFull
		}
		if err := reserveVideoRenderCredits(tx, &organization, &job); err != nil {
			return err
		}
		return tx.Create(&job).Error
	})
	return job, err
}

func reserveVideoRenderCredits(tx *gorm.DB, organization *model.Organization, job *model.VideoRenderJob) error {
	job.ReservedCredits, job.SettledCredits, job.ActualCredits, job.ReleasedCredits = job.EstimatedCredits, 0, 0, 0
	if organization.CreditMode == model.OrganizationCreditModeShared {
		job.CreditSource = model.CreditSourceOrganization
		budgetMonth := batchCreditBudgetMonth(job.CreatedAt)
		if organization.CreditBudgetMonth != budgetMonth {
			if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{"monthly_credits_used": 0, "credit_budget_month": budgetMonth}).Error; err != nil {
				return err
			}
			organization.MonthlyCreditsUsed, organization.CreditBudgetMonth = 0, budgetMonth
		}
		if organization.Credits-organization.ReservedCredits < job.EstimatedCredits {
			return ErrInsufficientOrganizationCredits
		}
		if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+organization.ReservedCredits+job.EstimatedCredits > organization.MonthlyCreditBudget {
			return ErrOrganizationCreditBudgetExceeded
		}
		result := tx.Model(&model.Organization{}).Where("id = ? AND credits - reserved_credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + reserved_credits + ? <= monthly_credit_budget)", organization.ID, job.EstimatedCredits, job.EstimatedCredits).Updates(map[string]any{"reserved_credits": gorm.Expr("reserved_credits + ?", job.EstimatedCredits), "updated_at": job.UpdatedAt})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationCreditBudgetExceeded
		}
		organization.ReservedCredits += job.EstimatedCredits
		return nil
	}
	job.CreditSource = model.CreditSourcePersonal
	var user model.User
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", job.CreatedBy).First(&user).Error; err != nil {
		return err
	}
	if user.Credits-user.ReservedCredits < job.EstimatedCredits {
		return ErrInsufficientUserCredits
	}
	result := tx.Model(&model.User{}).Where("id = ? AND credits - reserved_credits >= ?", user.ID, job.EstimatedCredits).Updates(map[string]any{"reserved_credits": gorm.Expr("reserved_credits + ?", job.EstimatedCredits), "updated_at": job.UpdatedAt})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrInsufficientUserCredits
	}
	return nil
}

func releaseVideoRenderCredits(tx *gorm.DB, job *model.VideoRenderJob, timestamp string) error {
	outstanding := job.ReservedCredits - job.SettledCredits - job.ReleasedCredits
	if outstanding < 0 {
		return ErrVideoRenderStateConflict
	}
	if outstanding == 0 {
		return nil
	}
	if job.CreditSource == model.CreditSourceOrganization {
		result := tx.Model(&model.Organization{}).Where("id = ? AND reserved_credits >= ?", job.OrganizationID, outstanding).Updates(map[string]any{"reserved_credits": gorm.Expr("reserved_credits - ?", outstanding), "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVideoRenderStateConflict
		}
	} else {
		result := tx.Model(&model.User{}).Where("id = ? AND reserved_credits >= ?", job.CreatedBy, outstanding).Updates(map[string]any{"reserved_credits": gorm.Expr("reserved_credits - ?", outstanding), "updated_at": timestamp})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVideoRenderStateConflict
		}
	}
	job.ReleasedCredits += outstanding
	return tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND id = ?", job.OrganizationID, job.ID).Update("released_credits", job.ReleasedCredits).Error
}

func settleVideoRenderCredits(tx *gorm.DB, job *model.VideoRenderJob, actual int, timestamp string) error {
	if actual < 0 || actual > job.ReservedCredits {
		return ErrVideoRenderStateConflict
	}
	release := job.ReservedCredits - job.SettledCredits - job.ReleasedCredits - actual
	if release < 0 {
		return ErrVideoRenderStateConflict
	}
	if job.CreditSource == model.CreditSourceOrganization {
		updates := map[string]any{
			"credits":              gorm.Expr("credits - ?", actual),
			"reserved_credits":     gorm.Expr("reserved_credits - ?", actual+release),
			"monthly_credits_used": gorm.Expr("monthly_credits_used + ?", actual),
			"updated_at":           timestamp,
		}
		result := tx.Model(&model.Organization{}).Where("id = ? AND credits >= ? AND reserved_credits >= ?", job.OrganizationID, actual, actual+release).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVideoRenderStateConflict
		}
	} else {
		result := tx.Model(&model.User{}).Where("id = ? AND credits >= ? AND reserved_credits >= ?", job.CreatedBy, actual, actual+release).Updates(map[string]any{
			"credits":          gorm.Expr("credits - ?", actual),
			"reserved_credits": gorm.Expr("reserved_credits - ?", actual+release),
			"updated_at":       timestamp,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrVideoRenderStateConflict
		}
	}
	job.SettledCredits, job.ReleasedCredits, job.ActualCredits = actual, job.ReleasedCredits+release, actual
	return nil
}

func ClaimNextVideoRenderJob(timestamp, leaseExpiresAt, leaseToken string, maxTenantRunning int) (model.VideoRenderJob, bool, error) {
	db, err := DB()
	if err != nil {
		return model.VideoRenderJob{}, false, err
	}
	if maxTenantRunning < 1 {
		maxTenantRunning = 1
	}
	var job model.VideoRenderJob
	claimed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var candidates []model.VideoRenderJob
		if err := tx.Where("status = ? AND (next_attempt_at = '' OR next_attempt_at <= ?)", model.VideoRenderStatusQueued, timestamp).
			Or("status IN ? AND lease_expires_at <> '' AND lease_expires_at <= ? AND attempt < ?", []model.VideoRenderJobStatus{model.VideoRenderStatusMaterializing, model.VideoRenderStatusRendering, model.VideoRenderStatusArchiving}, timestamp, 5).
			Order("created_at asc").Limit(20).Find(&candidates).Error; err != nil {
			return err
		}
		for _, candidate := range candidates {
			var running int64
			if err := tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND status IN ? AND lease_expires_at > ?", candidate.OrganizationID, []model.VideoRenderJobStatus{model.VideoRenderStatusMaterializing, model.VideoRenderStatusRendering, model.VideoRenderStatusArchiving}, timestamp).Count(&running).Error; err != nil {
				return err
			}
			if running >= int64(maxTenantRunning) {
				continue
			}
			result := tx.Model(&model.VideoRenderJob{}).
				Where("id = ? AND organization_id = ? AND ((status = ? AND (next_attempt_at = '' OR next_attempt_at <= ?)) OR (status IN ? AND lease_expires_at <> '' AND lease_expires_at <= ? AND attempt < ?))",
					candidate.ID, candidate.OrganizationID, model.VideoRenderStatusQueued, timestamp,
					[]model.VideoRenderJobStatus{model.VideoRenderStatusMaterializing, model.VideoRenderStatusRendering, model.VideoRenderStatusArchiving}, timestamp, 5).
				Updates(map[string]any{
					"status": model.VideoRenderStatusMaterializing, "phase": "materializing",
					"attempt": gorm.Expr("attempt + 1"), "lease_token": leaseToken, "lease_owner": leaseToken,
					"lease_expires_at": leaseExpiresAt, "started_at": timestamp, "error_message": "", "error_category": "",
					"next_attempt_at": "", "updated_at": timestamp,
				})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				continue
			}
			if err := tx.Where("id = ?", candidate.ID).First(&job).Error; err != nil {
				return err
			}
			claimed = true
			return nil
		}
		return nil
	})
	return job, claimed, err
}

func RenewVideoRenderLease(organizationID, id, leaseToken, leaseExpiresAt, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.VideoRenderJob{}).
		Where("organization_id = ? AND id = ? AND lease_token = ? AND status IN ?", organizationID, id, leaseToken, []model.VideoRenderJobStatus{model.VideoRenderStatusMaterializing, model.VideoRenderStatusRendering, model.VideoRenderStatusArchiving}).
		Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "updated_at": timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVideoRenderStateConflict
	}
	return nil
}

func AdvanceVideoRenderPhase(organizationID, id, leaseToken, phase string, status model.VideoRenderJobStatus, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	result := db.Model(&model.VideoRenderJob{}).
		Where("organization_id = ? AND id = ? AND lease_token = ?", organizationID, id, leaseToken).
		Updates(map[string]any{"phase": phase, "status": status, "updated_at": timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVideoRenderStateConflict
	}
	return nil
}

func FinishVideoRenderJobSuccess(organizationID, id, leaseToken, resultStorageKey string, resultSize int64, actualCredits int, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.VideoRenderJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND lease_token = ?", organizationID, id, leaseToken).First(&job).Error; err != nil {
			return err
		}
		if job.Status != model.VideoRenderStatusArchiving && job.Status != model.VideoRenderStatusRendering {
			return ErrVideoRenderStateConflict
		}
		if err := settleVideoRenderCredits(tx, &job, actualCredits, timestamp); err != nil {
			return err
		}
		return tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND id = ? AND lease_token = ?", organizationID, id, leaseToken).Updates(map[string]any{
			"status": model.VideoRenderStatusPendingReview, "phase": "pending_review",
			"result_storage_key": resultStorageKey, "result_size": resultSize,
			"settled_credits": job.SettledCredits, "released_credits": job.ReleasedCredits, "actual_credits": job.ActualCredits,
			"review_status": model.VideoRenderReviewPending, "lease_token": "", "lease_expires_at": "",
			"finished_at": timestamp, "updated_at": timestamp,
		}).Error
	})
}

func FinishVideoRenderJobFailure(organizationID, id, leaseToken, category, message, nextAttemptAt, timestamp string, terminal bool) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.VideoRenderJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND lease_token = ?", organizationID, id, leaseToken).First(&job).Error; err != nil {
			return err
		}
		updates := map[string]any{
			"error_category": category, "error_message": message, "updated_at": timestamp, "lease_token": "", "lease_expires_at": "",
		}
		if terminal || job.Attempt >= 5 {
			if err := releaseVideoRenderCredits(tx, &job, timestamp); err != nil {
				return err
			}
			updates["status"] = model.VideoRenderStatusFailed
			updates["phase"] = "failed"
			updates["released_credits"] = job.ReleasedCredits
			updates["finished_at"] = timestamp
			updates["next_attempt_at"] = ""
		} else {
			updates["status"] = model.VideoRenderStatusQueued
			updates["phase"] = "queued"
			updates["next_attempt_at"] = nextAttemptAt
		}
		return tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND id = ?", organizationID, id).Updates(updates).Error
	})
}

func CancelVideoRenderJob(organizationID, id, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.VideoRenderJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&job).Error; err != nil {
			return err
		}
		switch job.Status {
		case model.VideoRenderStatusCompleted, model.VideoRenderStatusFailed, model.VideoRenderStatusCancelled, model.VideoRenderStatusPendingReview:
			return ErrVideoRenderStateConflict
		}
		if err := releaseVideoRenderCredits(tx, &job, timestamp); err != nil {
			return err
		}
		return tx.Model(&model.VideoRenderJob{}).Where("organization_id = ? AND id = ?", organizationID, id).Updates(map[string]any{
			"status": model.VideoRenderStatusCancelled, "phase": "cancelled",
			"released_credits": job.ReleasedCredits, "lease_token": "", "lease_expires_at": "",
			"finished_at": timestamp, "updated_at": timestamp,
		}).Error
	})
}

func ReviewVideoRenderJob(organizationID, id string, status model.VideoRenderReviewStatus, timestamp string) error {
	db, err := DB()
	if err != nil {
		return err
	}
	finalStatus := model.VideoRenderStatusPendingReview
	if status == model.VideoRenderReviewApproved {
		finalStatus = model.VideoRenderStatusCompleted
	} else if status == model.VideoRenderReviewRejected {
		finalStatus = model.VideoRenderStatusFailed
	} else {
		return ErrVideoRenderStateConflict
	}
	result := db.Model(&model.VideoRenderJob{}).
		Where("organization_id = ? AND id = ? AND status = ?", organizationID, id, model.VideoRenderStatusPendingReview).
		Updates(map[string]any{"review_status": status, "status": finalStatus, "phase": string(finalStatus), "updated_at": timestamp})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrVideoRenderStateConflict
	}
	return nil
}

func VideoRenderRequestHash(projectID string, versionNumber int, timelineJSON, outputJSON string) string {
	payload := fmt.Sprintf("%s\x00%d\x00%s\x00%s", projectID, versionNumber, timelineJSON, outputJSON)
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
