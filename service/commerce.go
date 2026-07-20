package service

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
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
	if organizationID == "" { return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "缺少企业上下文"} }
	membership, ok, err := repository.GetOrganizationMember(organizationID, user.ID)
	if err != nil { return model.Organization{}, model.OrganizationMember{}, err }
	if !ok { return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "你不是该企业成员"} }
	organization, ok, err := repository.GetOrganization(organizationID)
	if err != nil { return model.Organization{}, model.OrganizationMember{}, err }
	if !ok || organization.Status != "active" { return model.Organization{}, model.OrganizationMember{}, safeMessageError{message: "企业不存在或已停用"} }
	return organization, membership, nil
}

func RequireOrganizationWrite(user model.AuthUser) error {
	_, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "当前企业角色没有生成权限"} }
	return nil
}

func CreateOrganization(user model.AuthUser, name string) (model.Organization, error) {
	name = strings.TrimSpace(name)
	if name == "" { return model.Organization{}, safeMessageError{message: "请输入企业名称"} }
	timestamp := now()
	organization := model.Organization{ID: newID("org"), Name: name, Slug: newID("workspace"), Status: "active", CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	membership := model.OrganizationMember{ID: newID("member"), OrganizationID: organization.ID, UserID: user.ID, Role: model.OrganizationRoleOwner, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.CreateOrganization(organization, membership, true, newAuditLog(user.ID, organization.ID, "organization.create", "organization", organization.ID, name, timestamp))
	return result, err
}

func OrganizationWorkspace(user model.AuthUser) (model.OrganizationWorkspace, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return model.OrganizationWorkspace{}, err }
	organizations, err := repository.ListUserOrganizations(user.ID)
	if err != nil { return model.OrganizationWorkspace{}, err }
	stats, err := repository.OrganizationStats(organization.ID)
	return model.OrganizationWorkspace{Organization: organization, Membership: membership, Organizations: organizations, Stats: stats}, err
}

func ListOrganizationMembers(user model.AuthUser, q model.Query) (model.OrganizationMemberList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.OrganizationMemberList{}, err }
	items, total, err := repository.ListOrganizationMembers(organization.ID, q)
	return model.OrganizationMemberList{Items: items, Total: int(total)}, err
}

func ListCurrentOrganizationInvitations(user model.AuthUser) ([]model.OrganizationInvitation, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return nil, err }
	if !canManageOrganization(membership.Role) { return []model.OrganizationInvitation{}, nil }
	return repository.ListOrganizationInvitations(organization.ID, now())
}

func SwitchOrganization(user model.AuthUser, organizationID string) error {
	if _, _, err := ResolveOrganizationAccess(user, organizationID); err != nil { return err }
	if err := repository.SwitchUserOrganization(user.ID, strings.TrimSpace(organizationID), newAuditLog(user.ID, organizationID, "organization.switch", "organization", organizationID, "", now())); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) { return safeMessageError{message: "你不是该企业成员"} }
		return err
	}
	return nil
}

func UpdateOrganization(user model.AuthUser, name string) (model.Organization, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return organization, err }
	if !canManageOrganization(membership.Role) { return organization, safeMessageError{message: "没有企业管理权限"} }
	name = strings.TrimSpace(name)
	if name == "" { return organization, safeMessageError{message: "请输入企业名称"} }
	organization.Name = name
	organization.UpdatedAt = now()
	result, err := repository.SaveOrganization(organization, newAuditLog(user.ID, organization.ID, "organization.update", "organization", organization.ID, name, organization.UpdatedAt))
	return result, err
}

func InviteOrganizationMember(user model.AuthUser, email string, role model.OrganizationRole) (model.OrganizationInvitation, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return model.OrganizationInvitation{}, err }
	if !canManageOrganization(membership.Role) { return model.OrganizationInvitation{}, safeMessageError{message: "没有成员管理权限"} }
	email, err = normalizeEmailAddress(email)
	if err != nil { return model.OrganizationInvitation{}, safeMessageError{message: "请输入有效邮箱"} }
	if !validAssignableRole(role) { return model.OrganizationInvitation{}, safeMessageError{message: "成员角色无效"} }
	if invitations, listErr := repository.ListOrganizationInvitations(organization.ID, now()); listErr != nil { return model.OrganizationInvitation{}, listErr } else {
		for _, item := range invitations { if item.Status == model.OrganizationInvitationPending && strings.EqualFold(item.Email, email) { return model.OrganizationInvitation{}, safeMessageError{message: "该邮箱已有待接受邀请"} } }
	}
	timestamp := now()
	invitation := model.OrganizationInvitation{ID: newID("invite"), OrganizationID: organization.ID, Email: email, Role: role, Status: model.OrganizationInvitationPending, InvitedBy: user.ID, ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(timestampLayout), CreatedAt: timestamp, UpdatedAt: timestamp}
	outbox := model.OrganizationEmailOutbox{ID: newID("email"), OrganizationID: organization.ID, UserID: user.ID, InvitationID: invitation.ID, Receiver: email, OrganizationName: organization.Name, Role: role, Status: "pending", NextAttemptAt: timestamp, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.SaveOrganizationInvitation(invitation, outbox, newAuditLog(user.ID, organization.ID, "member.invite", "invitation", invitation.ID, email, timestamp))
	if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) { return model.OrganizationInvitation{}, safeMessageError{message: "该邮箱已有待接受邀请"} }
	if err != nil { return model.OrganizationInvitation{}, err }
	return result, err
}

func ListPendingOrganizationInvitations(user model.AuthUser) ([]model.OrganizationInvitation, error) {
	if strings.TrimSpace(user.Email) == "" { return []model.OrganizationInvitation{}, nil }
	return repository.ListUserInvitations(user.Email, now())
}

func RevokeOrganizationInvitation(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canManageOrganization(membership.Role) { return safeMessageError{message: "没有成员管理权限"} }
	timestamp := now()
	if err := repository.RevokeOrganizationInvitation(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "member.invitation.revoke", "invitation", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) { return safeMessageError{message: "邀请不存在或状态已变化"} }
		return err
	}
	return nil
}

func AcceptOrganizationInvitation(user model.AuthUser, id string) error {
	invitation, ok, err := repository.GetOrganizationInvitation(id)
	if err != nil { return err }
	if !ok || invitation.Status != model.OrganizationInvitationPending || !strings.EqualFold(invitation.Email, user.Email) { return safeMessageError{message: "邀请不存在或不属于当前账号"} }
	expiresAt, err := time.Parse(time.RFC3339, invitation.ExpiresAt)
	if err != nil || time.Now().After(expiresAt) { return safeMessageError{message: "邀请已过期"} }
	if _, exists, err := repository.GetOrganizationMember(invitation.OrganizationID, user.ID); err != nil { return err } else if exists { return safeMessageError{message: "你已经是该企业成员"} }
	timestamp := now()
	membership := model.OrganizationMember{ID: newID("member"), OrganizationID: invitation.OrganizationID, UserID: user.ID, Role: invitation.Role, CreatedAt: timestamp, UpdatedAt: timestamp}
	if err := repository.AcceptOrganizationInvitation(invitation, membership, timestamp, newAuditLog(user.ID, invitation.OrganizationID, "member.accept", "invitation", invitation.ID, user.Email, timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationInvitationUnavailable) { return safeMessageError{message: "邀请已过期、已撤销或已被接受"} }
		if errors.Is(err, gorm.ErrRecordNotFound) { return safeMessageError{message: "受邀企业不存在或已停用"} }
		return err
	}
	return nil
}

func UpdateOrganizationMember(user model.AuthUser, memberID string, role model.OrganizationRole) (model.OrganizationMember, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return model.OrganizationMember{}, err }
	if !canManageOrganization(membership.Role) { return model.OrganizationMember{}, safeMessageError{message: "没有成员管理权限"} }
	if !validAssignableRole(role) { return model.OrganizationMember{}, safeMessageError{message: "成员角色无效"} }
	target, ok, err := repository.GetOrganizationMemberByID(organization.ID, memberID)
	if err != nil { return model.OrganizationMember{}, err }
	if !ok || target.Role == model.OrganizationRoleOwner { return model.OrganizationMember{}, safeMessageError{message: "不能修改企业所有者"} }
	item := model.OrganizationMember{ID: target.ID, OrganizationID: organization.ID, UserID: target.UserID, Role: role, CreatedAt: target.CreatedAt, UpdatedAt: now()}
	result, err := repository.SaveOrganizationMember(item, newAuditLog(user.ID, organization.ID, "member.update", "member", item.ID, string(role), item.UpdatedAt))
	if errors.Is(err, repository.ErrOrganizationOwnershipChanged) { return model.OrganizationMember{}, safeMessageError{message: "成员角色已变化，请刷新后重试"} }
	return result, err
}

func RemoveOrganizationMember(user model.AuthUser, memberID string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canManageOrganization(membership.Role) { return safeMessageError{message: "没有成员管理权限"} }
	if memberID == membership.ID { return safeMessageError{message: "不能从当前企业移除自己"} }
	timestamp := now()
	if err := repository.DeleteOrganizationMember(organization.ID, memberID, newAuditLog(user.ID, organization.ID, "member.remove", "member", memberID, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationOwnershipChanged) { return safeMessageError{message: "成员角色已变化，不能移除企业所有者"} }
		return err
	}
	return nil
}

func TransferOrganizationOwnership(user model.AuthUser, memberID string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if membership.Role != model.OrganizationRoleOwner { return safeMessageError{message: "只有企业所有者可以转移所有权"} }
	if memberID == membership.ID { return safeMessageError{message: "你已经是企业所有者"} }
	timestamp := now()
	if err := repository.TransferOrganizationOwnership(organization.ID, membership.ID, memberID, timestamp, newAuditLog(user.ID, organization.ID, "organization.transfer_owner", "member", memberID, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrOrganizationOwnershipChanged) { return safeMessageError{message: "企业所有权已变化，请刷新后重试"} }
		return err
	}
	return nil
}

func ListBrands(user model.AuthUser, q model.Query) (model.BrandList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.BrandList{}, err }
	items, total, err := repository.ListBrands(organization.ID, q)
	return model.BrandList{Items: items, Total: int(total)}, err
}

func SaveBrand(user model.AuthUser, item model.Brand) (model.Brand, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return item, err }
	if !canWriteCommerce(membership.Role) { return item, safeMessageError{message: "没有品牌编辑权限"} }
	item.Name, item.LogoURL = strings.TrimSpace(item.Name), strings.TrimSpace(item.LogoURL)
	if item.Name == "" || len(item.Name) > 200 { return item, safeMessageError{message: "品牌名称不能为空或过长"} }
	if item.LogoURL != "" && !validHTTPURL(item.LogoURL) { return item, safeMessageError{message: "品牌 Logo 必须是有效的 HTTP/HTTPS 地址"} }
	if !validCommerceRecord(item) { return item, safeMessageError{message: "品牌规范内容不能超过 64KB"} }
	create := item.ID == ""
	timestamp := now()
	if item.ID != "" { if saved, ok, err := repository.GetBrand(organization.ID, item.ID); err != nil { return item, err } else if !ok { return item, safeMessageError{message: "品牌不存在"} } else { item.CreatedAt, item.CreatedBy = saved.CreatedAt, saved.CreatedBy } }
	if item.ID == "" { item.ID, item.CreatedAt, item.CreatedBy = newID("brand"), timestamp, user.ID }
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	action := "brand.update"
	if create { action = "brand.create" }
	result, err := repository.SaveBrand(item, create, newAuditLog(user.ID, organization.ID, action, "brand", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrBrandNameConflict) { return item, safeMessageError{message: "企业内品牌名称不能重复"} }
	return result, err
}

func DeleteBrand(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "没有品牌编辑权限"} }
	timestamp := now()
	if err := repository.DeleteBrand(organization.ID, id, newAuditLog(user.ID, organization.ID, "brand.delete", "brand", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBrandInUse) { return safeMessageError{message: "品牌已被商品或生产任务引用，不能删除"} }
		return err
	}
	return nil
}

func ListProducts(user model.AuthUser, q model.Query) (model.ProductList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.ProductList{}, err }
	items, total, err := repository.ListProducts(organization.ID, q)
	return model.ProductList{Items: items, Total: int(total)}, err
}

func SaveProduct(user model.AuthUser, item model.Product) (model.Product, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return item, err }
	if !canWriteCommerce(membership.Role) { return item, safeMessageError{message: "没有商品编辑权限"} }
	item.Name, item.Code = strings.TrimSpace(item.Name), strings.TrimSpace(item.Code)
	if item.Name == "" || item.Code == "" || len(item.Name) > 200 || len(item.Code) > 120 { return item, safeMessageError{message: "商品名称或 SPU 编码为空或过长"} }
	create := item.ID == ""
	if item.BrandID != "" { if _, ok, err := repository.GetBrand(organization.ID, item.BrandID); err != nil { return item, err } else if !ok { return item, safeMessageError{message: "所属品牌不存在"} } }
	if item.Status == "" { item.Status = model.ProductStatusDraft }
	if !validProductStatus(item.Status) { return item, safeMessageError{message: "商品状态无效"} }
	if !validCommerceRecord(item) { return item, safeMessageError{message: "商品内容不能超过 64KB"} }
	timestamp := now()
	if item.ID != "" { if saved, ok, err := repository.GetProduct(organization.ID, item.ID); err != nil { return item, err } else if !ok { return item, safeMessageError{message: "商品不存在"} } else { item.CreatedAt, item.CreatedBy = saved.CreatedAt, saved.CreatedBy } }
	if item.ID == "" { item.ID, item.CreatedAt, item.CreatedBy = newID("product"), timestamp, user.ID }
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	action := "product.update"
	if create { action = "product.create" }
	result, err := repository.SaveProduct(item, create, newAuditLog(user.ID, organization.ID, action, "product", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrProductCodeConflict) { return item, safeMessageError{message: "企业内 SPU 编码不能重复"} }
	return result, err
}

func DeleteProduct(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "没有商品编辑权限"} }
	timestamp := now()
	if err := repository.DeleteProduct(organization.ID, id, newAuditLog(user.ID, organization.ID, "product.delete", "product", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrProductInUse) { return safeMessageError{message: "商品已被生产任务引用，不能删除"} }
		return err
	}
	return nil
}

func ListProductSKUs(user model.AuthUser, productID string, q model.Query) (model.ProductSKUList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.ProductSKUList{}, err }
	if _, ok, err := repository.GetProduct(organization.ID, productID); err != nil || !ok { if err != nil { return model.ProductSKUList{}, err }; return model.ProductSKUList{}, safeMessageError{message: "商品不存在"} }
	items, total, err := repository.ListProductSKUs(organization.ID, productID, q)
	return model.ProductSKUList{Items: items, Total: int(total)}, err
}

func SaveProductSKU(user model.AuthUser, item model.ProductSKU) (model.ProductSKU, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return item, err }
	if !canWriteCommerce(membership.Role) { return item, safeMessageError{message: "没有 SKU 编辑权限"} }
	if _, ok, err := repository.GetProduct(organization.ID, item.ProductID); err != nil || !ok { if err != nil { return item, err }; return item, safeMessageError{message: "商品不存在"} }
	item.Code, item.Name = strings.TrimSpace(item.Code), strings.TrimSpace(item.Name)
	if item.Code == "" || item.Name == "" || len(item.Code) > 120 || len(item.Name) > 200 { return item, safeMessageError{message: "SKU 编码或名称为空或过长"} }
	if len(item.ImageURLs) > 50 { return item, safeMessageError{message: "单个 SKU 最多保存 50 张参考图"} }
	for index, imageURL := range item.ImageURLs {
		item.ImageURLs[index] = strings.TrimSpace(imageURL)
		if !validHTTPURL(item.ImageURLs[index]) { return item, safeMessageError{message: "SKU 参考图必须是有效的 HTTP/HTTPS 地址"} }
	}
	create := item.ID == ""
	if item.Status == "" { item.Status = model.ProductStatusActive }
	if !validProductStatus(item.Status) { return item, safeMessageError{message: "SKU 状态无效"} }
	if !validCommerceRecord(item) { return item, safeMessageError{message: "SKU 内容不能超过 64KB"} }
	timestamp := now()
	if item.ID != "" { if saved, ok, err := repository.GetProductSKU(organization.ID, item.ID); err != nil { return item, err } else if !ok { return item, safeMessageError{message: "SKU 不存在"} } else if saved.ProductID != item.ProductID { return item, safeMessageError{message: "SKU 创建后不能变更所属商品"} } else { item.CreatedAt, item.CreatedBy = saved.CreatedAt, saved.CreatedBy } }
	if item.ID == "" { item.ID, item.CreatedAt, item.CreatedBy = newID("sku"), timestamp, user.ID }
	item.OrganizationID, item.UpdatedAt = organization.ID, timestamp
	action := "sku.update"
	if create { action = "sku.create" }
	result, err := repository.SaveProductSKU(item, create, newAuditLog(user.ID, organization.ID, action, "sku", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrProductSKUCodeConflict) { return item, safeMessageError{message: "企业内 SKU 编码不能重复"} }
	return result, err
}

func DeleteProductSKU(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "没有 SKU 编辑权限"} }
	timestamp := now()
	if err := repository.DeleteProductSKU(organization.ID, id, newAuditLog(user.ID, organization.ID, "sku.delete", "sku", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrProductSKUInUse) { return safeMessageError{message: "SKU 已被生产任务引用，不能删除"} }
		return err
	}
	return nil
}

func ListBatchProductionJobs(user model.AuthUser, q model.Query) (model.BatchProductionJobList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.BatchProductionJobList{}, err }
	items, total, err := repository.ListBatchProductionJobs(organization.ID, q)
	return model.BatchProductionJobList{Items: items, Total: int(total)}, err
}

func CreateBatchProductionJob(user model.AuthUser, input model.CreateBatchProductionJobInput) (model.BatchProductionJob, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return model.BatchProductionJob{}, err }
	if !canWriteCommerce(membership.Role) { return model.BatchProductionJob{}, safeMessageError{message: "没有批量生产权限"} }
	input.Name, input.PresetID = strings.TrimSpace(input.Name), strings.TrimSpace(input.PresetID)
	if input.Name == "" || len(input.Name) > 200 || input.PresetID == "" || len(input.ProductIDs) == 0 { return model.BatchProductionJob{}, safeMessageError{message: "任务名称、生产模板和商品不能为空或过长"} }
	if !commerceProductionPresets[input.PresetID] { return model.BatchProductionJob{}, safeMessageError{message: "生产模板无效"} }
	productIDs := make([]string, 0, len(input.ProductIDs))
	seenProductIDs := map[string]bool{}
	for _, productID := range input.ProductIDs {
		productID = strings.TrimSpace(productID)
		if productID != "" && !seenProductIDs[productID] { seenProductIDs[productID], productIDs = true, append(productIDs, productID) }
	}
	if len(productIDs) == 0 { return model.BatchProductionJob{}, safeMessageError{message: "请选择有效商品"} }
	if len(productIDs) > 200 { return model.BatchProductionJob{}, safeMessageError{message: "单个批量任务最多选择 200 个商品"} }
	input.ProductIDs = productIDs
	if input.BrandID != "" { if _, ok, err := repository.GetBrand(organization.ID, input.BrandID); err != nil { return model.BatchProductionJob{}, err } else if !ok { return model.BatchProductionJob{}, safeMessageError{message: "任务品牌不存在"} } }
	for _, productID := range input.ProductIDs { if _, ok, err := repository.GetProduct(organization.ID, productID); err != nil || !ok { if err != nil { return model.BatchProductionJob{}, err }; return model.BatchProductionJob{}, safeMessageError{message: "任务包含无效商品"} } }
	skus, err := repository.ListProductSKUsByProductIDs(organization.ID, input.ProductIDs)
	if err != nil { return model.BatchProductionJob{}, err }
	timestamp, jobID := now(), newID("batch")
	items := make([]model.BatchProductionItem, 0, len(skus)+len(input.ProductIDs))
	covered := map[string]bool{}
	for _, sku := range skus { covered[sku.ProductID] = true; items = append(items, model.BatchProductionItem{ID: newID("batch-item"), OrganizationID: organization.ID, JobID: jobID, ProductID: sku.ProductID, SKUID: sku.ID, Status: model.BatchProductionStatusQueued, RunNumber: 1, CreatedAt: timestamp, UpdatedAt: timestamp}) }
	for _, productID := range input.ProductIDs { if !covered[productID] { items = append(items, model.BatchProductionItem{ID: newID("batch-item"), OrganizationID: organization.ID, JobID: jobID, ProductID: productID, Status: model.BatchProductionStatusQueued, RunNumber: 1, CreatedAt: timestamp, UpdatedAt: timestamp}) } }
	if len(items) > 5000 { return model.BatchProductionJob{}, safeMessageError{message: "单个批量任务最多生成 5000 个生产项"} }
	job := model.BatchProductionJob{ID: jobID, OrganizationID: organization.ID, BrandID: input.BrandID, Name: input.Name, PresetID: input.PresetID, ProductIDs: input.ProductIDs, Status: model.BatchProductionStatusQueued, TotalItems: len(items), CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.CreateBatchProductionJob(job, items, newAuditLog(user.ID, organization.ID, "batch.create", "batch_job", job.ID, job.Name, timestamp))
	if errors.Is(err, repository.ErrBatchProductionSnapshotTooLarge) { return model.BatchProductionJob{}, safeMessageError{message: "批量任务输入快照总量不能超过 16MB"} }
	return result, err
}

func ListBatchProductionItems(user model.AuthUser, jobID string, q model.Query) (model.BatchProductionItemList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.BatchProductionItemList{}, err }
	items, total, err := repository.ListBatchProductionItems(organization.ID, jobID, q)
	return model.BatchProductionItemList{Items: items, Total: int(total)}, err
}

func CancelBatchProductionJob(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "没有批量生产权限"} }
	timestamp := now()
	if err := repository.CancelBatchProductionJob(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "batch.cancel", "batch_job", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBatchProductionStateConflict) { return safeMessageError{message: "任务状态已变化，请刷新后重试"} }
		return err
	}
	return nil
}

func RetryBatchProductionJob(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return err }
	if !canWriteCommerce(membership.Role) { return safeMessageError{message: "没有批量生产权限"} }
	timestamp := now()
	if err := repository.RetryBatchProductionJob(organization.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "batch.retry", "batch_job", id, "", timestamp)); err != nil {
		if errors.Is(err, repository.ErrBatchProductionStateConflict) || errors.Is(err, gorm.ErrRecordNotFound) { return safeMessageError{message: "只有失败任务可以重试"} }
		return err
	}
	return nil
}

// ClaimBatchProductionItem and FinishBatchProductionItem are the durable queue boundary
// used by a separately deployed production worker. HTTP handlers do not execute long jobs.
func ClaimBatchProductionItem() (model.BatchProductionItem, model.BatchProductionJob, bool, error) {
	claimedAt := time.Now().UTC()
	return repository.ClaimNextBatchProductionItem(claimedAt.Format(timestampLayout), claimedAt.Add(5*time.Minute).Format(timestampLayout), newID("lease"))
}

func FinishBatchProductionItem(item model.BatchProductionItem, succeeded bool, resultURL string, errorMessage string) error {
	status := model.BatchProductionStatusCompleted
	if !succeeded { status = model.BatchProductionStatusFailed }
	if succeeded && !validHTTPURL(resultURL) { status, resultURL, errorMessage = model.BatchProductionStatusFailed, "", "执行器返回的结果地址无效" }
	return repository.FinishBatchProductionItem(item, status, strings.TrimSpace(resultURL), batchProductionErrorMessage(errorMessage), now())
}

func RenewBatchProductionItemLease(item model.BatchProductionItem) error {
	return repository.RenewBatchProductionItemLease(item, time.Now().UTC().Add(5*time.Minute).Format(timestampLayout), now())
}

func ListOrganizationAuditLogs(user model.AuthUser, q model.Query) ([]model.OrganizationAuditLog, int, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return nil, 0, err }
	if !canManageOrganization(membership.Role) { return nil, 0, safeMessageError{message: "没有审计日志查看权限"} }
	items, total, err := repository.ListOrganizationAuditLogs(organization.ID, q)
	return items, int(total), err
}

func canManageOrganization(role model.OrganizationRole) bool { return role == model.OrganizationRoleOwner || role == model.OrganizationRoleAdmin }
func canWriteCommerce(role model.OrganizationRole) bool { return canManageOrganization(role) || role == model.OrganizationRoleMember }
func validAssignableRole(role model.OrganizationRole) bool { return role == model.OrganizationRoleAdmin || role == model.OrganizationRoleMember || role == model.OrganizationRoleReviewer }
func validProductStatus(status model.ProductStatus) bool { return status == model.ProductStatusDraft || status == model.ProductStatusActive || status == model.ProductStatusPaused }

func validCommerceRecord(value any) bool {
	data, err := json.Marshal(value)
	return err == nil && len(data) <= 64<<10
}

var commerceProductionPresets = map[string]bool{"product-main": true, "lifestyle": true, "selling-points": true, "promotion": true, "apparel-model": true, "sku-series": true}

func newAuditLog(userID string, organizationID string, action string, resourceType string, resourceID string, detail any, timestamp string) model.OrganizationAuditLog {
	value, _ := json.Marshal(detail)
	return model.OrganizationAuditLog{ID: newID("audit"), OrganizationID: organizationID, UserID: userID, Action: action, ResourceType: resourceType, ResourceID: resourceID, Detail: string(value), CreatedAt: timestamp}
}
