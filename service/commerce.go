package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
	"gorm.io/gorm"
)

func EnsureOrganization(user model.AuthUser) (model.Organization, model.OrganizationMember, error) {
	if strings.TrimSpace(user.OrganizationID) != "" {
		return ResolveOrganizationAccess(user, user.OrganizationID)
	}
	if organization, membership, ok, err := repository.GetOrganizationContext(user.ID); err != nil || ok {
		return organization, membership, err
	}
	timestamp, organizationID := now(), newID("org")
	return repository.EnsureDefaultOrganization(user, organizationID, newID("member"), timestamp, newAuditLog(user.ID, organizationID, "organization.create", "organization", organizationID, "default", timestamp))
}

func EnsureSessionOrganization(user model.AuthUser) (model.Organization, model.OrganizationMember, error) {
	if organization, membership, ok, err := repository.GetOrganizationContext(user.ID); err != nil || ok {
		return organization, membership, err
	}
	timestamp, organizationID := now(), newID("org")
	return repository.EnsureDefaultOrganization(user, organizationID, newID("member"), timestamp, newAuditLog(user.ID, organizationID, "organization.create", "organization", organizationID, "default", timestamp))
}

func ResolveOrganizationAccess(user model.AuthUser, organizationID string) (model.Organization, model.OrganizationMember, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "缺少企业上下文"}
	}
	membership, ok, err := repository.GetOrganizationMember(organizationID, user.ID)
	if err != nil {
		return model.Organization{}, model.OrganizationMember{}, err
	}
	if !ok {
		return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "你不是该企业成员"}
	}
	organization, ok, err := repository.GetOrganization(organizationID)
	if err != nil {
		return model.Organization{}, model.OrganizationMember{}, err
	}
	if !ok || organization.Status != "active" {
		return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "企业不存在或已停用"}
	}
	return organization, membership, nil
}

func RequireOrganizationWrite(user model.AuthUser) error {
	_, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "当前企业角色没有生成权限"}
	}
	return nil
}

func CreateOrganization(user model.AuthUser, name string) (model.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return model.Organization{}, safeMessageError{message: "企业名称不能为空或超过 200 个字符"}
	}
	timestamp := now()
	organization := model.Organization{ID: newID("org"), Name: name, Slug: newID("workspace"), Status: "active", Version: 1, CreditMode: model.OrganizationCreditModePersonal, CreditAlertThreshold: 80, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	membership := model.OrganizationMember{ID: newID("member"), OrganizationID: organization.ID, UserID: user.ID, Role: model.OrganizationRoleOwner, Version: 1, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.CreateOrganization(organization, membership, true, newAuditLog(user.ID, organization.ID, "organization.create", "organization", organization.ID, name, timestamp))
	return result, err
}

func OrganizationWorkspace(user model.AuthUser) (model.OrganizationWorkspace, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationWorkspace{}, err
	}
	organizations, err := repository.ListUserOrganizations(user.ID)
	if err != nil {
		return model.OrganizationWorkspace{}, err
	}
	stats, err := repository.OrganizationStats(organization.ID)
	return model.OrganizationWorkspace{Organization: organization, Membership: membership, Organizations: organizations, Stats: stats, CreditSummary: organizationCreditSummary(organization, user.Credits)}, err
}

func UpdateOrganizationCreditSettings(user model.AuthUser, mode model.OrganizationCreditMode, monthlyBudget int, alertThreshold int, expectedVersion int64) (model.OrganizationCreditSummary, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationCreditSummary{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "没有企业额度管理权限"}
	}
	if mode != model.OrganizationCreditModePersonal && mode != model.OrganizationCreditModeShared {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "企业扣费模式无效"}
	}
	if monthlyBudget < 0 || monthlyBudget > 1000000000 {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "月度预算应在 0 到 10 亿之间"}
	}
	if alertThreshold < 1 || alertThreshold > 100 {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "预算预警比例应在 1% 到 100% 之间"}
	}
	if expectedVersion <= 0 {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "企业额度设置已变化，请刷新后重试"}
	}
	timestamp := now()
	organization, err = repository.SaveOrganizationCreditSettings(organization.ID, mode, monthlyBudget, alertThreshold, expectedVersion, currentCreditMonth(), timestamp, newAuditLog(user.ID, organization.ID, "organization.credit.settings", "organization", organization.ID, string(mode), timestamp))
	if errors.Is(err, repository.ErrOrganizationVersionConflict) {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "企业额度设置已被其他管理员更新，请刷新后重试"}
	}
	return organizationCreditSummary(organization, user.Credits), err
}

func TransferOrganizationCredits(user model.AuthUser, amount int) (model.OrganizationCreditSummary, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationCreditSummary{}, err
	}
	if amount <= 0 || amount > 1000000000 {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "转入额度应在 1 到 10 亿之间"}
	}
	timestamp := now()
	personalLog := model.CreditLog{ID: newID("credit"), UserID: user.ID, OrganizationID: organization.ID, CreditSource: model.CreditSourcePersonal, Type: model.CreditLogTypeOrganizationTransferOut, Amount: -amount, RelatedID: organization.ID, Remark: "转入企业共享额度", CreatedAt: timestamp}
	organizationLog := model.CreditLog{ID: newID("credit"), UserID: user.ID, OrganizationID: organization.ID, CreditSource: model.CreditSourceOrganization, Type: model.CreditLogTypeOrganizationTransferIn, Amount: amount, RelatedID: organization.ID, Remark: "成员转入企业共享额度", CreatedAt: timestamp}
	organization, savedUser, err := repository.TransferCreditsToOrganization(organization.ID, user.ID, amount, timestamp, personalLog, organizationLog, newAuditLog(user.ID, organization.ID, "organization.credit.transfer", "organization", organization.ID, "personal_to_shared", timestamp))
	if errors.Is(err, repository.ErrInsufficientUserCredits) {
		return model.OrganizationCreditSummary{}, safeMessageError{message: "个人算力余额不足"}
	}
	return organizationCreditSummary(organization, savedUser.Credits), err
}

func organizationCreditSummary(organization model.Organization, personalBalance int) model.OrganizationCreditSummary {
	mode := organization.CreditMode
	if mode == "" {
		mode = model.OrganizationCreditModePersonal
	}
	threshold := organization.CreditAlertThreshold
	if threshold <= 0 {
		threshold = 80
	}
	monthlyUsed := organization.MonthlyCreditsUsed
	if organization.CreditBudgetMonth != currentCreditMonth() {
		monthlyUsed = 0
	}
	warning := organization.MonthlyCreditBudget > 0 && monthlyUsed*100 >= organization.MonthlyCreditBudget*threshold
	return model.OrganizationCreditSummary{Mode: mode, Balance: organization.Credits, PersonalBalance: personalBalance, MonthlyBudget: organization.MonthlyCreditBudget, MonthlyUsed: monthlyUsed, AlertThreshold: threshold, Warning: warning}
}

func currentCreditMonth() string { return time.Now().UTC().Format("2006-01") }

func ListOrganizationMembers(user model.AuthUser, q model.Query) (model.OrganizationMemberList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationMemberList{}, err
	}
	items, total, err := repository.ListOrganizationMembers(organization.ID, q)
	return model.OrganizationMemberList{Items: items, Total: int(total)}, err
}

func ListCurrentOrganizationInvitations(user model.AuthUser) ([]model.OrganizationInvitation, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return nil, err
	}
	if !canManageOrganization(membership.Role) {
		return []model.OrganizationInvitation{}, nil
	}
	return repository.ListOrganizationInvitations(organization.ID, now())
}

func SwitchOrganization(user model.AuthUser, organizationID string) error {
	if _, _, err := ResolveOrganizationAccess(user, organizationID); err != nil {
		return err
	}
	if err := repository.SwitchUserOrganization(user.ID, strings.TrimSpace(organizationID), newAuditLog(user.ID, organizationID, "organization.switch", "organization", organizationID, "", now())); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return safeMessageError{message: "你不是该企业成员"}
		}
		return err
	}
	return nil
}

func UpdateOrganization(user model.AuthUser, name string, expectedVersion int64) (model.Organization, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return organization, err
	}
	if !canManageOrganization(membership.Role) {
		return organization, safeMessageError{message: "没有企业管理权限"}
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return organization, safeMessageError{message: "企业名称不能为空或超过 200 个字符"}
	}
	if expectedVersion <= 0 {
		return organization, safeMessageError{message: "企业数据已变化，请刷新后重试"}
	}
	organization.Name = name
	organization.Version = expectedVersion + 1
	organization.UpdatedAt = now()
	result, err := repository.SaveOrganization(organization, newAuditLog(user.ID, organization.ID, "organization.update", "organization", organization.ID, name, organization.UpdatedAt))
	if errors.Is(err, repository.ErrOrganizationVersionConflict) {
		return organization, safeMessageError{message: "企业信息已被其他管理员更新，请刷新后重试"}
	}
	return result, err
}

func InviteOrganizationMember(user model.AuthUser, email string, role model.OrganizationRole) (model.OrganizationInvitation, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationInvitation{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.OrganizationInvitation{}, safeMessageError{message: "没有成员管理权限"}
	}
	email, err = normalizeEmailAddress(email)
	if err != nil {
		return model.OrganizationInvitation{}, safeMessageError{message: "请输入有效邮箱"}
	}
	if !validAssignableRole(role) {
		return model.OrganizationInvitation{}, safeMessageError{message: "成员角色无效"}
	}
	if invitations, listErr := repository.ListOrganizationInvitations(organization.ID, now()); listErr != nil {
		return model.OrganizationInvitation{}, listErr
	} else {
		for _, item := range invitations {
			if item.Status == model.OrganizationInvitationPending && strings.EqualFold(item.Email, email) {
				return model.OrganizationInvitation{}, safeMessageError{message: "该邮箱已有待接受邀请"}
			}
		}
	}
	timestamp := now()
	invitation := model.OrganizationInvitation{ID: newID("invite"), OrganizationID: organization.ID, Email: email, Role: role, Status: model.OrganizationInvitationPending, InvitedBy: user.ID, ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(timestampLayout), CreatedAt: timestamp, UpdatedAt: timestamp}
	outbox := model.OrganizationEmailOutbox{ID: newID("email"), OrganizationID: organization.ID, UserID: user.ID, InvitationID: invitation.ID, Receiver: email, OrganizationName: organization.Name, Role: role, Status: "pending", NextAttemptAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.SaveOrganizationInvitation(invitation, outbox, newAuditLog(user.ID, organization.ID, "member.invite", "invitation", invitation.ID, email, timestamp))
	if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) {
		return model.OrganizationInvitation{}, safeMessageError{message: "该邮箱已有待接受邀请"}
	}
	if err != nil {
		return model.OrganizationInvitation{}, err
	}
	return result, err
}

func ListPendingOrganizationInvitations(user model.AuthUser) ([]model.OrganizationInvitation, error) {
	if strings.TrimSpace(user.Email) == "" {
		return []model.OrganizationInvitation{}, nil
	}
	return repository.ListUserInvitations(user.Email, now())
}

func RevokeOrganizationInvitation(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canManageOrganization(membership.Role) {
		return safeMessageError{message: "没有成员管理权限"}
	}
	timestamp := now()
	if err := repository.RevokeOrganizationInvitation(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "member.invitation.revoke", "invitation", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) {
			return safeMessageError{message: "邀请不存在或状态已变化"}
		}
		return err
	}
	return nil
}

func AcceptOrganizationInvitation(user model.AuthUser, id string) error {
	invitation, ok, err := repository.GetOrganizationInvitation(id)
	if err != nil {
		return err
	}
	if !ok || invitation.Status != model.OrganizationInvitationPending || !strings.EqualFold(invitation.Email, user.Email) {
		return safeMessageError{message: "邀请不存在或不属于当前账号"}
	}
	expiresAt, err := time.Parse(time.RFC3339, invitation.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) {
		return safeMessageError{message: "邀请已过期"}
	}
	if _, exists, err := repository.GetOrganizationMember(invitation.OrganizationID, user.ID); err != nil {
		return err
	} else if exists {
		return safeMessageError{message: "你已经是该企业成员"}
	}
	timestamp := now()
	membership := model.OrganizationMember{ID: newID("member"), OrganizationID: invitation.OrganizationID, UserID: user.ID, Role: invitation.Role, Version: 1, CreatedAt: timestamp, UpdatedAt: timestamp}
	if err := repository.AcceptOrganizationInvitation(invitation, membership, timestamp, newAuditLog(user.ID, invitation.OrganizationID, "member.accept", "invitation", invitation.ID, user.Email, timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) {
			return safeMessageError{message: "邀请已过期、已撤销或已被接受"}
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return safeMessageError{message: "受邀企业不存在或已停用"}
		}
		return err
	}
	return nil
}

func UpdateOrganizationMember(user model.AuthUser, memberID string, role model.OrganizationRole, expectedVersion int64) (model.OrganizationMember, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.OrganizationMember{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.OrganizationMember{}, safeMessageError{message: "没有成员管理权限"}
	}
	if !validAssignableRole(role) {
		return model.OrganizationMember{}, safeMessageError{message: "成员角色无效"}
	}
	if expectedVersion <= 0 {
		return model.OrganizationMember{}, safeMessageError{message: "成员数据已变化，请刷新后重试"}
	}
	target, ok, err := repository.GetOrganizationMemberByID(organization.ID, memberID)
	if err != nil {
		return model.OrganizationMember{}, err
	}
	if !ok || target.Role == model.OrganizationRoleOwner {
		return model.OrganizationMember{}, safeMessageError{message: "不能修改企业所有者"}
	}
	item := model.OrganizationMember{ID: target.ID, OrganizationID: organization.ID, UserID: target.UserID, Role: role, Version: expectedVersion + 1, CreatedAt: target.CreatedAt, UpdatedAt: now()}
	result, err := repository.SaveOrganizationMember(item, newAuditLog(user.ID, organization.ID, "member.update", "member", item.ID, string(role), item.UpdatedAt))
	if errors.Is(err, repository.ErrOrganizationOwnershipChanged) {
		return model.OrganizationMember{}, safeMessageError{message: "成员角色已变化，请刷新后重试"}
	}
	if errors.Is(err, repository.ErrOrganizationMemberVersionConflict) {
		return model.OrganizationMember{}, safeMessageError{message: "成员信息已被其他管理员更新，请刷新后重试"}
	}
	return result, err
}

func RemoveOrganizationMember(user model.AuthUser, memberID string, expectedVersion int64) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canManageOrganization(membership.Role) {
		return safeMessageError{message: "没有成员管理权限"}
	}
	if memberID == membership.ID {
		return safeMessageError{message: "不能从当前企业移除自己"}
	}
	if expectedVersion <= 0 {
		return safeMessageError{message: "成员数据已变化，请刷新后重试"}
	}
	timestamp := now()
	if err := repository.DeleteOrganizationMember(organization.ID, memberID, expectedVersion, newAuditLog(user.ID, organization.ID, "member.remove", "member", memberID, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationOwnershipChanged) {
			return safeMessageError{message: "成员角色已变化，不能移除企业所有者"}
		}
		if errors.Is(err, repository.ErrOrganizationMemberVersionConflict) {
			return safeMessageError{message: "成员信息已被其他管理员更新，请刷新后重试"}
		}
		return err
	}
	return nil
}

func TransferOrganizationOwnership(user model.AuthUser, memberID string, targetExpectedVersion int64) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if membership.Role != model.OrganizationRoleOwner {
		return safeMessageError{message: "只有企业所有者可以转移所有权"}
	}
	if memberID == membership.ID {
		return safeMessageError{message: "你已经是企业所有者"}
	}
	if targetExpectedVersion <= 0 {
		return safeMessageError{message: "成员数据已变化，请刷新后重试"}
	}
	timestamp := now()
	if err := repository.TransferOrganizationOwnership(organization.ID, membership.ID, memberID, targetExpectedVersion, timestamp, newAuditLog(user.ID, organization.ID, "organization.transfer_owner", "member", memberID, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationOwnershipChanged) {
			return safeMessageError{message: "企业所有权已变化，请刷新后重试"}
		}
		if errors.Is(err, repository.ErrOrganizationMemberVersionConflict) {
			return safeMessageError{message: "目标成员信息已被其他管理员更新，请刷新后重试"}
		}
		return err
	}
	return nil
}

func ListBrands(user model.AuthUser, q model.Query) (model.BrandList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.BrandList{}, err
	}
	items, total, err := repository.ListBrands(organization.ID, q)
	return model.BrandList{Items: items, Total: int(total)}, err
}

func SaveBrand(user model.AuthUser, item model.Brand) (model.Brand, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return item, err
	}
	if !canWriteCommerce(membership.Role) {
		return item, safeMessageError{message: "没有品牌编辑权限"}
	}
	item.Name, item.LogoStorageKey = strings.TrimSpace(item.Name), strings.TrimSpace(item.LogoStorageKey)
	if item.Name == "" || len(item.Name) > 200 {
		return item, safeMessageError{message: "品牌名称不能为空或过长"}
	}
	if item.LogoStorageKey != "" && !validCommerceImageStorageKey(item.LogoStorageKey) {
		return item, safeMessageError{message: "品牌 Logo 存储编号无效"}
	}
	create := item.ID == ""
	timestamp := now()
	if item.ID != "" {
		if item.Version <= 0 {
			return item, safeMessageError{message: "品牌数据已变化，请刷新后重试"}
		}
		if saved, ok, err := repository.GetBrand(organization.ID, item.ID); err != nil {
			return item, err
		} else if !ok {
			return item, safeMessageError{message: "品牌不存在"}
		} else {
			item.CreatedAt, item.CreatedBy, item.Version = saved.CreatedAt, saved.CreatedBy, item.Version+1
		}
	}
	if item.ID == "" {
		item.ID, item.CreatedAt, item.CreatedBy, item.Version = newID("brand"), timestamp, user.ID, 1
	}
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	if !validCommerceRecord(item) {
		return item, safeMessageError{message: "品牌规范内容不能超过 64KB"}
	}
	action := "brand.update"
	if create {
		action = "brand.create"
	}
	result, err := repository.SaveBrand(item, create, newAuditLog(user.ID, organization.ID, action, "brand", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrBrandNameConflict) {
		return item, safeMessageError{message: "企业内品牌名称不能重复"}
	}
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return item, safeMessageError{message: "品牌已被其他成员更新，请刷新后重试"}
	}
	if errors.Is(err, repository.ErrWorkspaceFileMissing) {
		return item, safeMessageError{message: "品牌 Logo 不存在，请重新上传"}
	}
	return result, err
}

func DeleteBrand(user model.AuthUser, id string, expectedVersion int64) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有品牌编辑权限"}
	}
	timestamp := now()
	if err := repository.DeleteBrand(organization.ID, id, expectedVersion, timestamp, newAuditLog(user.ID, organization.ID, "brand.delete", "brand", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBrandInUse) {
			return safeMessageError{message: "品牌已被商品或生产任务引用，不能删除"}
		}
		if errors.Is(err, repository.ErrCommerceVersionConflict) {
			return safeMessageError{message: "品牌已被其他成员更新，请刷新后重试"}
		}
		return err
	}
	return nil
}

func ListProducts(user model.AuthUser, q model.Query) (model.ProductList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductList{}, err
	}
	items, total, err := repository.ListProducts(organization.ID, q)
	return model.ProductList{Items: items, Total: int(total)}, err
}

func GetProduct(user model.AuthUser, id string) (model.Product, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.Product{}, err
	}
	item, ok, err := repository.GetProduct(organization.ID, strings.TrimSpace(id))
	if err != nil {
		return model.Product{}, err
	}
	if !ok {
		return model.Product{}, safeMessageError{message: "商品不存在"}
	}
	if item.BrandID != "" {
		if brand, ok, err := repository.GetBrand(organization.ID, item.BrandID); err != nil {
			return model.Product{}, err
		} else if ok {
			item.BrandName = brand.Name
		}
	}
	return item, nil
}

func UpdateProductStatuses(user model.AuthUser, input model.UpdateProductStatusesInput) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有商品编辑权限"}
	}
	if !validProductStatus(input.Status) {
		return safeMessageError{message: "商品状态无效"}
	}
	if len(input.Items) == 0 || len(input.Items) > 100 {
		return safeMessageError{message: "单次最多批量更新 100 个商品"}
	}
	items := make([]model.ProductStatusItemInput, 0, len(input.Items))
	seen := map[string]bool{}
	for _, item := range input.Items {
		item.ID = strings.TrimSpace(item.ID)
		if item.ID == "" || item.Version <= 0 {
			return safeMessageError{message: "商品编号或数据版本无效，请刷新后重试"}
		}
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, item)
	}
	timestamp := now()
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	err = repository.UpdateProductStatuses(organization.ID, items, input.Status, timestamp, newAuditLog(user.ID, organization.ID, "product.batch_status", "product", "", map[string]any{"status": input.Status, "productIds": ids}, timestamp))
	if errors.Is(err, repository.ErrCommerceVersionConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
		return safeMessageError{message: "部分商品已被其他成员更新，请刷新后重试"}
	}
	return err
}

func SaveProduct(user model.AuthUser, item model.Product) (model.Product, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return item, err
	}
	if !canWriteCommerce(membership.Role) {
		return item, safeMessageError{message: "没有商品编辑权限"}
	}
	item.Name, item.Code = strings.TrimSpace(item.Name), strings.TrimSpace(item.Code)
	if item.Name == "" || item.Code == "" || len(item.Name) > 200 || len(item.Code) > 120 {
		return item, safeMessageError{message: "商品名称或 SPU 编码为空或过长"}
	}
	create := item.ID == ""
	if item.BrandID != "" {
		if _, ok, err := repository.GetBrand(organization.ID, item.BrandID); err != nil {
			return item, err
		} else if !ok {
			return item, safeMessageError{message: "所属品牌不存在"}
		}
	}
	if item.Status == "" {
		item.Status = model.ProductStatusDraft
	}
	if !validProductStatus(item.Status) {
		return item, safeMessageError{message: "商品状态无效"}
	}
	timestamp := now()
	if item.ID != "" {
		if item.Version <= 0 {
			return item, safeMessageError{message: "商品数据已变化，请刷新后重试"}
		}
		if saved, ok, err := repository.GetProduct(organization.ID, item.ID); err != nil {
			return item, err
		} else if !ok {
			return item, safeMessageError{message: "商品不存在"}
		} else {
			item.CreatedAt, item.CreatedBy, item.Version = saved.CreatedAt, saved.CreatedBy, item.Version+1
		}
	}
	if item.ID == "" {
		item.ID, item.CreatedAt, item.CreatedBy, item.Version = newID("product"), timestamp, user.ID, 1
	}
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	if !validCommerceRecord(item) {
		return item, safeMessageError{message: "商品内容不能超过 64KB"}
	}
	action := "product.update"
	if create {
		action = "product.create"
	}
	result, err := repository.SaveProduct(item, create, newAuditLog(user.ID, organization.ID, action, "product", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrProductCodeConflict) {
		return item, safeMessageError{message: "企业内 SPU 编码不能重复"}
	}
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return item, safeMessageError{message: "商品已被其他成员更新，请刷新后重试"}
	}
	return result, err
}

func DeleteProduct(user model.AuthUser, id string, expectedVersion int64) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有商品编辑权限"}
	}
	timestamp := now()
	if err := repository.DeleteProduct(organization.ID, id, expectedVersion, timestamp, newAuditLog(user.ID, organization.ID, "product.delete", "product", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrProductInUse) {
			return safeMessageError{message: "商品已被生产任务引用，不能删除"}
		}
		if errors.Is(err, repository.ErrCommerceVersionConflict) {
			return safeMessageError{message: "商品已被其他成员更新，请刷新后重试"}
		}
		return err
	}
	return nil
}

func ListProductSKUs(user model.AuthUser, productID string, q model.Query) (model.ProductSKUList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductSKUList{}, err
	}
	if _, ok, err := repository.GetProduct(organization.ID, productID); err != nil || !ok {
		if err != nil {
			return model.ProductSKUList{}, err
		}
		return model.ProductSKUList{}, safeMessageError{message: "商品不存在"}
	}
	items, total, err := repository.ListProductSKUs(organization.ID, productID, q)
	return model.ProductSKUList{Items: items, Total: int(total)}, err
}

func SaveProductSKU(user model.AuthUser, item model.ProductSKU) (model.ProductSKU, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return item, err
	}
	if !canWriteCommerce(membership.Role) {
		return item, safeMessageError{message: "没有 SKU 编辑权限"}
	}
	if _, ok, err := repository.GetProduct(organization.ID, item.ProductID); err != nil || !ok {
		if err != nil {
			return item, err
		}
		return item, safeMessageError{message: "商品不存在"}
	}
	item.Code, item.Name = strings.TrimSpace(item.Code), strings.TrimSpace(item.Name)
	if item.Code == "" || item.Name == "" || len(item.Code) > 120 || len(item.Name) > 200 {
		return item, safeMessageError{message: "SKU 编码或名称为空或过长"}
	}
	if len(item.ImageStorageKeys) > 50 {
		return item, safeMessageError{message: "单个 SKU 最多保存 50 张参考图"}
	}
	seenImageStorageKeys := make(map[string]bool, len(item.ImageStorageKeys))
	imageStorageKeys := make([]string, 0, len(item.ImageStorageKeys))
	for _, storageKey := range item.ImageStorageKeys {
		storageKey = strings.TrimSpace(storageKey)
		if !validCommerceImageStorageKey(storageKey) {
			return item, safeMessageError{message: "SKU 参考图存储编号无效"}
		}
		if !seenImageStorageKeys[storageKey] {
			seenImageStorageKeys[storageKey] = true
			imageStorageKeys = append(imageStorageKeys, storageKey)
		}
	}
	item.ImageStorageKeys = imageStorageKeys
	create := item.ID == ""
	if item.Status == "" {
		item.Status = model.ProductStatusActive
	}
	if !validProductStatus(item.Status) {
		return item, safeMessageError{message: "SKU 状态无效"}
	}
	timestamp := now()
	if item.ID != "" {
		if item.Version <= 0 {
			return item, safeMessageError{message: "SKU 数据已变化，请刷新后重试"}
		}
		if saved, ok, err := repository.GetProductSKU(organization.ID, item.ID); err != nil {
			return item, err
		} else if !ok {
			return item, safeMessageError{message: "SKU 不存在"}
		} else if saved.ProductID != item.ProductID {
			return item, safeMessageError{message: "SKU 创建后不能变更所属商品"}
		} else {
			item.CreatedAt, item.CreatedBy, item.Version = saved.CreatedAt, saved.CreatedBy, item.Version+1
		}
	}
	if item.ID == "" {
		item.ID, item.CreatedAt, item.CreatedBy, item.Version = newID("sku"), timestamp, user.ID, 1
	}
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	if !validCommerceRecord(item) {
		return item, safeMessageError{message: "SKU 内容不能超过 64KB"}
	}
	action := "sku.update"
	if create {
		action = "sku.create"
	}
	result, err := repository.SaveProductSKU(item, create, newAuditLog(user.ID, organization.ID, action, "sku", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrProductSKUCodeConflict) {
		return item, safeMessageError{message: "企业内 SKU 编码不能重复"}
	}
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return item, safeMessageError{message: "SKU 已被其他成员更新，请刷新后重试"}
	}
	if errors.Is(err, repository.ErrWorkspaceFileMissing) {
		return item, safeMessageError{message: "SKU 参考图不存在，请重新上传"}
	}
	return result, err
}

func DeleteProductSKU(user model.AuthUser, id string, expectedVersion int64) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有 SKU 编辑权限"}
	}
	timestamp := now()
	if err := repository.DeleteProductSKU(organization.ID, id, expectedVersion, timestamp, newAuditLog(user.ID, organization.ID, "sku.delete", "sku", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrProductSKUInUse) {
			return safeMessageError{message: "SKU 已被生产任务引用，不能删除"}
		}
		if errors.Is(err, repository.ErrCommerceVersionConflict) {
			return safeMessageError{message: "SKU 已被其他成员更新，请刷新后重试"}
		}
		return err
	}
	return nil
}

func ListProductionTemplates(user model.AuthUser, q model.Query) (model.ProductionTemplateList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductionTemplateList{}, err
	}
	q.Normalize()
	items, total, err := repository.ListProductionTemplates(organization.ID, q)
	if err != nil {
		return model.ProductionTemplateList{}, err
	}
	if q.Page == 1 && (q.Type == "" || q.Type == "all" || q.Type == string(model.ProductionTemplateStatusActive)) {
		keyword := strings.ToLower(strings.TrimSpace(q.Keyword))
		builtins := make([]model.ProductionTemplate, 0, len(builtinProductionTemplates))
		for _, definition := range builtinProductionTemplates {
			if keyword != "" && !strings.Contains(strings.ToLower(definition.Name+" "+definition.Description), keyword) {
				continue
			}
			builtins = append(builtins, model.ProductionTemplate{ID: definition.ID, Name: definition.Name, Description: definition.Description, Source: model.ProductionTemplateSourceBuiltin, MediaType: model.ProductionTemplateMediaTypeImage, TemplateType: definition.TemplateType, Platform: definition.Platform, Status: model.ProductionTemplateStatusActive, CurrentVersion: 1, CurrentPrompt: definition.Prompt, CurrentSpec: definition.SpecJSON, Version: 1})
		}
		items = append(builtins, items...)
		total += int64(len(builtins))
	}
	return model.ProductionTemplateList{Items: items, Total: int(total)}, nil
}

func ListProductionTemplateVersions(user model.AuthUser, id string) ([]model.ProductionTemplateVersion, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if _, ok, err := repository.GetProductionTemplate(organization.ID, id); err != nil {
		return nil, err
	} else if !ok {
		return nil, safeMessageError{message: "生产模板不存在"}
	}
	return repository.ListProductionTemplateVersions(organization.ID, id)
}

func SaveProductionTemplate(user model.AuthUser, input model.SaveProductionTemplateInput) (model.ProductionTemplate, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.ProductionTemplate{}, safeMessageError{message: "没有生产模板管理权限"}
	}
	input.ID, input.Name, input.Description, input.Prompt = strings.TrimSpace(input.ID), strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.Prompt)
	input.Source = model.ProductionTemplateSource(strings.TrimSpace(string(input.Source)))
	input.MediaType = model.ProductionTemplateMediaType(strings.TrimSpace(string(input.MediaType)))
	input.TemplateType = model.ProductionTemplateType(strings.TrimSpace(string(input.TemplateType)))
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Source == "" {
		input.Source = model.ProductionTemplateSourceOrganization
	}
	if input.MediaType == "" {
		input.MediaType = model.ProductionTemplateMediaTypeImage
	}
	if input.TemplateType == "" {
		input.TemplateType = model.ProductionTemplateTypeCustom
	}
	if input.Platform == "" {
		input.Platform = "original"
	}
	if input.Status == "" {
		input.Status = model.ProductionTemplateStatusDraft
	}
	if input.Status == model.ProductionTemplateStatusActive {
		return model.ProductionTemplate{}, safeMessageError{message: "启用模板请使用发布操作"}
	}
	if input.Name == "" || len([]rune(input.Name)) > 120 || len([]rune(input.Description)) > 500 || input.Prompt == "" || len([]rune(input.Prompt)) > 12000 || input.Source != model.ProductionTemplateSourceOrganization || input.MediaType != model.ProductionTemplateMediaTypeImage || (input.Status != model.ProductionTemplateStatusDraft && input.Status != model.ProductionTemplateStatusDisabled) {
		return model.ProductionTemplate{}, safeMessageError{message: "模板名称、说明、类型、状态或提示词无效"}
	}
	if err := validateProductionTemplateVariables(input.Prompt); err != nil {
		return model.ProductionTemplate{}, err
	}
	specJSON, _, err := normalizeProductionTemplateSpec(input.SpecJSON)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	if input.ID == "" {
		input.ID = newID("template")
	} else if input.Version <= 0 {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板版本无效，请刷新后重试"}
	}
	timestamp := now()
	item := model.ProductionTemplate{ID: input.ID, OrganizationID: organization.ID, Name: input.Name, Description: input.Description, Source: input.Source, MediaType: input.MediaType, TemplateType: input.TemplateType, Platform: input.Platform, Status: input.Status, DraftPrompt: input.Prompt, DraftSpecJSON: specJSON, CreatedBy: user.ID}
	result, err := repository.SaveProductionTemplate(item, input.Version, timestamp, newAuditLog(user.ID, organization.ID, "template.save", "production_template", item.ID, map[string]any{"name": item.Name}, timestamp))
	if errors.Is(err, repository.ErrProductionTemplateNameConflict) {
		return model.ProductionTemplate{}, safeMessageError{message: "企业内生产模板名称不能重复"}
	}
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板已被其他成员更新，请刷新后重试"}
	}
	return result, err
}

func PublishProductionTemplate(user model.AuthUser, id string, expectedVersion int64) (model.ProductionTemplate, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.ProductionTemplate{}, safeMessageError{message: "没有生产模板发布权限"}
	}
	id = strings.TrimSpace(id)
	if id == "" || expectedVersion <= 0 {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板版本无效，请刷新后重试"}
	}
	item, ok, err := repository.GetProductionTemplate(organization.ID, id)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	if !ok {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板不存在"}
	}
	if item.Version != expectedVersion {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板已被其他成员更新，请刷新后重试"}
	}
	if strings.TrimSpace(item.DraftPrompt) == "" {
		return model.ProductionTemplate{}, safeMessageError{message: "模板草稿提示词不能为空"}
	}
	if err := validateProductionTemplateVariables(item.DraftPrompt); err != nil {
		return model.ProductionTemplate{}, err
	}
	if _, _, err := normalizeProductionTemplateSpec(item.DraftSpecJSON); err != nil {
		return model.ProductionTemplate{}, err
	}
	timestamp := now()
	result, _, err := repository.PublishProductionTemplate(organization.ID, id, expectedVersion, newID("template-version"), user.ID, timestamp, newAuditLog(user.ID, organization.ID, "template.publish", "production_template", id, map[string]any{"version": item.CurrentVersion + 1}, timestamp))
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板已被其他成员更新，请刷新后重试"}
	}
	return result, err
}

func CopyProductionTemplate(user model.AuthUser, id string, input model.CopyProductionTemplateInput) (model.ProductionTemplate, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	if !canManageOrganization(membership.Role) {
		return model.ProductionTemplate{}, safeMessageError{message: "没有生产模板管理权限"}
	}
	id, name := strings.TrimSpace(id), strings.TrimSpace(input.Name)
	if id == "" {
		return model.ProductionTemplate{}, safeMessageError{message: "生产模板不存在"}
	}
	var sourceName, prompt, specJSON string
	templateType, platform := model.ProductionTemplateTypeCustom, "original"
	if definition, ok := findBuiltinProductionTemplate(id); ok {
		sourceName, prompt, specJSON, templateType, platform = definition.Name, definition.Prompt, definition.SpecJSON, definition.TemplateType, definition.Platform
	} else {
		source, ok, err := repository.GetProductionTemplate(organization.ID, id)
		if err != nil {
			return model.ProductionTemplate{}, err
		}
		if !ok {
			return model.ProductionTemplate{}, safeMessageError{message: "生产模板不存在"}
		}
		sourceName, prompt, specJSON, templateType, platform = source.Name, source.DraftPrompt, source.DraftSpecJSON, source.TemplateType, source.Platform
	}
	if name == "" {
		name = sourceName + " 副本"
	}
	if len([]rune(name)) > 120 {
		return model.ProductionTemplate{}, safeMessageError{message: "模板名称过长"}
	}
	if strings.TrimSpace(prompt) == "" {
		return model.ProductionTemplate{}, safeMessageError{message: "源模板草稿提示词为空，不能复制"}
	}
	if err := validateProductionTemplateVariables(prompt); err != nil {
		return model.ProductionTemplate{}, err
	}
	specJSON, _, err = normalizeProductionTemplateSpec(specJSON)
	if err != nil {
		return model.ProductionTemplate{}, err
	}
	timestamp := now()
	item := model.ProductionTemplate{ID: newID("template"), OrganizationID: organization.ID, Name: name, Description: "复制自「" + sourceName + "」", Source: model.ProductionTemplateSourceOrganization, MediaType: model.ProductionTemplateMediaTypeImage, TemplateType: templateType, Platform: platform, Status: model.ProductionTemplateStatusDraft, DraftPrompt: prompt, DraftSpecJSON: specJSON, CreatedBy: user.ID}
	result, err := repository.SaveProductionTemplate(item, 0, timestamp, newAuditLog(user.ID, organization.ID, "template.copy", "production_template", item.ID, map[string]any{"name": name, "sourceId": id}, timestamp))
	if errors.Is(err, repository.ErrProductionTemplateNameConflict) {
		return model.ProductionTemplate{}, safeMessageError{message: "企业内生产模板名称不能重复"}
	}
	return result, err
}

func PreviewProductionPrompt(user model.AuthUser, input model.PreviewProductionPromptInput) (model.ProductionPromptPreview, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.ProductionPromptPreview{}, err
	}
	input.PresetID, input.DeliverySpecID, input.BrandID, input.ProductID, input.SKUID = strings.TrimSpace(input.PresetID), strings.TrimSpace(input.DeliverySpecID), strings.TrimSpace(input.BrandID), strings.TrimSpace(input.ProductID), strings.TrimSpace(input.SKUID)
	presetPrompt, presetVersion, err := resolveProductionPreset(organization.ID, input.PresetID, input.PresetVersion)
	if err != nil {
		return model.ProductionPromptPreview{}, err
	}
	deliverySpec, err := resolveProductionDeliverySpec(input.DeliverySpecID)
	if err != nil {
		return model.ProductionPromptPreview{}, err
	}
	product, ok, err := repository.GetProduct(organization.ID, input.ProductID)
	if err != nil {
		return model.ProductionPromptPreview{}, err
	}
	if !ok {
		return model.ProductionPromptPreview{}, safeMessageError{message: "预览商品不存在"}
	}
	var brand *model.Brand
	if input.BrandID != "" {
		value, ok, err := repository.GetBrand(organization.ID, input.BrandID)
		if err != nil {
			return model.ProductionPromptPreview{}, err
		}
		if !ok {
			return model.ProductionPromptPreview{}, safeMessageError{message: "预览品牌不存在"}
		}
		brand = &value
	}
	var sku *model.ProductSKU
	if input.SKUID != "" {
		value, ok, err := repository.GetProductSKU(organization.ID, input.SKUID)
		if err != nil {
			return model.ProductionPromptPreview{}, err
		}
		if !ok || value.ProductID != product.ID {
			return model.ProductionPromptPreview{}, safeMessageError{message: "预览 SKU 不属于所选商品"}
		}
		sku = &value
	}
	prompt, err := batchProductionPrompt(BatchProductionExecution{Job: model.BatchProductionJob{PresetID: input.PresetID, PresetVersion: presetVersion, PresetPrompt: presetPrompt, DeliverySpec: deliverySpec}, Brand: brand, Product: product, SKU: sku})
	return model.ProductionPromptPreview{Prompt: prompt}, err
}

func ListBatchProductionJobs(user model.AuthUser, q model.Query) (model.BatchProductionJobList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.BatchProductionJobList{}, err
	}
	items, total, err := repository.ListBatchProductionJobs(organization.ID, q)
	return model.BatchProductionJobList{Items: items, Total: int(total)}, err
}

func legacyCreateBatchProductionJob(user model.AuthUser, input model.CreateBatchProductionJobInput) (model.BatchProductionJob, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.BatchProductionJob{}, err
	}
	if !canWriteCommerce(membership.Role) {
		return model.BatchProductionJob{}, safeMessageError{message: "没有批量生产权限"}
	}
	input.RequestID, input.Name, input.BrandID, input.PresetID, input.DeliverySpecID = strings.TrimSpace(input.RequestID), strings.TrimSpace(input.Name), strings.TrimSpace(input.BrandID), strings.TrimSpace(input.PresetID), strings.TrimSpace(input.DeliverySpecID)
	if input.DeliverySpecID == "" {
		input.DeliverySpecID = "original"
	}
	if input.RequestID == "" || len(input.RequestID) > 128 || input.Name == "" || len(input.Name) > 200 || input.PresetID == "" || len(input.ProductIDs) == 0 {
		return model.BatchProductionJob{}, safeMessageError{message: "任务请求编号、名称、生产模板和商品不能为空或过长"}
	}
	productIDs := make([]string, 0, len(input.ProductIDs))
	seenProductIDs := map[string]bool{}
	for _, productID := range input.ProductIDs {
		productID = strings.TrimSpace(productID)
		if productID != "" && !seenProductIDs[productID] {
			seenProductIDs[productID], productIDs = true, append(productIDs, productID)
		}
	}
	if len(productIDs) == 0 {
		return model.BatchProductionJob{}, safeMessageError{message: "请选择有效商品"}
	}
	if len(productIDs) > 200 {
		return model.BatchProductionJob{}, safeMessageError{message: "单个批量任务最多选择 200 个商品"}
	}
	sort.Strings(productIDs)
	input.ProductIDs = productIDs
	requestHash := batchProductionRequestHash(input)
	if existing, ok, err := repository.GetBatchProductionJobByRequest(organization.ID, input.RequestID); err != nil {
		return model.BatchProductionJob{}, err
	} else if ok {
		if existing.RequestHash != requestHash {
			return model.BatchProductionJob{}, safeMessageError{message: "任务请求编号已用于不同的请求内容"}
		}
		return existing, nil
	}
	presetPrompt, presetVersion, err := resolveProductionPreset(organization.ID, input.PresetID, input.PresetVersion)
	if err != nil {
		return model.BatchProductionJob{}, err
	}
	deliverySpec, err := resolveProductionDeliverySpec(input.DeliverySpecID)
	if err != nil {
		return model.BatchProductionJob{}, err
	}
	if input.BrandID != "" {
		if _, ok, err := repository.GetBrand(organization.ID, input.BrandID); err != nil {
			return model.BatchProductionJob{}, err
		} else if !ok {
			return model.BatchProductionJob{}, safeMessageError{message: "任务品牌不存在"}
		}
	}
	itemOperations := map[string]string{}
	for _, productID := range input.ProductIDs {
		if _, ok, err := repository.GetProduct(organization.ID, productID); err != nil || !ok {
			if err != nil {
				return model.BatchProductionJob{}, err
			}
			return model.BatchProductionJob{}, safeMessageError{message: "任务包含无效商品"}
		}
		totalSKUs := 0
		for page := 1; ; page++ {
			skus, total, err := repository.ListProductSKUs(organization.ID, productID, model.Query{Page: page, PageSize: model.MaxPageSize})
			if err != nil {
				return model.BatchProductionJob{}, err
			}
			totalSKUs += len(skus)
			for _, sku := range skus {
				operation := "generation"
				if len(sku.ImageStorageKeys) > 0 {
					operation = "edit"
				}
				itemOperations[productID+"\x00"+sku.ID] = operation
			}
			if totalSKUs >= int(total) {
				break
			}
		}
		if totalSKUs == 0 {
			itemOperations[productID+"\x00"] = "generation"
		}
	}
	settings, err := PublicSettings()
	if err != nil {
		return model.BatchProductionJob{}, err
	}
	modelName := strings.TrimSpace(settings.ModelChannel.DefaultImageModel)
	if modelName == "" {
		return model.BatchProductionJob{}, safeMessageError{message: "默认图片模型未配置，无法估算任务价格"}
	}
	creditsByOperation, itemEstimatedCredits := map[string]int{}, make(map[string]int, len(itemOperations))
	for key, operation := range itemOperations {
		credits, ok := creditsByOperation[operation]
		if !ok {
			credits, err = CalculateRequestCreditsForGroup(standardBatchPricingRequest(modelName, operation, deliverySpec), user.Group)
			if err != nil {
				return model.BatchProductionJob{}, err
			}
			creditsByOperation[operation] = credits
		}
		if credits <= 0 {
			return model.BatchProductionJob{}, safeMessageError{message: "默认图片模型未设置有效价格"}
		}
		itemEstimatedCredits[key] = credits
	}
	timestamp, jobID := now(), newID("batch")
	job := model.BatchProductionJob{ID: jobID, OrganizationID: organization.ID, RequestID: input.RequestID, RequestHash: requestHash, ArchiveToken: newID("archive"), BrandID: input.BrandID, Name: input.Name, PresetID: input.PresetID, PresetVersion: presetVersion, PresetPrompt: presetPrompt, DeliverySpec: deliverySpec, ProductIDs: input.ProductIDs, Status: model.BatchProductionStatusQueued, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.CreateBatchProductionJob(job, itemEstimatedCredits, newAuditLog(user.ID, organization.ID, "batch.create", "batch_job", job.ID, job.Name, timestamp))
	if errors.Is(err, repository.ErrBatchProductionItemsTooLarge) {
		return model.BatchProductionJob{}, safeMessageError{message: "单个批量任务最多生成 5000 个生产项"}
	}
	if errors.Is(err, repository.ErrBatchProductionSnapshotTooLarge) {
		return model.BatchProductionJob{}, safeMessageError{message: "批量任务输入快照总量不能超过 16MB"}
	}
	if errors.Is(err, repository.ErrBatchProductionRequestConflict) {
		return model.BatchProductionJob{}, safeMessageError{message: "任务请求编号已用于不同的请求内容"}
	}
	if errors.Is(err, repository.ErrBatchProductionOrganizationQueueFull) {
		return model.BatchProductionJob{}, safeMessageError{message: "企业待生产项目已达上限，请等待现有任务完成"}
	}
	if errors.Is(err, repository.ErrInsufficientUserCredits) {
		return result, safeMessageError{message: "个人算力余额不足"}
	}
	if errors.Is(err, repository.ErrInsufficientOrganizationCredits) {
		return result, safeMessageError{message: "企业共享算力余额不足"}
	}
	if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
		return result, safeMessageError{message: "企业本月算力预算不足"}
	}
	return result, err
}

func batchProductionRequestHash(input model.CreateBatchProductionJobInput) string {
	data, _ := json.Marshal(struct {
		Name, BrandID, PresetID, DeliverySpecID string
		PresetVersion                           int
		ProductIDs                              []string
	}{Name: input.Name, BrandID: input.BrandID, PresetID: input.PresetID, PresetVersion: input.PresetVersion, DeliverySpecID: input.DeliverySpecID, ProductIDs: input.ProductIDs})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ListBatchProductionItems(user model.AuthUser, jobID string, q model.Query) (model.BatchProductionItemList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.BatchProductionItemList{}, err
	}
	items, total, err := repository.ListBatchProductionItems(organization.ID, jobID, q)
	if err == nil {
		err = attachBatchProductionQualityContexts(organization.ID, jobID, items)
	}
	return model.BatchProductionItemList{Items: items, Total: int(total)}, err
}

func attachBatchProductionQualityContexts(organizationID string, jobID string, items []model.BatchProductionItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items)*3)
	for _, item := range items {
		if item.BrandSnapshotID != "" {
			ids = append(ids, item.BrandSnapshotID)
		}
		ids = append(ids, item.ProductSnapshotID)
		if item.SKUSnapshotID != "" {
			ids = append(ids, item.SKUSnapshotID)
		}
	}
	snapshots, err := repository.GetBatchProductionSnapshots(organizationID, ids)
	if err != nil {
		return err
	}
	decode := func(id string, kind string, resourceID string, value any) error {
		snapshot, ok := snapshots[id]
		if !ok || snapshot.JobID != jobID || snapshot.Kind != kind || (resourceID != "" && snapshot.ResourceID != resourceID) || json.Unmarshal([]byte(snapshot.Data), value) != nil {
			return errors.New("batch production quality snapshot is invalid")
		}
		return nil
	}
	for index := range items {
		item := &items[index]
		context := &model.BatchProductionQualityContext{}
		if err := decode(item.ProductSnapshotID, "product", item.ProductID, &context.Product); err != nil || context.Product.ID != item.ProductID || context.Product.OrganizationID != organizationID {
			return errors.New("batch production product snapshot is invalid")
		}
		if item.BrandSnapshotID != "" {
			context.Brand = &model.Brand{}
			if err := decode(item.BrandSnapshotID, "brand", "", context.Brand); err != nil || context.Brand.ID != snapshots[item.BrandSnapshotID].ResourceID || context.Brand.OrganizationID != organizationID {
				return errors.New("batch production brand snapshot is invalid")
			}
		}
		if item.SKUID != "" {
			context.SKU = &model.ProductSKU{}
			if err := decode(item.SKUSnapshotID, "sku", item.SKUID, context.SKU); err != nil || context.SKU.ID != item.SKUID || context.SKU.ProductID != item.ProductID || context.SKU.OrganizationID != organizationID {
				return errors.New("batch production SKU snapshot is invalid")
			}
		}
		item.QualityContext = context
	}
	return nil
}

func ReviewBatchProductionItem(user model.AuthUser, jobID string, itemID string, input model.ReviewBatchProductionItemInput) (model.BatchProductionItem, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.BatchProductionItem{}, err
	}
	if !canReviewCommerce(membership.Role) {
		return model.BatchProductionItem{}, safeMessageError{message: "没有批量结果审核权限"}
	}
	input.Comment = strings.TrimSpace(input.Comment)
	if input.RunNumber < 1 || (input.Status != model.BatchProductionReviewApproved && input.Status != model.BatchProductionReviewRejected) || len(input.Comment) > 1000 {
		return model.BatchProductionItem{}, safeMessageError{message: "审核状态、生产轮次或批注无效"}
	}
	if input.Status == model.BatchProductionReviewRejected && input.Comment == "" {
		return model.BatchProductionItem{}, safeMessageError{message: "驳回时请填写批注"}
	}
	timestamp := now()
	item, err := repository.ReviewBatchProductionItem(organization.ID, strings.TrimSpace(jobID), strings.TrimSpace(itemID), input.RunNumber, input.Status, input.Comment, user.ID, timestamp, newAuditLog(user.ID, organization.ID, "batch.review", "batch_item", itemID, map[string]any{"status": input.Status, "runNumber": input.RunNumber}, timestamp))
	if errors.Is(err, repository.ErrBatchProductionStateConflict) {
		return model.BatchProductionItem{}, safeMessageError{message: "生产结果已变化，请刷新后重试"}
	}
	return item, err
}

func SetBatchProductionPrimary(user model.AuthUser, jobID string, itemID string, input model.BatchProductionItemRunInput) (model.BatchProductionItem, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.BatchProductionItem{}, err
	}
	if !canReviewCommerce(membership.Role) {
		return model.BatchProductionItem{}, safeMessageError{message: "没有主图选择权限"}
	}
	if input.RunNumber < 1 {
		return model.BatchProductionItem{}, safeMessageError{message: "生产轮次无效"}
	}
	timestamp := now()
	item, err := repository.SetBatchProductionPrimary(organization.ID, strings.TrimSpace(jobID), strings.TrimSpace(itemID), input.RunNumber, timestamp, newAuditLog(user.ID, organization.ID, "batch.primary", "batch_item", itemID, map[string]any{"runNumber": input.RunNumber}, timestamp))
	if errors.Is(err, repository.ErrBatchProductionStateConflict) {
		return model.BatchProductionItem{}, safeMessageError{message: "只有已审核通过的有效结果可以设为主图"}
	}
	return item, err
}

func RetryBatchProductionItem(user model.AuthUser, jobID string, itemID string, input model.BatchProductionItemRunInput) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) && !canReviewCommerce(membership.Role) {
		return safeMessageError{message: "没有重新生成权限"}
	}
	if input.RunNumber < 1 {
		return safeMessageError{message: "生产轮次无效"}
	}
	timestamp := now()
	err = repository.RetryBatchProductionItem(organization.ID, strings.TrimSpace(jobID), strings.TrimSpace(itemID), input.RunNumber, timestamp, newAuditLog(user.ID, organization.ID, "batch.item_retry", "batch_item", itemID, map[string]any{"runNumber": input.RunNumber}, timestamp))
	if errors.Is(err, repository.ErrBatchProductionOrganizationQueueFull) {
		return safeMessageError{message: "企业待生产项目已达上限，请等待现有任务完成"}
	}
	if errors.Is(err, repository.ErrInsufficientUserCredits) {
		return safeMessageError{message: "个人算力余额不足"}
	}
	if errors.Is(err, repository.ErrInsufficientOrganizationCredits) {
		return safeMessageError{message: "企业共享算力余额不足"}
	}
	if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
		return safeMessageError{message: "企业本月算力预算不足"}
	}
	if errors.Is(err, repository.ErrBatchProductionStateConflict) {
		return safeMessageError{message: "只有失败或已驳回的结果可以重新生成"}
	}
	return err
}

func CancelBatchProductionJob(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有批量生产权限"}
	}
	timestamp := now()
	if err := repository.CancelBatchProductionJob(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "batch.cancel", "batch_job", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBatchProductionStateConflict) {
			return safeMessageError{message: "任务状态已变化，请刷新后重试"}
		}
		return err
	}
	return nil
}

func RetryBatchProductionJob(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有批量生产权限"}
	}
	timestamp := now()
	if err := repository.RetryBatchProductionJob(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "batch.retry", "batch_job", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBatchProductionOrganizationQueueFull) {
			return safeMessageError{message: "企业待生产项目已达上限，请等待现有任务完成"}
		}
		if errors.Is(err, repository.ErrInsufficientUserCredits) {
			return safeMessageError{message: "个人算力余额不足"}
		}
		if errors.Is(err, repository.ErrInsufficientOrganizationCredits) {
			return safeMessageError{message: "企业共享算力余额不足"}
		}
		if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
			return safeMessageError{message: "企业本月算力预算不足"}
		}
		if errors.Is(err, repository.ErrBatchProductionStateConflict) || errors.Is(err, gorm.ErrRecordNotFound) {
			return safeMessageError{message: "只有失败或部分成功任务可以重试"}
		}
		return err
	}
	return nil
}

// ClaimBatchProductionItem and FinishBatchProductionItem are the durable queue boundary
// used by a separately deployed production worker. HTTP handlers do not execute long jobs.
func ClaimBatchProductionItem(maxTenantRunning int) (model.BatchProductionItem, model.BatchProductionJob, bool, error) {
	claimedAt := time.Now().UTC()
	return repository.ClaimNextBatchProductionItem(claimedAt.Format(timestampLayout), claimedAt.Add(5*time.Minute).Format(timestampLayout), newID("lease"), maxTenantRunning)
}

func FinishBatchProductionItem(item model.BatchProductionItem, succeeded bool, resultStorageKey string, errorMessage string) error {
	status := model.BatchProductionStatusCompleted
	if !succeeded {
		status = model.BatchProductionStatusFailed
	}
	if succeeded && !userStorageKeyPattern.MatchString(resultStorageKey) {
		status, resultStorageKey, errorMessage = model.BatchProductionStatusFailed, "", "批量结果存储编号无效"
	}
	return repository.FinishBatchProductionItem(item, status, strings.TrimSpace(resultStorageKey), batchProductionErrorMessage(errorMessage), now())
}

func RenewBatchProductionItemLease(item model.BatchProductionItem) error {
	return repository.RenewBatchProductionItemLease(item, time.Now().UTC().Add(5*time.Minute).Format(timestampLayout), now())
}

func ListOrganizationAuditLogs(user model.AuthUser, q model.Query) ([]model.OrganizationAuditLog, int, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return nil, 0, err
	}
	if !canManageOrganization(membership.Role) {
		return nil, 0, safeMessageError{message: "没有审计日志查看权限"}
	}
	items, total, err := repository.ListOrganizationAuditLogs(organization.ID, q)
	return items, int(total), err
}

func canManageOrganization(role model.OrganizationRole) bool {
	return role == model.OrganizationRoleOwner || role == model.OrganizationRoleAdmin
}
func canWriteCommerce(role model.OrganizationRole) bool {
	return canManageOrganization(role) || role == model.OrganizationRoleMember
}
func canReviewCommerce(role model.OrganizationRole) bool {
	return canManageOrganization(role) || role == model.OrganizationRoleReviewer
}
func validAssignableRole(role model.OrganizationRole) bool {
	return role == model.OrganizationRoleAdmin || role == model.OrganizationRoleMember || role == model.OrganizationRoleReviewer
}
func validProductStatus(status model.ProductStatus) bool {
	return status == model.ProductStatusDraft || status == model.ProductStatusActive || status == model.ProductStatusPaused
}
func validCommerceImageStorageKey(storageKey string) bool {
	return strings.HasPrefix(storageKey, "image:") && userStorageKeyPattern.MatchString(storageKey)
}

func validCommerceRecord(value any) bool {
	data, err := json.Marshal(value)
	return err == nil && len(data) <= 64<<10
}

func resolveProductionPreset(organizationID string, presetID string, presetVersion int) (string, int, error) {
	presetID = strings.TrimSpace(presetID)
	if definition, ok := findBuiltinProductionTemplate(presetID); ok {
		if presetVersion > 1 {
			return "", 0, safeMessageError{message: "生产模板版本无效"}
		}
		return definition.Prompt, 1, nil
	}
	item, version, ok, err := repository.GetProductionTemplateVersion(organizationID, presetID, presetVersion)
	if err != nil {
		return "", 0, err
	}
	if !ok || item.MediaType != model.ProductionTemplateMediaTypeImage || item.Status != model.ProductionTemplateStatusActive || item.CurrentVersion <= 0 {
		return "", 0, safeMessageError{message: "生产模板无效、未发布或已停用"}
	}
	return version.Prompt, version.Version, nil
}

func newAuditLog(userID string, organizationID string, action string, resourceType string, resourceID string, detail any, timestamp string) model.OrganizationAuditLog {
	value, _ := json.Marshal(detail)
	return model.OrganizationAuditLog{ID: newID("audit"), OrganizationID: organizationID, UserID: userID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Detail: string(value), CreatedAt: timestamp}
}
