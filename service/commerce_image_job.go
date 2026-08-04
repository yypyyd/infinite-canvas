package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func CreateBatchProductionJob(user model.AuthUser, input model.CreateBatchProductionJobInput) (model.BatchProductionJob, error) {
	if len(input.ProductScopes) == 0 && len(input.TemplateSelections) == 0 { return legacyCreateBatchProductionJob(user, input) }
	organization, membership, err := EnsureOrganization(user)
	if err != nil { return model.BatchProductionJob{}, err }
	if !canWriteCommerce(membership.Role) { return model.BatchProductionJob{}, safeMessageError{message: "没有批量生产权限"} }
	input.RequestID, input.Name = strings.TrimSpace(input.RequestID), strings.TrimSpace(input.Name)
	if input.RequestID == "" || len(input.RequestID) > 128 || input.Name == "" || len(input.Name) > 200 { return model.BatchProductionJob{}, safeMessageError{message: "任务请求编号或名称不能为空或过长"} }
	resolved, err := resolveImageProduction(organization.ID, user.Group, input)
	if err != nil { return model.BatchProductionJob{}, err }
	if !resolved.Preflight.CanSubmit { return model.BatchProductionJob{}, safeMessageError{message: "图片生产预检未通过，请修复商品、SKU 或模板问题"} }
	normalized := resolved.Preflight.NormalizedInput
	normalized.RequestID, normalized.Name = input.RequestID, input.Name
	hashPayload, err := json.Marshal(struct { Name string `json:"name"`; BrandID string `json:"brandId"`; Scopes []model.BatchProductionProductScope `json:"productScopes"`; Templates []model.BatchProductionTemplateSelectionInput `json:"templateSelections"` }{Name: normalized.Name, BrandID: normalized.BrandID, Scopes: normalized.ProductScopes, Templates: normalized.TemplateSelections})
	if err != nil { return model.BatchProductionJob{}, err }
	sum := sha256.Sum256(hashPayload); requestHash := hex.EncodeToString(sum[:])
	if existing, ok, err := repository.GetBatchProductionJobByRequest(organization.ID, input.RequestID); err != nil { return model.BatchProductionJob{}, err } else if ok { if existing.RequestHash != requestHash { return model.BatchProductionJob{}, safeMessageError{message: "任务请求编号已用于不同的请求内容"} }; return existing, nil }
	timestamp, id := now(), newID("batch")
	job := model.BatchProductionJob{ID: id, OrganizationID: organization.ID, RequestID: input.RequestID, RequestHash: requestHash, ArchiveToken: newID("archive"), BrandID: normalized.BrandID, Name: input.Name, Kind: "image_pack", ProductIDs: normalized.ProductIDs, Status: model.BatchProductionStatusQueued, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	result, err := repository.CreateExpandedBatchProductionJob(job, normalized.ProductScopes, resolved.Selections, resolved.ItemEstimatedCredits, newAuditLog(user.ID, organization.ID, "batch.create", "batch_job", id, map[string]any{"name": input.Name, "items": resolved.Preflight.TotalItems}, timestamp))
	if errors.Is(err, repository.ErrBatchProductionItemsTooLarge) { return result, safeMessageError{message: "单个批量任务最多生成 5000 个生产项"} }
	if errors.Is(err, repository.ErrBatchProductionSnapshotTooLarge) { return result, safeMessageError{message: "批量任务输入快照总量不能超过 16MB"} }
	if errors.Is(err, repository.ErrBatchProductionRequestConflict) { return result, safeMessageError{message: "任务请求编号已用于不同的请求内容"} }
	if errors.Is(err, repository.ErrBatchProductionOrganizationQueueFull) { return result, safeMessageError{message: "企业待生产项目已达上限，请等待现有任务完成"} }
	if errors.Is(err, repository.ErrInsufficientUserCredits) { return result, safeMessageError{message: "个人算力余额不足"} }
	if errors.Is(err, repository.ErrInsufficientOrganizationCredits) { return result, safeMessageError{message: "企业共享算力余额不足"} }
	if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) { return result, safeMessageError{message: "企业本月算力预算不足"} }
	return result, err
}

func GetBatchProductionJob(user model.AuthUser, id string) (map[string]any, error) {
	organization, _, err := EnsureOrganization(user); if err != nil { return nil, err }
	job, selections, ok, err := repository.GetBatchProductionJob(organization.ID, strings.TrimSpace(id)); if err != nil { return nil, err }; if !ok { return nil, safeMessageError{message: "批量任务不存在"} }
	return map[string]any{"job": job, "templateSelections": selections, "progress": map[string]int{"total": job.TotalItems, "queued": job.QueuedItems, "running": job.RunningItems, "succeeded": job.CompletedItems, "failed": job.FailedItems}}, nil
}
