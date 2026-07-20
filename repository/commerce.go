package repository

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDefaultOrganizationAlreadyCreated = errors.New("default organization already created")
	ErrOrganizationInvitationUnavailable = errors.New("organization invitation unavailable")
	ErrOrganizationOwnershipChanged      = errors.New("organization ownership changed")
	ErrBrandInUse                        = errors.New("brand is in use")
	ErrProductInUse                      = errors.New("product is in use")
	ErrProductSKUInUse                   = errors.New("product sku is in use")
	ErrBrandNameConflict                 = errors.New("brand name conflict")
	ErrProductCodeConflict               = errors.New("product code conflict")
	ErrProductSKUCodeConflict            = errors.New("product sku code conflict")
	ErrBatchProductionLeaseLost          = errors.New("batch production lease lost")
	ErrBatchProductionStateConflict      = errors.New("batch production state conflict")
	ErrBatchProductionSnapshotTooLarge   = errors.New("batch production snapshot too large")
)

func EnsureDefaultOrganization(user model.AuthUser, organizationID string, memberID string, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.Organization, model.OrganizationMember, error) {
	db, err := DB()
	if err != nil {
		return model.Organization{}, model.OrganizationMember{}, err
	}
	var organization model.Organization
	var membership model.OrganizationMember
	err = db.Transaction(func(tx *gorm.DB) error {
		var savedUser model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&savedUser, "id = ?", user.ID).Error; err != nil {
			return err
		}
		if savedUser.OrganizationID != "" {
			if err := tx.First(&organization, "id = ? AND status = ?", savedUser.OrganizationID, "active").Error; err != nil {
				return err
			}
			return tx.First(&membership, "organization_id = ? AND user_id = ?", organization.ID, user.ID).Error
		}
		name := strings.TrimSpace(user.DisplayName)
		if name == "" {
			name = strings.TrimSpace(user.Username)
		}
		organization = model.Organization{ID: organizationID, Name: name + "的企业", Slug: organizationID, Status: "active", CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
		membership = model.OrganizationMember{ID: memberID, OrganizationID: organization.ID, UserID: user.ID, Role: model.OrganizationRoleOwner, CreatedAt: timestamp, UpdatedAt: timestamp}
		if err := tx.Create(&organization).Error; err != nil {
			return err
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ? AND organization_id = ''", user.ID).Update("organization_id", organization.ID)
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrDefaultOrganizationAlreadyCreated }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	if errors.Is(err, ErrDefaultOrganizationAlreadyCreated) {
		organization, membership, ok, readErr := GetOrganizationContext(user.ID)
		if readErr != nil { return model.Organization{}, model.OrganizationMember{}, readErr }
		if ok { return organization, membership, nil }
	}
	return organization, membership, err
}

func GetOrganizationContext(userID string) (model.Organization, model.OrganizationMember, bool, error) {
	db, err := DB()
	if err != nil {
		return model.Organization{}, model.OrganizationMember{}, false, err
	}
	var user model.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return model.Organization{}, model.OrganizationMember{}, false, err
	}
	var membership model.OrganizationMember
	findMembership := func(organizationID string) error {
		tx := db.Table("organization_members AS m").Select("m.*").Joins("JOIN organizations AS o ON o.id = m.organization_id AND o.status = ?", "active").Where("m.user_id = ?", userID)
		if organizationID != "" { tx = tx.Where("m.organization_id = ?", organizationID) }
		return tx.Order("m.created_at asc").First(&membership).Error
	}
	err = gorm.ErrRecordNotFound
	if user.OrganizationID != "" {
		err = findMembership(user.OrganizationID)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) { err = findMembership("") }
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Organization{}, model.OrganizationMember{}, false, nil
	}
	if err != nil {
		return model.Organization{}, model.OrganizationMember{}, false, err
	}
	var organization model.Organization
	err = db.First(&organization, "id = ?", membership.OrganizationID).Error
	return organization, membership, err == nil, err
}

func GetOrganization(id string) (model.Organization, bool, error) {
	db, err := DB()
	if err != nil { return model.Organization{}, false, err }
	var item model.Organization
	err = db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.Organization{}, false, nil }
	return item, err == nil, err
}

func CreateOrganization(organization model.Organization, membership model.OrganizationMember, setCurrent bool, auditLogs ...model.OrganizationAuditLog) (model.Organization, error) {
	db, err := DB()
	if err != nil { return organization, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&organization).Error; err != nil { return err }
		if err := tx.Create(&membership).Error; err != nil { return err }
		if setCurrent { if err := tx.Model(&model.User{}).Where("id = ?", membership.UserID).Update("organization_id", organization.ID).Error; err != nil { return err } }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return organization, err
}

func ListUserOrganizations(userID string) ([]model.OrganizationSummary, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.OrganizationSummary
	err = db.Table("organization_members AS m").
		Select("o.id, o.name, o.slug, o.status, m.role, o.created_at").
		Joins("JOIN organizations AS o ON o.id = m.organization_id AND o.status = ?", "active").
		Where("m.user_id = ?", userID).Order("o.created_at asc").Scan(&items).Error
	return items, err
}

func SwitchUserOrganization(userID string, organizationID string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var membership model.OrganizationMember
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND organization_id = ?", userID, organizationID).First(&membership).Error; err != nil { return err }
		var organization model.Organization
		if err := tx.Where("id = ? AND status = ?", organizationID, "active").First(&organization).Error; err != nil { return err }
		if err := tx.Model(&model.User{}).Where("id = ?", userID).Update("organization_id", organizationID).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListOrganizationInvitations(organizationID string, timestamp string) ([]model.OrganizationInvitation, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.OrganizationInvitation
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrganizationInvitation{}).Where("organization_id = ? AND status = ? AND expires_at <= ?", organizationID, model.OrganizationInvitationPending, timestamp).Updates(map[string]any{"status": model.OrganizationInvitationExpired, "updated_at": timestamp}).Error; err != nil { return err }
		return tx.Where("organization_id = ? AND status = ? AND expires_at > ?", organizationID, model.OrganizationInvitationPending, timestamp).Order("created_at desc").Find(&items).Error
	})
	return items, err
}

func ListUserInvitations(email string, timestamp string) ([]model.OrganizationInvitation, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.OrganizationInvitation
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrganizationInvitation{}).Where("LOWER(email) = ? AND status = ? AND expires_at <= ?", strings.ToLower(email), model.OrganizationInvitationPending, timestamp).Updates(map[string]any{"status": model.OrganizationInvitationExpired, "updated_at": timestamp}).Error; err != nil { return err }
		return tx.Table("organization_invitations AS i").Select("i.*, o.name AS organization_name").Joins("JOIN organizations AS o ON o.id = i.organization_id AND o.status = ?", "active").Where("LOWER(i.email) = ? AND i.status = ? AND i.expires_at > ?", strings.ToLower(email), model.OrganizationInvitationPending, timestamp).Order("i.created_at desc").Scan(&items).Error
	})
	return items, err
}

func SaveOrganizationInvitation(invitation model.OrganizationInvitation, outbox model.OrganizationEmailOutbox, auditLogs ...model.OrganizationAuditLog) (model.OrganizationInvitation, error) {
	db, err := DB()
	if err != nil { return invitation, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", invitation.OrganizationID).Error; err != nil { return err }
		if err := tx.Model(&model.OrganizationInvitation{}).Where("organization_id = ? AND LOWER(email) = ? AND status = ? AND expires_at <= ?", invitation.OrganizationID, strings.ToLower(invitation.Email), model.OrganizationInvitationPending, invitation.CreatedAt).Updates(map[string]any{"status": model.OrganizationInvitationExpired, "updated_at": invitation.CreatedAt}).Error; err != nil { return err }
		var total int64
		if err := tx.Model(&model.OrganizationInvitation{}).Where("organization_id = ? AND LOWER(email) = ? AND status = ?", invitation.OrganizationID, strings.ToLower(invitation.Email), model.OrganizationInvitationPending).Count(&total).Error; err != nil { return err }
		if total > 0 { return ErrOrganizationInvitationUnavailable }
		if err := tx.Create(&invitation).Error; err != nil { return err }
		if err := tx.Create(&outbox).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return invitation, err
}

func ClaimOrganizationEmailOutbox(timestamp string, leaseExpiresAt string, leaseToken string) (model.OrganizationEmailOutbox, bool, error) {
	db, err := DB()
	if err != nil { return model.OrganizationEmailOutbox{}, false, err }
	var item model.OrganizationEmailOutbox
	err = db.Transaction(func(tx *gorm.DB) error {
		unavailableInvitations := tx.Model(&model.OrganizationInvitation{}).Select("id").Where("status <> ? OR expires_at <= ?", model.OrganizationInvitationPending, timestamp)
		if err := tx.Model(&model.OrganizationEmailOutbox{}).Where("status IN ? AND invitation_id IN (?)", []string{"pending", "failed"}, unavailableInvitations).Updates(map[string]any{"status": "cancelled", "updated_at": timestamp}).Error; err != nil { return err }
		query := tx.Table("organization_email_outboxes AS e").Select("e.*").Joins("JOIN organization_invitations AS i ON i.id = e.invitation_id AND i.status = ? AND i.expires_at > ?", model.OrganizationInvitationPending, timestamp).Clauses(clause.Locking{Strength: "UPDATE"}).Where("(e.status IN ? AND e.attempts < ? AND e.next_attempt_at <= ?) OR (e.status = ? AND e.lease_expires_at <= ?)", []string{"pending", "failed"}, 10, timestamp, "processing", timestamp).Order("e.created_at asc")
		if err := query.First(&item).Error; err != nil { return err }
		return tx.Model(&model.OrganizationEmailOutbox{}).Where("id = ?", item.ID).Updates(map[string]any{"status": "processing", "attempts": gorm.Expr("attempts + 1"), "lease_token": leaseToken, "lease_expires_at": leaseExpiresAt, "updated_at": timestamp}).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.OrganizationEmailOutbox{}, false, nil }
	if err != nil { return model.OrganizationEmailOutbox{}, false, err }
	item.Status, item.LeaseToken, item.LeaseExpiresAt, item.UpdatedAt = "processing", leaseToken, leaseExpiresAt, timestamp
	item.Attempts++
	return item, true, nil
}

func FinishOrganizationEmailOutbox(item model.OrganizationEmailOutbox, succeeded bool, errorMessage string, nextAttemptAt string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	values := map[string]any{"lease_token": "", "lease_expires_at": "", "updated_at": timestamp}
	if succeeded {
		values["status"], values["sent_at"], values["last_error"] = "sent", timestamp, ""
	} else {
		status := "failed"
		if item.Attempts >= 10 { status = "dead" }
		values["status"], values["last_error"], values["next_attempt_at"] = status, errorMessage, nextAttemptAt
	}
	result := db.Model(&model.OrganizationEmailOutbox{}).Where("id = ? AND status = ? AND lease_token = ?", item.ID, "processing", item.LeaseToken).Updates(values)
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
	return nil
}

func RevokeOrganizationInvitation(organizationID string, id string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.OrganizationInvitation{}).Where("organization_id = ? AND id = ? AND status = ?", organizationID, id, model.OrganizationInvitationPending).Updates(map[string]any{"status": model.OrganizationInvitationRevoked, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationInvitationUnavailable }
		if err := tx.Model(&model.OrganizationEmailOutbox{}).Where("organization_id = ? AND invitation_id = ? AND status IN ?", organizationID, id, []string{"pending", "failed", "processing"}).Updates(map[string]any{"status": "cancelled", "lease_token": "", "lease_expires_at": "", "updated_at": timestamp}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func GetOrganizationInvitation(id string) (model.OrganizationInvitation, bool, error) {
	db, err := DB()
	if err != nil { return model.OrganizationInvitation{}, false, err }
	var item model.OrganizationInvitation
	err = db.First(&item, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.OrganizationInvitation{}, false, nil }
	return item, err == nil, err
}

func AcceptOrganizationInvitation(invitation model.OrganizationInvitation, membership model.OrganizationMember, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", invitation.OrganizationID, "active").First(&organization).Error; err != nil { return err }
		result := tx.Model(&model.OrganizationInvitation{}).
			Where("id = ? AND organization_id = ? AND email = ? AND status = ? AND expires_at > ?", invitation.ID, invitation.OrganizationID, invitation.Email, model.OrganizationInvitationPending, timestamp).
			Updates(map[string]any{"status": model.OrganizationInvitationAccepted, "accepted_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationInvitationUnavailable }
		if err := tx.Model(&model.OrganizationEmailOutbox{}).Where("organization_id = ? AND invitation_id = ? AND status IN ?", invitation.OrganizationID, invitation.ID, []string{"pending", "failed", "processing"}).Updates(map[string]any{"status": "cancelled", "lease_token": "", "lease_expires_at": "", "updated_at": timestamp}).Error; err != nil { return err }
		if err := tx.Create(&membership).Error; err != nil { return err }
		if err := tx.Model(&model.User{}).Where("id = ?", membership.UserID).Update("organization_id", membership.OrganizationID).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func saveOrganizationAuditLogs(tx *gorm.DB, logs []model.OrganizationAuditLog) error {
	if len(logs) == 0 { return nil }
	return tx.Create(&logs).Error
}

func ListOrganizationAuditLogs(organizationID string, q model.Query) ([]model.OrganizationAuditLog, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.OrganizationAuditLog{}).Where("organization_id = ?", organizationID)
	if q.Keyword != "" { tx = tx.Where("action LIKE ? OR resource_type LIKE ? OR detail LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.OrganizationAuditLog
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func SaveOrganization(organization model.Organization, auditLogs ...model.OrganizationAuditLog) (model.Organization, error) {
	db, err := DB()
	if err != nil {
		return organization, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var saved model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&saved, "id = ?", organization.ID).Error; err != nil { return err }
		result := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{"name": organization.Name, "updated_at": organization.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return organization, err
}

func ListOrganizationMembers(organizationID string, q model.Query) ([]model.OrganizationMemberView, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Table("organization_members AS m").Joins("JOIN users AS u ON u.id = m.user_id").Where("m.organization_id = ?", organizationID)
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("u.username LIKE ? OR u.display_name LIKE ? OR u.email LIKE ?", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.OrganizationMemberView
	err = tx.
		Select("m.id, m.user_id, u.username, u.display_name, u.email, u.avatar_url, m.role, m.created_at").
		Order("m.created_at asc").Offset(q.Offset()).Limit(q.PageSize).Scan(&items).Error
	return items, total, err
}

func SaveOrganizationMember(member model.OrganizationMember, auditLogs ...model.OrganizationAuditLog) (model.OrganizationMember, error) {
	db, err := DB()
	if err != nil {
		return member, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", member.OrganizationID).Error; err != nil { return err }
		result := tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role <> ?", member.OrganizationID, member.ID, model.OrganizationRoleOwner).Updates(map[string]any{"role": member.Role, "updated_at": member.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationOwnershipChanged }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return member, err
}

func GetOrganizationMember(organizationID string, userID string) (model.OrganizationMember, bool, error) {
	db, err := DB()
	if err != nil {
		return model.OrganizationMember{}, false, err
	}
	var item model.OrganizationMember
	err = db.Where("organization_id = ? AND user_id = ?", organizationID, userID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.OrganizationMember{}, false, nil
	}
	return item, err == nil, err
}

func GetOrganizationMemberByID(organizationID string, id string) (model.OrganizationMember, bool, error) {
	db, err := DB()
	if err != nil { return model.OrganizationMember{}, false, err }
	var item model.OrganizationMember
	err = db.Where("organization_id = ? AND id = ?", organizationID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.OrganizationMember{}, false, nil }
	return item, err == nil, err
}

func DeleteOrganizationMember(organizationID string, memberID string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var member model.OrganizationMember
		if err := tx.Where("organization_id = ? AND id = ? AND role <> ?", organizationID, memberID, model.OrganizationRoleOwner).First(&member).Error; err != nil { return err }
		result := tx.Where("organization_id = ? AND id = ? AND role <> ?", organizationID, memberID, model.OrganizationRoleOwner).Delete(&model.OrganizationMember{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationOwnershipChanged }
		if err := tx.Model(&model.User{}).Where("id = ? AND organization_id = ?", member.UserID, organizationID).Update("organization_id", "").Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func TransferOrganizationOwnership(organizationID string, ownerMemberID string, targetMemberID string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var owner model.OrganizationMember
		if err := tx.Where("organization_id = ? AND role = ?", organizationID, model.OrganizationRoleOwner).First(&owner).Error; err != nil { return err }
		if owner.ID != ownerMemberID { return ErrOrganizationOwnershipChanged }
		var target model.OrganizationMember
		if err := tx.Where("organization_id = ? AND id = ? AND role <> ?", organizationID, targetMemberID, model.OrganizationRoleOwner).First(&target).Error; err != nil { return err }
		result := tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role = ?", organizationID, owner.ID, model.OrganizationRoleOwner).Updates(map[string]any{"role": model.OrganizationRoleAdmin, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationOwnershipChanged }
		result = tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role <> ?", organizationID, target.ID, model.OrganizationRoleOwner).Updates(map[string]any{"role": model.OrganizationRoleOwner, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationOwnershipChanged }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func OrganizationStats(organizationID string) (model.OrganizationWorkspaceStats, error) {
	db, err := DB()
	if err != nil {
		return model.OrganizationWorkspaceStats{}, err
	}
	stats := model.OrganizationWorkspaceStats{}
	counts := []struct {
		model any
		value *int64
	}{
		{&model.Brand{}, &stats.Brands},
		{&model.Product{}, &stats.Products},
		{&model.ProductSKU{}, &stats.SKUs},
		{&model.BatchProductionJob{}, &stats.BatchJobs},
	}
	for _, item := range counts {
		if err := db.Model(item.model).Where("organization_id = ?", organizationID).Count(item.value).Error; err != nil {
			return stats, err
		}
	}
	err = db.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status IN ?", organizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&stats.PendingItems).Error
	return stats, err
}

func ListBrands(organizationID string, q model.Query) ([]model.Brand, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.Brand{}).Where("organization_id = ?", organizationID)
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("name LIKE ? OR tone LIKE ? OR guidelines LIKE ?", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Brand
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func SaveBrand(item model.Brand, create bool, auditLogs ...model.OrganizationAuditLog) (model.Brand, error) {
	db, err := DB()
	if err != nil {
		return item, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		if create {
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrBrandNameConflict }; return err }
		} else {
			var saved model.Brand
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).First(&saved).Error; err != nil { return err }
			result := tx.Model(&model.Brand{}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrBrandNameConflict }; return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func GetBrand(organizationID string, id string) (model.Brand, bool, error) {
	db, err := DB()
	if err != nil { return model.Brand{}, false, err }
	var item model.Brand
	err = db.Where("organization_id = ? AND id = ?", organizationID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.Brand{}, false, nil }
	return item, err == nil, err
}

func DeleteBrand(organizationID string, id string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var brand model.Brand
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&brand).Error; err != nil { return err }
		var references int64
		if err := tx.Model(&model.Product{}).Where("organization_id = ? AND brand_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references == 0 {
			if err := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND brand_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		}
		if references > 0 { return ErrBrandInUse }
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, id).Delete(&model.Brand{}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListProducts(organizationID string, q model.Query) ([]model.Product, int64, error) {
	db, err := DB()
	if err != nil {
		return nil, 0, err
	}
	q.Normalize()
	tx := db.Model(&model.Product{}).Joins("LEFT JOIN brands ON brands.id = products.brand_id AND brands.organization_id = products.organization_id").Where("products.organization_id = ?", organizationID)
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		tx = tx.Where("products.name LIKE ? OR products.code LIKE ? OR products.category LIKE ?", like, like, like)
	}
	if q.Type != "" && q.Type != "all" {
		tx = tx.Where("products.status = ?", q.Type)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var items []model.Product
	if err := tx.Select("products.*, brands.name AS brand_name").Order("products.updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	if len(items) == 0 {
		return items, total, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	type countRow struct { ProductID string; Total int64 }
	var rows []countRow
	if err := db.Model(&model.ProductSKU{}).Select("product_id, COUNT(*) AS total").Where("organization_id = ? AND product_id IN ?", organizationID, ids).Group("product_id").Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows { counts[row.ProductID] = row.Total }
	for index := range items { items[index].SKUCount = counts[items[index].ID] }
	return items, total, nil
}

func SaveProduct(item model.Product, create bool, auditLogs ...model.OrganizationAuditLog) (model.Product, error) {
	db, err := DB()
	if err != nil { return item, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		if item.BrandID != "" {
			var brand model.Brand
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.BrandID).First(&brand).Error; err != nil { return err }
		}
		if create {
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrProductCodeConflict }; return err }
		} else {
			var saved model.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).First(&saved).Error; err != nil { return err }
			result := tx.Model(&model.Product{}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrProductCodeConflict }; return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func GetProduct(organizationID string, id string) (model.Product, bool, error) {
	db, err := DB()
	if err != nil { return model.Product{}, false, err }
	var item model.Product
	err = db.Where("organization_id = ? AND id = ?", organizationID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.Product{}, false, nil }
	return item, err == nil, err
}

func DeleteProduct(organizationID string, id string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&product).Error; err != nil { return err }
		var references int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND product_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references > 0 { return ErrProductInUse }
		if err := tx.Where("organization_id = ? AND product_id = ?", organizationID, id).Delete(&model.ProductSKU{}).Error; err != nil { return err }
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, id).Delete(&model.Product{}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListProductSKUs(organizationID string, productID string, q model.Query) ([]model.ProductSKU, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.ProductSKU{}).Where("organization_id = ? AND product_id = ?", organizationID, productID)
	if q.Keyword != "" { tx = tx.Where("name LIKE ? OR code LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	if q.Type != "" && q.Type != "all" { tx = tx.Where("status = ?", q.Type) }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.ProductSKU
	err = tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func ListProductSKUsByProductIDs(organizationID string, productIDs []string) ([]model.ProductSKU, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.ProductSKU
	err = db.Where("organization_id = ? AND product_id IN ?", organizationID, productIDs).Order("created_at asc").Limit(5001).Find(&items).Error
	return items, err
}

func SaveProductSKU(item model.ProductSKU, create bool, auditLogs ...model.OrganizationAuditLog) (model.ProductSKU, error) {
	db, err := DB()
	if err != nil { return item, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ProductID).First(&product).Error; err != nil { return err }
		if create {
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrProductSKUCodeConflict }; return err }
		} else {
			var saved model.ProductSKU
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND product_id = ?", item.OrganizationID, item.ID, item.ProductID).First(&saved).Error; err != nil { return err }
			result := tx.Model(&model.ProductSKU{}).Where("organization_id = ? AND id = ? AND product_id = ?", item.OrganizationID, item.ID, item.ProductID).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrProductSKUCodeConflict }; return result.Error }
			if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func GetProductSKU(organizationID string, id string) (model.ProductSKU, bool, error) {
	db, err := DB()
	if err != nil { return model.ProductSKU{}, false, err }
	var item model.ProductSKU
	err = db.Where("organization_id = ? AND id = ?", organizationID, id).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.ProductSKU{}, false, nil }
	return item, err == nil, err
}

func DeleteProductSKU(organizationID string, id string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var sku model.ProductSKU
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, id).First(&sku).Error; err != nil { return err }
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, sku.ProductID).First(&product).Error; err != nil { return err }
		var references int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND sku_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references > 0 { return ErrProductSKUInUse }
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, id).Delete(&model.ProductSKU{}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListBatchProductionJobs(organizationID string, q model.Query) ([]model.BatchProductionJob, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.BatchProductionJob{}).Where("organization_id = ?", organizationID)
	if q.Keyword != "" { tx = tx.Where("name LIKE ?", "%"+q.Keyword+"%") }
	if q.Type != "" && q.Type != "all" { tx = tx.Where("status = ?", q.Type) }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.BatchProductionJob
	err = tx.Order("created_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func CreateBatchProductionJob(job model.BatchProductionJob, items []model.BatchProductionItem, auditLogs ...model.OrganizationAuditLog) (model.BatchProductionJob, error) {
	db, err := DB()
	if err != nil { return job, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var brand *model.Brand
		if job.BrandID != "" {
			var saved model.Brand
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", job.OrganizationID, job.BrandID).First(&saved).Error; err != nil { return err }
			brand = &saved
		}
		var products []model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id IN ?", job.OrganizationID, job.ProductIDs).Order("id asc").Find(&products).Error; err != nil { return err }
		if len(products) != len(job.ProductIDs) { return gorm.ErrRecordNotFound }
		productByID := make(map[string]model.Product, len(products))
		for _, product := range products { productByID[product.ID] = product }
		skuIDs := make([]string, 0, len(items))
		for _, item := range items { if item.SKUID != "" { skuIDs = append(skuIDs, item.SKUID) } }
		skuByID := make(map[string]model.ProductSKU, len(skuIDs))
		if len(skuIDs) > 0 {
			var skus []model.ProductSKU
			if err := tx.Where("organization_id = ? AND id IN ?", job.OrganizationID, skuIDs).Find(&skus).Error; err != nil { return err }
			for _, sku := range skus { skuByID[sku.ID] = sku }
		}
		snapshots := make([]model.BatchProductionSnapshot, 0, 1+len(products)+len(skuByID))
		totalSnapshotBytes := 0
		appendSnapshot := func(kind string, resourceID string, value any) (string, error) {
			data, err := json.Marshal(value)
			if err != nil { return "", err }
			totalSnapshotBytes += len(data)
			if totalSnapshotBytes > 16<<20 { return "", ErrBatchProductionSnapshotTooLarge }
			id := job.ID + ":" + kind + ":" + resourceID
			snapshots = append(snapshots, model.BatchProductionSnapshot{ID: id, OrganizationID: job.OrganizationID, JobID: job.ID, Kind: kind, ResourceID: resourceID, Data: string(data), Size: len(data), CreatedAt: job.CreatedAt})
			return id, nil
		}
		brandSnapshotID := ""
		if brand != nil { id, snapshotErr := appendSnapshot("brand", brand.ID, *brand); if snapshotErr != nil { return snapshotErr }; brandSnapshotID = id }
		productSnapshotIDs := make(map[string]string, len(products))
		for _, product := range products { id, err := appendSnapshot("product", product.ID, product); if err != nil { return err }; productSnapshotIDs[product.ID] = id }
		skuSnapshotIDs := make(map[string]string, len(skuByID))
		for _, sku := range skuByID { id, err := appendSnapshot("sku", sku.ID, sku); if err != nil { return err }; skuSnapshotIDs[sku.ID] = id }
		for index, item := range items {
			product, ok := productByID[item.ProductID]
			if !ok || item.OrganizationID != job.OrganizationID || item.JobID != job.ID { return gorm.ErrRecordNotFound }
			items[index].BrandSnapshotID = brandSnapshotID
			items[index].ProductSnapshotID = productSnapshotIDs[product.ID]
			if item.SKUID != "" {
				sku, ok := skuByID[item.SKUID]
				if !ok || sku.ProductID != item.ProductID { return gorm.ErrRecordNotFound }
				items[index].SKUSnapshotID = skuSnapshotIDs[sku.ID]
			}
		}
		if err := tx.Create(&job).Error; err != nil { return err }
		if len(snapshots) > 0 { if err := tx.CreateInBatches(&snapshots, 200).Error; err != nil { return err } }
		if len(items) > 0 { if err := tx.CreateInBatches(&items, 200).Error; err != nil { return err } }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return job, err
}

func GetBatchProductionSnapshots(organizationID string, ids []string) (map[string]model.BatchProductionSnapshot, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.BatchProductionSnapshot
	if err := db.Where("organization_id = ? AND id IN ?", organizationID, ids).Find(&items).Error; err != nil { return nil, err }
	result := make(map[string]model.BatchProductionSnapshot, len(items))
	for _, item := range items { result[item.ID] = item }
	return result, nil
}

func CancelBatchProductionJob(organizationID string, id string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND id = ? AND status IN ?", organizationID, id, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": model.BatchProductionStatusCancelled, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionStateConflict }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status IN ?", organizationID, id, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": model.BatchProductionStatusCancelled, "lease_token": "", "lease_expires_at": "", "locked_at": "", "finished_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListBatchProductionItems(organizationID string, jobID string, q model.Query) ([]model.BatchProductionItem, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ?", organizationID, jobID)
	if q.Keyword != "" { tx = tx.Where("product_id LIKE ? OR sku_id LIKE ? OR error_message LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	if q.Type != "" && q.Type != "all" { tx = tx.Where("status = ?", q.Type) }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.BatchProductionItem
	err = tx.Order("created_at asc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error
	return items, total, err
}

func RetryBatchProductionJob(organizationID string, id string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND status = ?", organizationID, id, model.BatchProductionStatusFailed).First(&job).Error; err != nil { return err }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", organizationID, id, model.BatchProductionStatusFailed).Updates(map[string]any{"status": model.BatchProductionStatusQueued, "error_message": "", "run_number": gorm.Expr("run_number + 1"), "attempts": 0, "lease_token": "", "lease_expires_at": "", "locked_at": "", "started_at": "", "finished_at": "", "updated_at": timestamp}).Error; err != nil { return err }
		var queued int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", organizationID, id, model.BatchProductionStatusQueued).Count(&queued).Error; err != nil { return err }
		if queued == 0 { return ErrBatchProductionStateConflict }
		result := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND id = ? AND status = ?", organizationID, id, model.BatchProductionStatusFailed).Updates(map[string]any{"status": model.BatchProductionStatusQueued, "failed_items": 0, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionStateConflict }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ClaimNextBatchProductionItem(timestamp string, leaseExpiresAt string, leaseToken string) (model.BatchProductionItem, model.BatchProductionJob, bool, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, err }
	if err := failExhaustedBatchProductionItems(db, timestamp, 5); err != nil { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, err }
	for attempt := 0; attempt < 3; attempt++ {
		var item model.BatchProductionItem
		var job model.BatchProductionJob
		claimed := false
		err = db.Transaction(func(tx *gorm.DB) error {
			candidate := tx.Model(&model.BatchProductionItem{}).
				Select("batch_production_items.*").
				Joins("JOIN batch_production_jobs AS jobs ON jobs.id = batch_production_items.job_id AND jobs.organization_id = batch_production_items.organization_id").
				Where("jobs.status IN ?", []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).
				Where("batch_production_items.status = ? OR (batch_production_items.status = ? AND batch_production_items.attempts < ? AND batch_production_items.lease_expires_at <> '' AND batch_production_items.lease_expires_at <= ?)", model.BatchProductionStatusQueued, model.BatchProductionStatusRunning, 5, timestamp).
				Order("batch_production_items.created_at asc, batch_production_items.id asc")
			if err := candidate.First(&item).Error; err != nil { return err }
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ? AND organization_id = ? AND status IN ?", item.JobID, item.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Error; err != nil { return err }
			result := tx.Model(&model.BatchProductionItem{}).
				Where("id = ? AND organization_id = ? AND (status = ? OR (status = ? AND lease_expires_at <> '' AND lease_expires_at <= ?))", item.ID, item.OrganizationID, model.BatchProductionStatusQueued, model.BatchProductionStatusRunning, timestamp).
				Updates(map[string]any{"status": model.BatchProductionStatusRunning, "attempts": gorm.Expr("attempts + 1"), "lease_token": leaseToken, "lease_expires_at": leaseExpiresAt, "locked_at": timestamp, "started_at": timestamp, "finished_at": "", "result_url": "", "error_message": "", "updated_at": timestamp})
			if result.Error != nil { return result.Error }
			if result.RowsAffected == 0 { return nil }
			jobResult := tx.Model(&model.BatchProductionJob{}).Where("id = ? AND organization_id = ? AND status IN ?", job.ID, item.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": model.BatchProductionStatusRunning, "updated_at": timestamp})
			if jobResult.Error != nil { return jobResult.Error }
			claimed = true
			item.Status, item.LockedAt, item.StartedAt, item.UpdatedAt = model.BatchProductionStatusRunning, timestamp, timestamp, timestamp
			item.LeaseToken, item.LeaseExpiresAt = leaseToken, leaseExpiresAt
			item.Attempts++
			return nil
		})
		if errors.Is(err, gorm.ErrRecordNotFound) { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, nil }
		if err != nil { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, err }
		if claimed { return item, job, true, nil }
	}
	return model.BatchProductionItem{}, model.BatchProductionJob{}, false, nil
}

func failExhaustedBatchProductionItems(db *gorm.DB, timestamp string, maxAttempts int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var items []model.BatchProductionItem
		if err := tx.Where("status = ? AND attempts >= ? AND lease_expires_at <> '' AND lease_expires_at <= ?", model.BatchProductionStatusRunning, maxAttempts, timestamp).Find(&items).Error; err != nil { return err }
		byJob := map[string][]string{}
		jobs := map[string]model.BatchProductionItem{}
		for _, item := range items {
			key := item.OrganizationID + ":" + item.JobID
			byJob[key] = append(byJob[key], item.ID)
			jobs[key] = item
		}
		keys := make([]string, 0, len(byJob))
		for key := range byJob { keys = append(keys, key) }
		sort.Strings(keys)
		for _, key := range keys {
			ids := byJob[key]
			item := jobs[key]
			var job model.BatchProductionJob
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND status IN ?", item.JobID, item.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { continue } else if err != nil { return err }
			if err := tx.Model(&model.BatchProductionItem{}).Where("id IN ? AND organization_id = ? AND job_id = ? AND status = ? AND attempts >= ? AND lease_expires_at <= ?", ids, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, maxAttempts, timestamp).Updates(map[string]any{"status": model.BatchProductionStatusFailed, "error_message": "执行租约连续超时", "lease_token": "", "lease_expires_at": "", "locked_at": "", "finished_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
			var completed, failed, pending int64
			base := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ?", job.OrganizationID, job.ID)
			if err := base.Where("status = ?", model.BatchProductionStatusCompleted).Count(&completed).Error; err != nil { return err }
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusFailed).Count(&failed).Error; err != nil { return err }
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status IN ?", job.OrganizationID, job.ID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil { return err }
			status := model.BatchProductionStatusRunning
			if pending == 0 { status = model.BatchProductionStatusFailed }
			if err := tx.Model(&model.BatchProductionJob{}).Where("id = ? AND organization_id = ? AND status IN ?", job.ID, job.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": status, "completed_items": completed, "failed_items": failed, "updated_at": timestamp}).Error; err != nil { return err }
		}
		return nil
	})
}

func FinishBatchProductionItem(item model.BatchProductionItem, status model.BatchProductionStatus, resultURL string, errorMessage string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ? AND organization_id = ?", item.JobID, item.OrganizationID).Error; err != nil { return err }
		if job.Status == model.BatchProductionStatusCancelled || job.Status == model.BatchProductionStatusCompleted || job.Status == model.BatchProductionStatusFailed { return ErrBatchProductionLeaseLost }
		result := tx.Model(&model.BatchProductionItem{}).Where("id = ? AND organization_id = ? AND job_id = ? AND status = ? AND lease_token = ?", item.ID, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, item.LeaseToken).Updates(map[string]any{"status": status, "result_url": resultURL, "error_message": errorMessage, "finished_at": timestamp, "locked_at": "", "lease_token": "", "lease_expires_at": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionLeaseLost }
		var completed, failed, pending int64
		base := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ?", item.OrganizationID, item.JobID)
		if err := base.Where("status = ?", model.BatchProductionStatusCompleted).Count(&completed).Error; err != nil { return err }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", item.OrganizationID, item.JobID, model.BatchProductionStatusFailed).Count(&failed).Error; err != nil { return err }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status IN ?", item.OrganizationID, item.JobID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil { return err }
		jobStatus := model.BatchProductionStatusRunning
		if pending == 0 { if failed > 0 { jobStatus = model.BatchProductionStatusFailed } else { jobStatus = model.BatchProductionStatusCompleted } }
		result = tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND id = ? AND status IN ?", item.OrganizationID, item.JobID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": jobStatus, "completed_items": completed, "failed_items": failed, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionLeaseLost }
		return nil
	})
}

func RenewBatchProductionItemLease(item model.BatchProductionItem, leaseExpiresAt string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	result := db.Model(&model.BatchProductionItem{}).
		Where("id = ? AND organization_id = ? AND job_id = ? AND status = ? AND lease_token = ?", item.ID, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, item.LeaseToken).
		Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "locked_at": timestamp, "updated_at": timestamp})
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return ErrBatchProductionLeaseLost }
	return nil
}
