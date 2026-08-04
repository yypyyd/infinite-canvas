package repository

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrDefaultOrganizationAlreadyCreated = errors.New("default organization already created")
	ErrOrganizationInvitationUnavailable = errors.New("organization invitation unavailable")
	ErrOrganizationOwnershipChanged      = errors.New("organization ownership changed")
	ErrOrganizationVersionConflict       = errors.New("organization version conflict")
	ErrOrganizationMemberVersionConflict = errors.New("organization member version conflict")
	ErrBrandInUse                        = errors.New("brand is in use")
	ErrProductInUse                      = errors.New("product is in use")
	ErrProductSKUInUse                   = errors.New("product sku is in use")
	ErrBrandNameConflict                 = errors.New("brand name conflict")
	ErrProductCodeConflict               = errors.New("product code conflict")
	ErrProductSKUCodeConflict            = errors.New("product sku code conflict")
	ErrProductionTemplateNameConflict    = errors.New("production template name conflict")
	ErrCommerceVersionConflict           = errors.New("commerce version conflict")
	ErrBatchProductionLeaseLost          = errors.New("batch production lease lost")
	ErrBatchProductionStateConflict      = errors.New("batch production state conflict")
	ErrBatchProductionRequestConflict    = errors.New("batch production request conflict")
	ErrBatchProductionItemsTooLarge      = errors.New("batch production items too large")
	ErrBatchProductionSnapshotTooLarge   = errors.New("batch production snapshot too large")
	ErrBatchProductionOrganizationQueueFull = errors.New("batch production organization queue full")
)

const (
	maxBatchProductionItems = 5000
	maxBatchProductionPendingItemsPerOrganization = 10000
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
		organization = model.Organization{ID: organizationID, Name: name + "的企业", Slug: organizationID, Status: "active", Version: 1, CreditMode: model.OrganizationCreditModePersonal, CreditAlertThreshold: 80, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
		membership = model.OrganizationMember{ID: memberID, OrganizationID: organization.ID, UserID: user.ID, Role: model.OrganizationRoleOwner, Version: 1, CreatedAt: timestamp, UpdatedAt: timestamp}
		if err := tx.Create(&organization).Error; err != nil {
			return err
		}
		if err := tx.Create(&membership).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ? AND organization_id = ''", user.ID).Update("organization_id", organization.ID)
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrDefaultOrganizationAlreadyCreated }
		if err := assignLegacyOrganizationData(tx, user.ID, organization.ID); err != nil { return err }
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
	found := false
	err = db.Transaction(func(tx *gorm.DB) error {
		unavailableInvitations := tx.Model(&model.OrganizationInvitation{}).Select("id").Where("status <> ? OR expires_at <= ?", model.OrganizationInvitationPending, timestamp)
		if err := tx.Model(&model.OrganizationEmailOutbox{}).Where("status IN ? AND invitation_id IN (?)", []string{"pending", "failed"}, unavailableInvitations).Updates(map[string]any{"status": "cancelled", "updated_at": timestamp}).Error; err != nil { return err }
		query := tx.Table("organization_email_outboxes AS e").Select("e.*").Joins("JOIN organization_invitations AS i ON i.id = e.invitation_id AND i.status = ? AND i.expires_at > ?", model.OrganizationInvitationPending, timestamp).Clauses(clause.Locking{Strength: "UPDATE"}).Where("(e.status IN ? AND e.attempts < ? AND e.next_attempt_at <= ?) OR (e.status = ? AND e.lease_expires_at <= ?)", []string{"pending", "failed"}, 10, timestamp, "processing", timestamp).Order("e.created_at asc")
		if err := query.First(&item).Error; errors.Is(err, gorm.ErrRecordNotFound) { return nil } else if err != nil { return err }
		found = true
		return tx.Model(&model.OrganizationEmailOutbox{}).Where("id = ?", item.ID).Updates(map[string]any{"status": "processing", "attempts": gorm.Expr("attempts + 1"), "lease_token": leaseToken, "lease_expires_at": leaseExpiresAt, "updated_at": timestamp}).Error
	})
	if err != nil { return model.OrganizationEmailOutbox{}, false, err }
	if !found { return model.OrganizationEmailOutbox{}, false, nil }
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
		if saved.Version != organization.Version-1 { return ErrOrganizationVersionConflict }
		result := tx.Model(&model.Organization{}).Where("id = ? AND version = ?", organization.ID, saved.Version).Updates(map[string]any{"name": organization.Name, "version": organization.Version, "updated_at": organization.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationVersionConflict }
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
		Select("m.id, m.user_id, u.username, u.display_name, u.email, u.avatar_url, m.role, m.version, m.created_at").
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
		var saved model.OrganizationMember
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", member.OrganizationID, member.ID).First(&saved).Error; err != nil { return err }
		if saved.Role == model.OrganizationRoleOwner { return ErrOrganizationOwnershipChanged }
		if saved.Version != member.Version-1 { return ErrOrganizationMemberVersionConflict }
		result := tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role <> ? AND version = ?", member.OrganizationID, member.ID, model.OrganizationRoleOwner, saved.Version).Updates(map[string]any{"role": member.Role, "version": member.Version, "updated_at": member.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationMemberVersionConflict }
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

func DeleteOrganizationMember(organizationID string, memberID string, expectedVersion int64, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var member model.OrganizationMember
		if err := tx.Where("organization_id = ? AND id = ?", organizationID, memberID).First(&member).Error; err != nil { return err }
		if member.Role == model.OrganizationRoleOwner { return ErrOrganizationOwnershipChanged }
		if member.Version != expectedVersion { return ErrOrganizationMemberVersionConflict }
		result := tx.Where("organization_id = ? AND id = ? AND role <> ? AND version = ?", organizationID, memberID, model.OrganizationRoleOwner, expectedVersion).Delete(&model.OrganizationMember{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationMemberVersionConflict }
		if err := tx.Model(&model.User{}).Where("id = ? AND organization_id = ?", member.UserID, organizationID).Update("organization_id", "").Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func TransferOrganizationOwnership(organizationID string, ownerMemberID string, targetMemberID string, targetExpectedVersion int64, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
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
		if target.Version != targetExpectedVersion { return ErrOrganizationMemberVersionConflict }
		result := tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role = ? AND version = ?", organizationID, owner.ID, model.OrganizationRoleOwner, owner.Version).Updates(map[string]any{"role": model.OrganizationRoleAdmin, "version": gorm.Expr("version + 1"), "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationOwnershipChanged }
		result = tx.Model(&model.OrganizationMember{}).Where("organization_id = ? AND id = ? AND role <> ? AND version = ?", organizationID, target.ID, model.OrganizationRoleOwner, targetExpectedVersion).Updates(map[string]any{"role": model.OrganizationRoleOwner, "version": gorm.Expr("version + 1"), "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationMemberVersionConflict }
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
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		if create {
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrBrandNameConflict }; return err }
		} else {
			var saved model.Brand
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).First(&saved).Error; err != nil { return err }
			if saved.Version != item.Version-1 { return ErrCommerceVersionConflict }
			result := tx.Model(&model.Brand{}).Where("organization_id = ? AND id = ? AND version = ?", item.OrganizationID, item.ID, saved.Version).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrBrandNameConflict }; return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		}
		storageKeys := []string{}
		if item.LogoStorageKey != "" { storageKeys = append(storageKeys, item.LogoStorageKey) }
		if err := replaceUserFileReferences(tx, item.OrganizationID, "brand", item.ID, "brand-"+item.ID, storageKeys, false, item.UpdatedAt); err != nil { return err }
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

func DeleteBrand(organizationID string, id string, expectedVersion int64, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var brand model.Brand
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&brand).Error; err != nil { return err }
		if brand.Version != expectedVersion { return ErrCommerceVersionConflict }
		var references int64
		if err := tx.Model(&model.Product{}).Where("organization_id = ? AND brand_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references == 0 {
			if err := tx.Model(&model.BatchProductionJob{}).Where("organization_id = ? AND brand_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		}
		if references > 0 { return ErrBrandInUse }
		if err := replaceUserFileReferences(tx, organizationID, "brand", id, "brand-"+id, nil, true, timestamp); err != nil { return err }
		result := tx.Where("organization_id = ? AND id = ? AND version = ?", organizationID, id, expectedVersion).Delete(&model.Brand{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
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
	if q.Brand != "" && q.Brand != "all" {
		tx = tx.Where("products.brand_id = ?", q.Brand)
	}
	if q.Category != "" && q.Category != "all" {
		tx = tx.Where("products.category = ?", q.Category)
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
			if saved.Version != item.Version-1 { return ErrCommerceVersionConflict }
			result := tx.Model(&model.Product{}).Where("organization_id = ? AND id = ? AND version = ?", item.OrganizationID, item.ID, saved.Version).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrProductCodeConflict }; return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
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

func UpdateProductStatuses(organizationID string, items []model.ProductStatusItemInput, status model.ProductStatus, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		for _, item := range items {
			result := tx.Model(&model.Product{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, item.ID, item.Version).Updates(map[string]any{"status": status, "version": gorm.Expr("version + 1"), "updated_at": timestamp})
			if result.Error != nil { return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func DeleteProduct(organizationID string, id string, expectedVersion int64, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&product).Error; err != nil { return err }
		if product.Version != expectedVersion { return ErrCommerceVersionConflict }
		var references int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND product_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references > 0 { return ErrProductInUse }
		var skuIDs []string
		if err := tx.Model(&model.ProductSKU{}).Where("organization_id = ? AND product_id = ?", organizationID, id).Pluck("id", &skuIDs).Error; err != nil { return err }
		storageKeys := []string{}
		for start := 0; start < len(skuIDs); start += 200 {
			end := min(start+200, len(skuIDs))
			var batchKeys []string
			if err := tx.Model(&model.UserFileReference{}).Where("organization_id = ? AND domain = ? AND object_id IN ?", organizationID, "sku", skuIDs[start:end]).Pluck("storage_key", &batchKeys).Error; err != nil { return err }
			storageKeys = append(storageKeys, batchKeys...)
			if err := tx.Where("organization_id = ? AND domain = ? AND object_id IN ?", organizationID, "sku", skuIDs[start:end]).Delete(&model.UserFileReference{}).Error; err != nil { return err }
		}
		if err := refreshUserFileReferenceState(tx, organizationID, storageKeys, timestamp); err != nil { return err }
		if err := tx.Where("organization_id = ? AND product_id = ?", organizationID, id).Delete(&model.ProductSKU{}).Error; err != nil { return err }
		result := tx.Where("organization_id = ? AND id = ? AND version = ?", organizationID, id, expectedVersion).Delete(&model.Product{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
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

func SaveProductSKU(item model.ProductSKU, create bool, auditLogs ...model.OrganizationAuditLog) (model.ProductSKU, error) {
	db, err := DB()
	if err != nil { return item, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ProductID).First(&product).Error; err != nil { return err }
		if create {
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrProductSKUCodeConflict }; return err }
		} else {
			var saved model.ProductSKU
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND product_id = ?", item.OrganizationID, item.ID, item.ProductID).First(&saved).Error; err != nil { return err }
			if saved.Version != item.Version-1 { return ErrCommerceVersionConflict }
			result := tx.Model(&model.ProductSKU{}).Where("organization_id = ? AND id = ? AND product_id = ? AND version = ?", item.OrganizationID, item.ID, item.ProductID, saved.Version).Select("*").Updates(&item)
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrProductSKUCodeConflict }; return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		}
		if err := replaceUserFileReferences(tx, item.OrganizationID, "sku", item.ID, "sku-"+item.ID, item.ImageStorageKeys, false, item.UpdatedAt); err != nil { return err }
		result := tx.Model(&model.Product{}).Where("organization_id = ? AND id = ? AND version = ?", item.OrganizationID, item.ProductID, product.Version).Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": item.UpdatedAt})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
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

func DeleteProductSKU(organizationID string, id string, expectedVersion int64, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		var sku model.ProductSKU
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, id).First(&sku).Error; err != nil { return err }
		if sku.Version != expectedVersion { return ErrCommerceVersionConflict }
		var product model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, sku.ProductID).First(&product).Error; err != nil { return err }
		var references int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND sku_id = ?", organizationID, id).Count(&references).Error; err != nil { return err }
		if references > 0 { return ErrProductSKUInUse }
		if err := replaceUserFileReferences(tx, organizationID, "sku", id, "sku-"+id, nil, true, timestamp); err != nil { return err }
		result := tx.Where("organization_id = ? AND id = ? AND version = ?", organizationID, id, expectedVersion).Delete(&model.ProductSKU{})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		result = tx.Model(&model.Product{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, sku.ProductID, product.Version).Updates(map[string]any{"version": gorm.Expr("version + 1"), "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func SaveOrganizationCreditSettings(organizationID string, mode model.OrganizationCreditMode, monthlyBudget int, alertThreshold int, expectedVersion int64, budgetMonth string, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.Organization, error) {
	db, err := DB()
	if err != nil { return model.Organization{}, err }
	var organization model.Organization
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		if organization.Version != expectedVersion { return ErrOrganizationVersionConflict }
		monthlyUsed := organization.MonthlyCreditsUsed
		if organization.CreditBudgetMonth != budgetMonth { monthlyUsed = 0 }
		result := tx.Model(&model.Organization{}).Where("id = ? AND version = ?", organizationID, expectedVersion).Updates(map[string]any{
			"credit_mode": mode, "monthly_credit_budget": monthlyBudget, "monthly_credits_used": monthlyUsed,
			"credit_budget_month": budgetMonth, "credit_alert_threshold": alertThreshold,
			"version": expectedVersion + 1, "updated_at": timestamp,
		})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrOrganizationVersionConflict }
		organization.CreditMode, organization.MonthlyCreditBudget, organization.MonthlyCreditsUsed = mode, monthlyBudget, monthlyUsed
		organization.CreditBudgetMonth, organization.CreditAlertThreshold = budgetMonth, alertThreshold
		organization.Version, organization.UpdatedAt = expectedVersion+1, timestamp
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return organization, err
}

func TransferCreditsToOrganization(organizationID string, userID string, amount int, timestamp string, personalLog model.CreditLog, organizationLog model.CreditLog, auditLogs ...model.OrganizationAuditLog) (model.Organization, model.User, error) {
	db, err := DB()
	if err != nil { return model.Organization{}, model.User{}, err }
	var organization model.Organization
	var user model.User
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", organizationID, "active").First(&organization).Error; err != nil { return err }
		var membership model.OrganizationMember
		if err := tx.Where("organization_id = ? AND user_id = ?", organizationID, userID).First(&membership).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", userID).Error; err != nil { return err }
		result := tx.Model(&model.User{}).Where("id = ? AND credits >= ?", userID, amount).Updates(map[string]any{"credits": gorm.Expr("credits - ?", amount), "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrInsufficientUserCredits }
		result = tx.Model(&model.Organization{}).Where("id = ?", organizationID).Updates(map[string]any{"credits": gorm.Expr("credits + ?", amount), "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return gorm.ErrRecordNotFound }
		user.Credits -= amount
		organization.Credits += amount
		personalLog.Balance, organizationLog.Balance = user.Credits, organization.Credits
		if err := tx.Create(&personalLog).Error; err != nil { return err }
		if err := tx.Create(&organizationLog).Error; err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return organization, user, err
}

func ListProductionTemplates(organizationID string, q model.Query) ([]model.ProductionTemplate, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.ProductionTemplate{}).Where("organization_id = ?", organizationID)
	if q.Keyword != "" { tx = tx.Where("name LIKE ? OR description LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	if q.Type != "" && q.Type != "all" { tx = tx.Where("status = ?", q.Type) }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.ProductionTemplate
	if err := tx.Order("updated_at desc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error; err != nil { return nil, 0, err }
	for index := range items {
		items[index].CurrentPrompt, items[index].CurrentSpec = items[index].DraftPrompt, items[index].DraftSpecJSON
	}
	return items, total, nil
}

func GetProductionTemplate(organizationID string, templateID string) (model.ProductionTemplate, bool, error) {
	db, err := DB()
	if err != nil { return model.ProductionTemplate{}, false, err }
	var item model.ProductionTemplate
	err = db.Where("organization_id = ? AND id = ?", organizationID, templateID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return item, false, nil }
	if err != nil { return item, false, err }
	item.CurrentPrompt, item.CurrentSpec = item.DraftPrompt, item.DraftSpecJSON
	return item, true, nil
}

func GetProductionTemplateVersion(organizationID string, templateID string, versionNumber int) (model.ProductionTemplate, model.ProductionTemplateVersion, bool, error) {
	db, err := DB()
	if err != nil { return model.ProductionTemplate{}, model.ProductionTemplateVersion{}, false, err }
	var item model.ProductionTemplate
	if err := db.Where("organization_id = ? AND id = ?", organizationID, templateID).First(&item).Error; errors.Is(err, gorm.ErrRecordNotFound) { return model.ProductionTemplate{}, model.ProductionTemplateVersion{}, false, nil } else if err != nil { return model.ProductionTemplate{}, model.ProductionTemplateVersion{}, false, err }
	if versionNumber <= 0 { versionNumber = item.CurrentVersion }
	var version model.ProductionTemplateVersion
	if err := db.Where("organization_id = ? AND template_id = ? AND version = ?", organizationID, templateID, versionNumber).First(&version).Error; errors.Is(err, gorm.ErrRecordNotFound) { return item, model.ProductionTemplateVersion{}, false, nil } else if err != nil { return item, model.ProductionTemplateVersion{}, false, err }
	item.CurrentPrompt, item.CurrentSpec = version.Prompt, version.SpecJSON
	return item, version, true, nil
}

func ListProductionTemplateVersions(organizationID string, templateID string) ([]model.ProductionTemplateVersion, error) {
	db, err := DB()
	if err != nil { return nil, err }
	var items []model.ProductionTemplateVersion
	err = db.Where("organization_id = ? AND template_id = ?", organizationID, templateID).Order("version desc").Find(&items).Error
	return items, err
}

func SaveProductionTemplate(item model.ProductionTemplate, expectedVersion int64, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.ProductionTemplate, error) {
	db, err := DB()
	if err != nil { return item, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		if item.ID == "" { return gorm.ErrRecordNotFound }
		var saved model.ProductionTemplate
		find := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", item.OrganizationID, item.ID).First(&saved)
		if errors.Is(find.Error, gorm.ErrRecordNotFound) {
			if expectedVersion != 0 { return ErrCommerceVersionConflict }
			item.Status, item.CurrentVersion, item.Version = model.ProductionTemplateStatusDraft, 0, 1
			item.CreatedAt, item.UpdatedAt = timestamp, timestamp
			if err := tx.Create(&item).Error; err != nil { if isDuplicateKeyError(err) { return ErrProductionTemplateNameConflict }; return err }
		} else {
			if find.Error != nil { return find.Error }
			if saved.Version != expectedVersion { return ErrCommerceVersionConflict }
			item.CreatedBy, item.CreatedAt = saved.CreatedBy, saved.CreatedAt
			item.CurrentVersion, item.Version, item.UpdatedAt = saved.CurrentVersion, saved.Version+1, timestamp
			result := tx.Model(&model.ProductionTemplate{}).Where("organization_id = ? AND id = ? AND version = ?", item.OrganizationID, item.ID, expectedVersion).Updates(map[string]any{"name": item.Name, "description": item.Description, "source": item.Source, "media_type": item.MediaType, "template_type": item.TemplateType, "platform": item.Platform, "status": item.Status, "draft_prompt": item.DraftPrompt, "draft_spec_json": item.DraftSpecJSON, "version": item.Version, "updated_at": timestamp})
			if result.Error != nil { if isDuplicateKeyError(result.Error) { return ErrProductionTemplateNameConflict }; return result.Error }
			if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		}
		item.CurrentPrompt, item.CurrentSpec = item.DraftPrompt, item.DraftSpecJSON
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func PublishProductionTemplate(organizationID string, templateID string, expectedVersion int64, versionID string, createdBy string, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.ProductionTemplate, model.ProductionTemplateVersion, error) {
	db, err := DB()
	if err != nil { return model.ProductionTemplate{}, model.ProductionTemplateVersion{}, err }
	var item model.ProductionTemplate
	var version model.ProductionTemplateVersion
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", organizationID).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, templateID).First(&item).Error; err != nil { return err }
		if item.Version != expectedVersion { return ErrCommerceVersionConflict }
		version = model.ProductionTemplateVersion{ID: versionID, OrganizationID: organizationID, TemplateID: templateID, Version: item.CurrentVersion+1, Prompt: item.DraftPrompt, SpecJSON: item.DraftSpecJSON, CreatedBy: createdBy, CreatedAt: timestamp}
		if err := tx.Create(&version).Error; err != nil { return err }
		result := tx.Model(&model.ProductionTemplate{}).Where("organization_id = ? AND id = ? AND version = ?", organizationID, templateID, expectedVersion).Updates(map[string]any{"status": model.ProductionTemplateStatusActive, "current_version": version.Version, "version": expectedVersion+1, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrCommerceVersionConflict }
		item.Status, item.CurrentVersion, item.Version, item.UpdatedAt = model.ProductionTemplateStatusActive, version.Version, expectedVersion+1, timestamp
		item.CurrentPrompt, item.CurrentSpec = item.DraftPrompt, item.DraftSpecJSON
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, version, err
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

func GetBatchProductionJobByRequest(organizationID string, requestID string) (model.BatchProductionJob, bool, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionJob{}, false, err }
	var item model.BatchProductionJob
	err = db.Where("organization_id = ? AND request_id = ?", organizationID, requestID).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) { return model.BatchProductionJob{}, false, nil }
	return item, err == nil, err
}

func CreateBatchProductionJob(job model.BatchProductionJob, itemEstimatedCredits map[string]int, auditLogs ...model.OrganizationAuditLog) (model.BatchProductionJob, error) {
	db, err := DB()
	if err != nil { return job, err }
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", job.OrganizationID).Error; err != nil { return err }
		var existing model.BatchProductionJob
		if err := tx.Where("organization_id = ? AND request_id = ?", job.OrganizationID, job.RequestID).First(&existing).Error; err == nil { if existing.RequestHash != job.RequestHash { return ErrBatchProductionRequestConflict }; job = existing; return nil } else if !errors.Is(err, gorm.ErrRecordNotFound) { return err }
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
		var skus []model.ProductSKU
		if err := tx.Where("organization_id = ? AND product_id IN ?", job.OrganizationID, job.ProductIDs).Order("created_at asc, id asc").Limit(maxBatchProductionItems + 1).Find(&skus).Error; err != nil { return err }
		if len(skus) > maxBatchProductionItems { return ErrBatchProductionItemsTooLarge }
		items := make([]model.BatchProductionItem, 0, len(skus)+len(products))
		covered := make(map[string]bool, len(products))
		appendItem := func(productID string, skuID string) error {
			if len(items) >= maxBatchProductionItems { return ErrBatchProductionItemsTooLarge }
			estimatedCredits, ok := itemEstimatedCredits[productID+"\x00"+skuID]
			if !ok || estimatedCredits <= 0 { return errors.New("batch production item estimate is missing") }
			items = append(items, model.BatchProductionItem{ID: job.ID + "-item-" + strconv.Itoa(len(items)), OrganizationID: job.OrganizationID, JobID: job.ID, ProductID: productID, SKUID: skuID, EstimatedCredits: estimatedCredits, Status: model.BatchProductionStatusQueued, RunNumber: 1, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt})
			return nil
		}
		for _, sku := range skus { covered[sku.ProductID] = true; if err := appendItem(sku.ProductID, sku.ID); err != nil { return err } }
		for _, product := range products { if !covered[product.ID] { if err := appendItem(product.ID, ""); err != nil { return err } } }
		job.TotalItems, job.QueuedItems, job.EstimatedCredits = len(items), len(items), 0
		for _, item := range items { job.EstimatedCredits += item.EstimatedCredits }
		var pending int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status IN ?", job.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil { return err }
		if pending+int64(len(items)) > maxBatchProductionPendingItemsPerOrganization { return ErrBatchProductionOrganizationQueueFull }
		skuByID := make(map[string]model.ProductSKU, len(skus))
		for _, sku := range skus { skuByID[sku.ID] = sku }
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
		for _, sku := range skus { id, err := appendSnapshot("sku", sku.ID, sku); if err != nil { return err }; skuSnapshotIDs[sku.ID] = id }
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
		if err := reserveBatchProductionCredits(tx, &organization, &job); err != nil { return err }
		if err := tx.Create(&job).Error; err != nil { return err }
		if len(snapshots) > 0 { if err := tx.CreateInBatches(&snapshots, 200).Error; err != nil { return err } }
		if len(items) > 0 { if err := tx.CreateInBatches(&items, 200).Error; err != nil { return err } }
		inputStorageKeys := []string{}
		if brand != nil && brand.LogoStorageKey != "" { inputStorageKeys = append(inputStorageKeys, brand.LogoStorageKey) }
		for _, sku := range skus { inputStorageKeys = append(inputStorageKeys, sku.ImageStorageKeys...) }
		if err := replaceUserFileReferences(tx, job.OrganizationID, "batch_input", job.ID, "batch-input-"+job.ID, inputStorageKeys, false, job.CreatedAt); err != nil { return err }
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

func releaseBatchProductionCredits(tx *gorm.DB, job *model.BatchProductionJob, timestamp string) error {
	outstanding := job.ReservedCredits - job.SettledCredits - job.ReleasedCredits
	if outstanding < 0 {
		return ErrBatchProductionStateConflict
	}
	if outstanding == 0 {
		return nil
	}
	if job.CreditSource == model.CreditSourceOrganization {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", job.OrganizationID).Error; err != nil {
			return err
		}
		if organization.ReservedCredits < outstanding {
			return ErrBatchProductionStateConflict
		}
		result := tx.Model(&model.Organization{}).
			Where("id = ? AND reserved_credits >= ?", organization.ID, outstanding).
			Updates(map[string]any{
				"reserved_credits": gorm.Expr("reserved_credits - ?", outstanding),
				"updated_at":       timestamp,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBatchProductionStateConflict
		}
	} else if job.CreditSource == model.CreditSourcePersonal {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", job.CreatedBy).Error; err != nil {
			return err
		}
		if user.ReservedCredits < outstanding {
			return ErrBatchProductionStateConflict
		}
		result := tx.Model(&model.User{}).
			Where("id = ? AND reserved_credits >= ?", user.ID, outstanding).
			Updates(map[string]any{
				"reserved_credits": gorm.Expr("reserved_credits - ?", outstanding),
				"updated_at":       timestamp,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrBatchProductionStateConflict
		}
	} else {
		return ErrBatchProductionStateConflict
	}
	result := tx.Model(&model.BatchProductionJob{}).
		Where("organization_id = ? AND id = ?", job.OrganizationID, job.ID).
		Update("released_credits", gorm.Expr("released_credits + ?", outstanding))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBatchProductionStateConflict
	}
	job.ReleasedCredits += outstanding
	return nil
}

func aggregateBatchProductionJob(tx *gorm.DB, job *model.BatchProductionJob, timestamp string) error {
	base := tx.Model(&model.BatchProductionItem{}).
		Where("organization_id = ? AND job_id = ?", job.OrganizationID, job.ID)
	var total, queued, running, completed, failed, cancelled int64
	if err := base.Count(&total).Error; err != nil {
		return err
	}
	if total == 0 {
		return ErrBatchProductionStateConflict
	}
	if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusQueued).Count(&queued).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusRunning).Count(&running).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusCompleted).Count(&completed).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusFailed).Count(&failed).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusCancelled).Count(&cancelled).Error; err != nil {
		return err
	}

	status := job.Status
	if status != model.BatchProductionStatusCancelled {
		if queued > 0 || running > 0 {
			var started int64
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND started_at <> ''", job.OrganizationID, job.ID).Count(&started).Error; err != nil {
				return err
			}
			status = model.BatchProductionStatusQueued
			if running > 0 || completed > 0 || failed > 0 || started > 0 {
				status = model.BatchProductionStatusRunning
			}
		} else {
			var pendingReviews, rejected int64
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ? AND review_status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusCompleted, model.BatchProductionReviewPending).Count(&pendingReviews).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status = ? AND review_status = ?", job.OrganizationID, job.ID, model.BatchProductionStatusCompleted, model.BatchProductionReviewRejected).Count(&rejected).Error; err != nil {
				return err
			}
			if failed > 0 {
				if completed > 0 {
					status = model.BatchProductionStatusPartialSuccess
				} else {
					status = model.BatchProductionStatusFailed
				}
			} else if cancelled > 0 {
				if completed > 0 {
					status = model.BatchProductionStatusPartialSuccess
				} else {
					status = model.BatchProductionStatusFailed
				}
			} else if pendingReviews > 0 {
				status = model.BatchProductionStatusPendingReview
			} else if rejected > 0 {
				status = model.BatchProductionStatusPartialSuccess
			} else {
				status = model.BatchProductionStatusCompleted
			}
		}
	}

	result := tx.Model(&model.BatchProductionJob{}).
		Where("organization_id = ? AND id = ?", job.OrganizationID, job.ID).
		Updates(map[string]any{
			"status":          status,
			"total_items":     total,
			"queued_items":    queued,
			"running_items":   running,
			"completed_items": completed,
			"failed_items":    failed,
			"updated_at":      timestamp,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBatchProductionStateConflict
	}
	job.Status = status
	job.TotalItems = int(total)
	job.QueuedItems = int(queued)
	job.RunningItems = int(running)
	job.CompletedItems = int(completed)
	job.FailedItems = int(failed)
	job.UpdatedAt = timestamp
	if status != model.BatchProductionStatusQueued && status != model.BatchProductionStatusRunning {
		return releaseBatchProductionCredits(tx, job, timestamp)
	}
	return nil
}

func reserveBatchProductionRetryCredits(tx *gorm.DB, job *model.BatchProductionJob, amount int, timestamp string) error {
	if amount <= 0 {
		return ErrBatchProductionStateConflict
	}
	if job.CreditSource == model.CreditSourceOrganization {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", job.OrganizationID).Error; err != nil {
			return err
		}
		budgetMonth := batchCreditBudgetMonth(timestamp)
		if organization.CreditBudgetMonth != budgetMonth {
			if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Updates(map[string]any{
				"monthly_credits_used": 0,
				"credit_budget_month":  budgetMonth,
				"updated_at":            timestamp,
			}).Error; err != nil {
				return err
			}
			organization.MonthlyCreditsUsed = 0
			organization.CreditBudgetMonth = budgetMonth
		}
		if organization.Credits-organization.ReservedCredits < amount {
			return ErrInsufficientOrganizationCredits
		}
		if organization.MonthlyCreditBudget > 0 && organization.MonthlyCreditsUsed+organization.ReservedCredits+amount > organization.MonthlyCreditBudget {
			return ErrOrganizationCreditBudgetExceeded
		}
		result := tx.Model(&model.Organization{}).
			Where("id = ? AND credits - reserved_credits >= ? AND (monthly_credit_budget = 0 OR monthly_credits_used + reserved_credits + ? <= monthly_credit_budget)", organization.ID, amount, amount).
			Updates(map[string]any{
				"reserved_credits": gorm.Expr("reserved_credits + ?", amount),
				"updated_at":       timestamp,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrOrganizationCreditBudgetExceeded
		}
	} else if job.CreditSource == model.CreditSourcePersonal {
		var user model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, "id = ?", job.CreatedBy).Error; err != nil {
			return err
		}
		if user.Credits-user.ReservedCredits < amount {
			return ErrInsufficientUserCredits
		}
		result := tx.Model(&model.User{}).
			Where("id = ? AND credits - reserved_credits >= ?", user.ID, amount).
			Updates(map[string]any{
				"reserved_credits": gorm.Expr("reserved_credits + ?", amount),
				"updated_at":       timestamp,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrInsufficientUserCredits
		}
	} else {
		return ErrBatchProductionStateConflict
	}
	result := tx.Model(&model.BatchProductionJob{}).
		Where("organization_id = ? AND id = ?", job.OrganizationID, job.ID).
		Update("reserved_credits", gorm.Expr("reserved_credits + ?", amount))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBatchProductionStateConflict
	}
	job.ReservedCredits += amount
	return nil
}

func CancelBatchProductionJob(organizationID string, id string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND status IN ?", organizationID, id, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND status IN ?", organizationID, id, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Updates(map[string]any{"status": model.BatchProductionStatusCancelled, "error_code": model.BatchProductionErrorCancelledLeaseLost, "retryable": false, "next_attempt_at": "", "lease_token": "", "lease_expires_at": "", "locked_at": "", "finished_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
		job.Status = model.BatchProductionStatusCancelled
		if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ListBatchProductionItems(organizationID string, jobID string, q model.Query) ([]model.BatchProductionItem, int64, error) {
	db, err := DB()
	if err != nil { return nil, 0, err }
	q.Normalize()
	tx := db.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ?", organizationID, jobID)
	if q.Keyword != "" { tx = tx.Where("product_id LIKE ? OR sku_id LIKE ? OR error_message LIKE ? OR review_comment LIKE ?", "%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%", "%"+q.Keyword+"%") }
	if q.Type != "" && q.Type != "all" { tx = tx.Where("status = ?", q.Type) }
	var total int64
	if err := tx.Count(&total).Error; err != nil { return nil, 0, err }
	var items []model.BatchProductionItem
	if err := tx.Order("created_at asc").Offset(q.Offset()).Limit(q.PageSize).Find(&items).Error; err != nil { return nil, 0, err }
	storageKeys := make([]string, 0, len(items))
	for _, item := range items { if item.ResultStorageKey != "" { storageKeys = append(storageKeys, item.ResultStorageKey) } }
	if len(storageKeys) == 0 { return items, total, nil }
	var files []model.UserFile
	if err := db.Select("storage_key", "mime_type", "size").Where("organization_id = ? AND storage_key IN ?", organizationID, storageKeys).Find(&files).Error; err != nil { return nil, 0, err }
	fileByKey := make(map[string]model.UserFile, len(files))
	for _, file := range files { fileByKey[file.StorageKey] = file }
	for index := range items { if file, ok := fileByKey[items[index].ResultStorageKey]; ok { items[index].ResultMimeType, items[index].ResultSize = file.MimeType, file.Size } }
	return items, total, nil
}

func ListBatchProductionArchiveItems(organizationID string, jobID string) (model.BatchProductionJob, []model.BatchProductionArchiveItem, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionJob{}, nil, err }
	var job model.BatchProductionJob
	if err := db.Where("organization_id = ? AND id = ?", organizationID, jobID).First(&job).Error; err != nil { return model.BatchProductionJob{}, nil, err }
	var items []model.BatchProductionArchiveItem
	err = db.Table("batch_production_items AS items").
		Select("items.id, items.product_id, products.code AS product_code, items.sku_id, product_skus.code AS sku_code, items.template_selection_id, selections.id AS resolved_template_selection_id, items.result_storage_key, user_files.mime_type, user_files.size, items.is_primary, items.template_type, items.variant_index, selections.delivery_id, selections.delivery_platform, selections.delivery_name, selections.delivery_width, selections.delivery_height, selections.delivery_format, selections.delivery_quality, selections.delivery_filename_pattern").
		Joins("JOIN products ON products.organization_id = items.organization_id AND products.id = items.product_id").
		Joins("LEFT JOIN product_skus ON product_skus.organization_id = items.organization_id AND product_skus.id = items.sku_id").
		Joins("LEFT JOIN batch_production_template_selections AS selections ON selections.organization_id = items.organization_id AND selections.job_id = items.job_id AND selections.id = items.template_selection_id").
		Joins("JOIN user_files ON user_files.organization_id = items.organization_id AND user_files.storage_key = items.result_storage_key").
		Where("items.organization_id = ? AND items.job_id = ? AND items.status = ? AND items.review_status = ? AND items.result_storage_key <> ''", organizationID, jobID, model.BatchProductionStatusCompleted, model.BatchProductionReviewApproved).
		Order("items.product_id asc, items.is_primary desc, items.sku_id asc, items.id asc").
		Find(&items).Error
	return job, items, err
}

func ReviewBatchProductionItem(organizationID string, jobID string, itemID string, runNumber int, status model.BatchProductionReviewStatus, comment string, reviewerID string, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.BatchProductionItem, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionItem{}, err }
	var item model.BatchProductionItem
	err = db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, jobID).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND job_id = ? AND id = ? AND status = ? AND run_number = ?", organizationID, jobID, itemID, model.BatchProductionStatusCompleted, runNumber).First(&item).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		updates := map[string]any{"review_status": status, "review_comment": comment, "reviewed_by": reviewerID, "reviewed_at": timestamp, "updated_at": timestamp}
		if status == model.BatchProductionReviewRejected { updates["is_primary"] = false }
		result := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND id = ? AND status = ? AND run_number = ?", organizationID, jobID, itemID, model.BatchProductionStatusCompleted, runNumber).Updates(updates)
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionStateConflict }
		item.ReviewStatus, item.ReviewComment, item.ReviewedBy, item.ReviewedAt, item.UpdatedAt = status, comment, reviewerID, timestamp, timestamp
		if status == model.BatchProductionReviewRejected { item.IsPrimary = false }
		if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func SetBatchProductionPrimary(organizationID string, jobID string, itemID string, runNumber int, timestamp string, auditLogs ...model.OrganizationAuditLog) (model.BatchProductionItem, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionItem{}, err }
	var item model.BatchProductionItem
	err = db.Transaction(func(tx *gorm.DB) error {
		var candidate model.BatchProductionItem
		if err := tx.Where("organization_id = ? AND job_id = ? AND id = ? AND status = ? AND run_number = ? AND review_status = ? AND result_storage_key <> ''", organizationID, jobID, itemID, model.BatchProductionStatusCompleted, runNumber, model.BatchProductionReviewApproved).First(&candidate).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ?", organizationID, jobID).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		var scopeItems []model.BatchProductionItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND product_id = ? AND sku_id = ?", organizationID, candidate.ProductID, candidate.SKUID).Order("id asc").Find(&scopeItems).Error; err != nil { return err }
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND job_id = ? AND id = ? AND product_id = ? AND sku_id = ? AND status = ? AND run_number = ? AND review_status = ? AND result_storage_key <> ''", organizationID, jobID, itemID, candidate.ProductID, candidate.SKUID, model.BatchProductionStatusCompleted, runNumber, model.BatchProductionReviewApproved).First(&item).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND product_id = ? AND sku_id = ? AND id <> ? AND is_primary = ?", organizationID, item.ProductID, item.SKUID, item.ID, true).Update("is_primary", false).Error; err != nil { return err }
		result := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND id = ? AND product_id = ? AND sku_id = ? AND status = ? AND run_number = ? AND review_status = ? AND result_storage_key <> ''", organizationID, jobID, itemID, item.ProductID, item.SKUID, model.BatchProductionStatusCompleted, runNumber, model.BatchProductionReviewApproved).Updates(map[string]any{"is_primary": true, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionStateConflict }
		item.IsPrimary, item.UpdatedAt = true, timestamp
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return item, err
}

func RetryBatchProductionItem(organizationID string, jobID string, itemID string, runNumber int, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND status <> ?", organizationID, jobID, model.BatchProductionStatusCancelled).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		var item model.BatchProductionItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND job_id = ? AND id = ? AND run_number = ?", organizationID, jobID, itemID, runNumber).First(&item).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		if item.Status != model.BatchProductionStatusFailed && (item.Status != model.BatchProductionStatusCompleted || item.ReviewStatus != model.BatchProductionReviewRejected) { return ErrBatchProductionStateConflict }
		var pending int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status IN ?", organizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil { return err }
		if pending+1 > maxBatchProductionPendingItemsPerOrganization { return ErrBatchProductionOrganizationQueueFull }
		if err := reserveBatchProductionRetryCredits(tx, &job, item.EstimatedCredits, timestamp); err != nil { return err }
		result := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND id = ? AND run_number = ? AND (status = ? OR (status = ? AND review_status = ?))", organizationID, jobID, itemID, runNumber, model.BatchProductionStatusFailed, model.BatchProductionStatusCompleted, model.BatchProductionReviewRejected).Updates(map[string]any{"status": model.BatchProductionStatusQueued, "result_storage_key": "", "error_code": "", "retryable": false, "next_attempt_at": "", "error_message": "", "review_status": "", "review_comment": "", "reviewed_by": "", "reviewed_at": "", "is_primary": false, "run_number": gorm.Expr("run_number + 1"), "attempts": 0, "lease_token": "", "lease_expires_at": "", "locked_at": "", "started_at": "", "finished_at": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionStateConflict }
		if err := replaceUserFileReferences(tx, organizationID, "batch_result", item.ID, "batch-result-"+item.ID, nil, true, timestamp); err != nil { return err }
		if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func RetryBatchProductionJob(organizationID string, id string, timestamp string, auditLogs ...model.OrganizationAuditLog) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id = ? AND status IN ?", organizationID, id, []model.BatchProductionStatus{model.BatchProductionStatusFailed, model.BatchProductionStatusPartialSuccess}).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) { return ErrBatchProductionStateConflict } else if err != nil { return err }
		var retryItems []model.BatchProductionItem
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND job_id = ? AND status = ?", organizationID, id, model.BatchProductionStatusFailed).Order("id asc").Find(&retryItems).Error; err != nil { return err }
		if len(retryItems) == 0 { return ErrBatchProductionStateConflict }
		var pending int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status IN ?", organizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil { return err }
		if pending+int64(len(retryItems)) > maxBatchProductionPendingItemsPerOrganization { return ErrBatchProductionOrganizationQueueFull }
		retryCredits := 0
		itemIDs := make([]string, 0, len(retryItems))
		for _, item := range retryItems {
			if item.EstimatedCredits <= 0 { return ErrBatchProductionStateConflict }
			retryCredits += item.EstimatedCredits
			itemIDs = append(itemIDs, item.ID)
		}
		if err := reserveBatchProductionRetryCredits(tx, &job, retryCredits, timestamp); err != nil { return err }
		result := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND job_id = ? AND id IN ? AND status = ?", organizationID, id, itemIDs, model.BatchProductionStatusFailed).Updates(map[string]any{"status": model.BatchProductionStatusQueued, "result_storage_key": "", "error_code": "", "retryable": false, "next_attempt_at": "", "error_message": "", "review_status": "", "review_comment": "", "reviewed_by": "", "reviewed_at": "", "is_primary": false, "run_number": gorm.Expr("run_number + 1"), "attempts": 0, "lease_token": "", "lease_expires_at": "", "locked_at": "", "started_at": "", "finished_at": "", "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != int64(len(retryItems)) { return ErrBatchProductionStateConflict }
		for _, item := range retryItems {
			if err := replaceUserFileReferences(tx, organizationID, "batch_result", item.ID, "batch-result-"+item.ID, nil, true, timestamp); err != nil { return err }
		}
		if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
}

func ClaimNextBatchProductionItem(timestamp string, leaseExpiresAt string, leaseToken string, maxTenantRunning int) (model.BatchProductionItem, model.BatchProductionJob, bool, error) {
	db, err := DB()
	if err != nil { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, err }
	if maxTenantRunning < 1 { maxTenantRunning = 1 }
	if err := failExhaustedBatchProductionItems(db, timestamp, 5); err != nil { return model.BatchProductionItem{}, model.BatchProductionJob{}, false, err }
	for attempt := 0; attempt < 3; attempt++ {
		var item model.BatchProductionItem
		var job model.BatchProductionJob
		claimed := false
		err = db.Transaction(func(tx *gorm.DB) error {
			var candidates []struct {
				OrganizationID  string `gorm:"column:organization_id"`
				BatchClaimCursor string `gorm:"column:batch_claim_cursor"`
			}
			candidate := tx.Table("organizations").
				Select("organizations.id AS organization_id, organizations.batch_claim_cursor AS batch_claim_cursor").
				Where("organizations.status = ?", "active").
				Where("EXISTS (SELECT 1 FROM batch_production_items AS ready_items JOIN batch_production_jobs AS ready_jobs ON ready_jobs.id = ready_items.job_id AND ready_jobs.organization_id = ready_items.organization_id WHERE ready_items.organization_id = organizations.id AND ready_jobs.status IN ? AND ((ready_items.status = ? AND (ready_items.next_attempt_at = '' OR ready_items.next_attempt_at <= ?)) OR (ready_items.status = ? AND ready_items.attempts < ? AND ready_items.lease_expires_at <> '' AND ready_items.lease_expires_at <= ?)))", []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}, model.BatchProductionStatusQueued, timestamp, model.BatchProductionStatusRunning, 5, timestamp).
				Where("(SELECT COUNT(*) FROM batch_production_items AS active_items WHERE active_items.organization_id = organizations.id AND active_items.status = ? AND active_items.lease_expires_at > ?) < ?", model.BatchProductionStatusRunning, timestamp, maxTenantRunning).
				Order("organizations.batch_claim_cursor asc, organizations.id asc").
				Limit(1)
			if err := candidate.Find(&candidates).Error; err != nil { return err }
			if len(candidates) == 0 { return gorm.ErrRecordNotFound }
			selected := candidates[0]
			itemCandidate := tx.Model(&model.BatchProductionItem{}).
				Select("batch_production_items.*").
				Joins("JOIN batch_production_jobs AS jobs ON jobs.id = batch_production_items.job_id AND jobs.organization_id = batch_production_items.organization_id").
				Where("batch_production_items.organization_id = ? AND jobs.status IN ?", selected.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).
				Where("(batch_production_items.status = ? AND (batch_production_items.next_attempt_at = '' OR batch_production_items.next_attempt_at <= ?)) OR (batch_production_items.status = ? AND batch_production_items.attempts < ? AND batch_production_items.lease_expires_at <> '' AND batch_production_items.lease_expires_at <= ?)", model.BatchProductionStatusQueued, timestamp, model.BatchProductionStatusRunning, 5, timestamp).
				Order("batch_production_items.created_at asc, batch_production_items.id asc")
			if err := itemCandidate.First(&item).Error; err != nil { return err }
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ? AND organization_id = ? AND status IN ?", item.JobID, selected.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Error; err != nil { return err }
			var organization model.Organization
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, status, batch_claim_cursor").First(&organization, "id = ?", selected.OrganizationID).Error; err != nil { return err }
			if organization.Status != "active" || organization.BatchClaimCursor != selected.BatchClaimCursor { return nil }
			var activeRunning int64
			if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status = ? AND lease_expires_at > ?", selected.OrganizationID, model.BatchProductionStatusRunning, timestamp).Count(&activeRunning).Error; err != nil { return err }
			if activeRunning >= int64(maxTenantRunning) { return nil }
			var currentItem model.BatchProductionItem
			if err := tx.Where("id = ? AND organization_id = ? AND job_id = ? AND ((status = ? AND (next_attempt_at = '' OR next_attempt_at <= ?)) OR (status = ? AND attempts < ? AND lease_expires_at <> '' AND lease_expires_at <= ?))", item.ID, selected.OrganizationID, job.ID, model.BatchProductionStatusQueued, timestamp, model.BatchProductionStatusRunning, 5, timestamp).First(&currentItem).Error; errors.Is(err, gorm.ErrRecordNotFound) { return nil } else if err != nil { return err }
			item = currentItem
			result := tx.Model(&model.BatchProductionItem{}).
				Where("id = ? AND organization_id = ? AND job_id = ? AND ((status = ? AND (next_attempt_at = '' OR next_attempt_at <= ?)) OR (status = ? AND attempts < ? AND lease_expires_at <> '' AND lease_expires_at <= ?))", item.ID, selected.OrganizationID, job.ID, model.BatchProductionStatusQueued, timestamp, model.BatchProductionStatusRunning, 5, timestamp).
				Updates(map[string]any{"status": model.BatchProductionStatusRunning, "attempts": gorm.Expr("attempts + 1"), "lease_token": leaseToken, "lease_expires_at": leaseExpiresAt, "locked_at": timestamp, "started_at": timestamp, "finished_at": "", "result_storage_key": "", "error_code": "", "retryable": false, "next_attempt_at": "", "error_message": "", "review_status": "", "review_comment": "", "reviewed_by": "", "reviewed_at": "", "is_primary": false, "updated_at": timestamp})
			if result.Error != nil { return result.Error }
			if result.RowsAffected == 0 { return nil }
			if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
			if err := tx.Model(&model.Organization{}).Where("id = ?", organization.ID).Update("batch_claim_cursor", timestamp+":"+leaseToken).Error; err != nil { return err }
			claimed = true
			item.Status, item.LockedAt, item.StartedAt, item.UpdatedAt = model.BatchProductionStatusRunning, timestamp, timestamp, timestamp
			item.LeaseToken, item.LeaseExpiresAt = leaseToken, leaseExpiresAt
			item.FinishedAt, item.ResultStorageKey, item.ErrorMessage, item.NextAttemptAt = "", "", "", ""
			item.ErrorCode, item.Retryable = "", false
			item.ReviewStatus, item.ReviewComment, item.ReviewedBy, item.ReviewedAt = "", "", "", ""
			item.IsPrimary = false
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
		if err := tx.Where("status = ? AND attempts >= ? AND lease_expires_at <> '' AND lease_expires_at <= ?", model.BatchProductionStatusRunning, maxAttempts, timestamp).Order("organization_id asc, job_id asc, id asc").Limit(500).Find(&items).Error; err != nil { return err }
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
			if err := tx.Model(&model.BatchProductionItem{}).Where("id IN ? AND organization_id = ? AND job_id = ? AND status = ? AND attempts >= ? AND lease_expires_at <= ?", ids, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, maxAttempts, timestamp).Updates(map[string]any{"status": model.BatchProductionStatusFailed, "error_code": model.BatchProductionErrorCancelledLeaseLost, "retryable": false, "next_attempt_at": "", "error_message": "执行租约连续超时", "lease_token": "", "lease_expires_at": "", "locked_at": "", "finished_at": timestamp, "updated_at": timestamp}).Error; err != nil { return err }
			if err := aggregateBatchProductionJob(tx, &job, timestamp); err != nil { return err }
		}
		return nil
	})
}

func FinishBatchProductionItem(item model.BatchProductionItem, status model.BatchProductionStatus, resultStorageKey string, errorMessage string, timestamp string) error {
	return FinishBatchProductionItemWithSchedule(item, status, resultStorageKey, errorMessage, "", false, false, "", timestamp)
}

func FinishBatchProductionItemWithSchedule(item model.BatchProductionItem, status model.BatchProductionStatus, resultStorageKey string, errorMessage string, errorCategory model.BatchProductionErrorCategory, retryable bool, reserveRetryCredits bool, nextAttemptAt string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var job model.BatchProductionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ? AND organization_id = ?", item.JobID, item.OrganizationID).Error; err != nil { return err }
		if job.Status == model.BatchProductionStatusCancelled || job.Status == model.BatchProductionStatusCompleted || job.Status == model.BatchProductionStatusFailed || job.Status == model.BatchProductionStatusPendingReview || job.Status == model.BatchProductionStatusPartialSuccess { return ErrBatchProductionLeaseLost }
		if retryable {
			if status != model.BatchProductionStatusQueued || errorCategory == "" || nextAttemptAt == "" || item.EstimatedCredits <= 0 {
				return ErrBatchProductionStateConflict
			}
			if reserveRetryCredits {
				if err := reserveBatchProductionRetryCredits(tx, &job, item.EstimatedCredits, timestamp); err != nil {
					if !errors.Is(err, ErrInsufficientUserCredits) && !errors.Is(err, ErrInsufficientOrganizationCredits) && !errors.Is(err, ErrOrganizationCreditBudgetExceeded) {
						return err
					}
					status = model.BatchProductionStatusFailed
					errorCategory = model.BatchProductionErrorPricingCredit
					retryable = false
					nextAttemptAt = ""
					errorMessage = "自动重试额度不足"
				}
			}
		}
		reviewStatus := model.BatchProductionReviewStatus("")
		if status == model.BatchProductionStatusCompleted {
			reviewStatus = model.BatchProductionReviewPending
			errorCategory, retryable, nextAttemptAt, errorMessage = "", false, "", ""
		} else if status == model.BatchProductionStatusFailed {
			if errorCategory == "" { return ErrBatchProductionStateConflict }
			retryable, nextAttemptAt = false, ""
		} else if status != model.BatchProductionStatusQueued {
			return ErrBatchProductionStateConflict
		}
		finishedAt := timestamp
		if status == model.BatchProductionStatusQueued { finishedAt = "" }
		updates := map[string]any{"status": status, "result_storage_key": resultStorageKey, "error_code": errorCategory, "retryable": retryable, "next_attempt_at": nextAttemptAt, "error_message": errorMessage, "review_status": reviewStatus, "review_comment": "", "reviewed_by": "", "reviewed_at": "", "is_primary": false, "finished_at": finishedAt, "locked_at": "", "lease_token": "", "lease_expires_at": "", "updated_at": timestamp}
		if status == model.BatchProductionStatusQueued {
			updates["run_number"] = gorm.Expr("run_number + 1")
		}
		result := tx.Model(&model.BatchProductionItem{}).Where("id = ? AND organization_id = ? AND job_id = ? AND status = ? AND lease_token = ?", item.ID, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, item.LeaseToken).Updates(updates)
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionLeaseLost }
		storageKeys := []string{}
		if resultStorageKey != "" { storageKeys = append(storageKeys, resultStorageKey) }
		if err := replaceUserFileReferences(tx, item.OrganizationID, "batch_result", item.ID, "batch-result-"+item.ID, storageKeys, status != model.BatchProductionStatusCompleted, timestamp); err != nil { return err }
		return aggregateBatchProductionJob(tx, &job, timestamp)
	})
}

func RenewBatchProductionItemLease(item model.BatchProductionItem, leaseExpiresAt string, timestamp string) error {
	db, err := DB()
	if err != nil { return err }
	return db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&organization, "id = ?", item.OrganizationID).Error; err != nil { return err }
		result := tx.Model(&model.BatchProductionItem{}).
			Where("id = ? AND organization_id = ? AND job_id = ? AND status = ? AND lease_token = ?", item.ID, item.OrganizationID, item.JobID, model.BatchProductionStatusRunning, item.LeaseToken).
			Updates(map[string]any{"lease_expires_at": leaseExpiresAt, "locked_at": timestamp, "updated_at": timestamp})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return ErrBatchProductionLeaseLost }
		return nil
	})
}
