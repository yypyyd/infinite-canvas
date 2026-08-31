package repository

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func CreateExpandedBatchProductionJob(job model.BatchProductionJob, scopes []model.BatchProductionProductScope, selections []model.BatchProductionTemplateSelection, itemPricing map[string]model.BatchProductionItemPricing, auditLogs ...model.OrganizationAuditLog) (model.BatchProductionJob, error) {
	db, err := DB()
	if err != nil {
		return job, err
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		var organization model.Organization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&organization, "id = ?", job.OrganizationID).Error; err != nil {
			return err
		}
		var existing model.BatchProductionJob
		if err := tx.Where("organization_id = ? AND request_id = ?", job.OrganizationID, job.RequestID).First(&existing).Error; err == nil {
			if existing.RequestHash != job.RequestHash {
				return ErrBatchProductionRequestConflict
			}
			job = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		productIDs := make([]string, 0, len(scopes))
		scopeByProduct := map[string]model.BatchProductionProductScope{}
		for _, scope := range scopes {
			productIDs = append(productIDs, scope.ProductID)
			scopeByProduct[scope.ProductID] = scope
		}
		sort.Strings(productIDs)
		var products []model.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("organization_id = ? AND id IN ?", job.OrganizationID, productIDs).Find(&products).Error; err != nil {
			return err
		}
		if len(products) != len(productIDs) {
			return gorm.ErrRecordNotFound
		}
		var skus []model.ProductSKU
		if err := tx.Where("organization_id = ? AND product_id IN ?", job.OrganizationID, productIDs).Order("product_id asc, id asc").Find(&skus).Error; err != nil {
			return err
		}
		selectedSKUs := []model.ProductSKU{}
		for _, sku := range skus {
			scope := scopeByProduct[sku.ProductID]
			match := scope.AllActiveSKUs && sku.Status == model.ProductStatusActive
			for _, id := range scope.SKUIDs {
				if id == sku.ID {
					match = true
					break
				}
			}
			if match {
				selectedSKUs = append(selectedSKUs, sku)
			}
		}
		items := make([]model.BatchProductionItem, 0)
		for selectionIndex := range selections {
			selection := &selections[selectionIndex]
			selection.ID = fmt.Sprintf("%s-selection-%d", job.ID, selectionIndex+1)
			selection.OrganizationID, selection.JobID, selection.CreatedAt = job.OrganizationID, job.ID, job.CreatedAt
		}
		for _, sku := range selectedSKUs {
			for selectionIndex := range selections {
				selection := &selections[selectionIndex]
				pricing, ok := itemPricing[fmt.Sprintf("%s\x00%d", sku.ID, selection.SelectionIndex)]
				if !ok || !validBatchProductionItemPricing(job, pricing) {
					return errors.New("batch production item estimate is missing")
				}
				for variant := 1; variant <= selection.Quantity; variant++ {
					items = append(items, model.BatchProductionItem{ID: fmt.Sprintf("%s-item-%d", job.ID, len(items)+1), OrganizationID: job.OrganizationID, JobID: job.ID, ProductID: sku.ProductID, SKUID: sku.ID, TemplateSelectionID: selection.ID, TemplateID: selection.TemplateID, TemplateVersion: selection.TemplateVersion, TemplateType: selection.TemplateType, VariantIndex: variant, Operation: pricing.Operation, ResolutionTier: pricing.ResolutionTier, EstimatedCredits: pricing.EstimatedCredits, PricingSnapshot: pricing.PricingSnapshot, Status: model.BatchProductionStatusQueued, RunNumber: 1, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt})
				}
			}
		}
		if len(items) == 0 {
			return gorm.ErrRecordNotFound
		}
		if len(items) > maxBatchProductionItems {
			return ErrBatchProductionItemsTooLarge
		}
		var pending int64
		if err := tx.Model(&model.BatchProductionItem{}).Where("organization_id = ? AND status IN ?", job.OrganizationID, []model.BatchProductionStatus{model.BatchProductionStatusQueued, model.BatchProductionStatusRunning}).Count(&pending).Error; err != nil {
			return err
		}
		if pending+int64(len(items)) > maxBatchProductionPendingItemsPerOrganization {
			return ErrBatchProductionOrganizationQueueFull
		}
		job.TotalItems, job.QueuedItems = len(items), len(items)
		job.EstimatedCredits = 0
		for _, item := range items {
			job.EstimatedCredits += item.EstimatedCredits
		}
		if err := reserveBatchProductionCredits(tx, &organization, &job); err != nil {
			return err
		}
		snapshots := []model.BatchProductionSnapshot{}
		snapshotSize := 0
		appendSnapshot := func(kind, resourceID string, value any) (string, error) {
			data, err := json.Marshal(value)
			if err != nil {
				return "", err
			}
			snapshotSize += len(data)
			if snapshotSize > 16<<20 {
				return "", ErrBatchProductionSnapshotTooLarge
			}
			id := job.ID + ":" + kind + ":" + resourceID
			snapshots = append(snapshots, model.BatchProductionSnapshot{ID: id, OrganizationID: job.OrganizationID, JobID: job.ID, Kind: kind, ResourceID: resourceID, Data: string(data), Size: len(data), CreatedAt: job.CreatedAt})
			return id, nil
		}
		productSnapshots := map[string]string{}
		for _, product := range products {
			id, err := appendSnapshot("product", product.ID, product)
			if err != nil {
				return err
			}
			productSnapshots[product.ID] = id
		}
		skuSnapshots := map[string]string{}
		for _, sku := range selectedSKUs {
			id, err := appendSnapshot("sku", sku.ID, sku)
			if err != nil {
				return err
			}
			skuSnapshots[sku.ID] = id
		}
		for index := range selections {
			snapshotID, err := appendSnapshot("template", selections[index].ID, selections[index])
			if err != nil {
				return err
			}
			selections[index].TemplateSnapshotID = snapshotID
		}
		_, err := appendSnapshot("request", job.RequestID, map[string]any{"productScopes": scopes, "templateSelections": selections})
		if err != nil {
			return err
		}
		for index := range items {
			items[index].ProductSnapshotID, items[index].SKUSnapshotID = productSnapshots[items[index].ProductID], skuSnapshots[items[index].SKUID]
		}
		inputStorageKeys := []string{}
		var brand *model.Brand
		if job.BrandID != "" {
			var saved model.Brand
			if err := tx.Where("organization_id = ? AND id = ?", job.OrganizationID, job.BrandID).First(&saved).Error; err != nil {
				return err
			}
			brand = &saved
			brandID, err := appendSnapshot("brand", saved.ID, saved)
			if err != nil {
				return err
			}
			if saved.LogoStorageKey != "" {
				inputStorageKeys = append(inputStorageKeys, saved.LogoStorageKey)
			}
			for index := range items {
				items[index].BrandSnapshotID = brandID
			}
		}
		selectedSKUByID := make(map[string]model.ProductSKU, len(selectedSKUs))
		for _, sku := range selectedSKUs {
			selectedSKUByID[sku.ID] = sku
		}
		for _, item := range items {
			sku := selectedSKUByID[item.SKUID]
			if item.Operation != model.BatchProductionImageOperation(batchProductionHasReferences(brand, &sku)) {
				return errors.New("batch production input references changed during creation")
			}
		}
		if err := tx.Create(&job).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&selections, 100).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&snapshots, 200).Error; err != nil {
			return err
		}
		if err := tx.CreateInBatches(&items, 200).Error; err != nil {
			return err
		}
		for _, sku := range selectedSKUs {
			inputStorageKeys = append(inputStorageKeys, sku.ImageStorageKeys...)
		}
		if err := replaceUserFileReferences(tx, job.OrganizationID, "batch_input", job.ID, "batch-input-"+job.ID, inputStorageKeys, false, job.CreatedAt); err != nil {
			return err
		}
		return saveOrganizationAuditLogs(tx, auditLogs)
	})
	return job, err
}

func reserveBatchProductionCredits(tx *gorm.DB, organization *model.Organization, job *model.BatchProductionJob) error {
	job.ReservedCredits, job.SettledCredits, job.ActualCredits = job.EstimatedCredits, 0, 0
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

func batchCreditBudgetMonth(timestamp string) string {
	if len(timestamp) >= 7 {
		return timestamp[:7]
	}
	return time.Now().UTC().Format("2006-01")
}

func GetBatchProductionJob(organizationID, id string) (model.BatchProductionJob, []model.BatchProductionTemplateSelection, bool, error) {
	db, err := DB()
	if err != nil {
		return model.BatchProductionJob{}, nil, false, err
	}
	var job model.BatchProductionJob
	if err := db.Where("organization_id = ? AND id = ?", organizationID, id).First(&job).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return job, nil, false, nil
	} else if err != nil {
		return job, nil, false, err
	}
	var selections []model.BatchProductionTemplateSelection
	err = db.Where("organization_id = ? AND job_id = ?", organizationID, id).Order("created_at asc, id asc").Find(&selections).Error
	return job, selections, err == nil, err
}

func GetBatchProductionTemplateSelection(organizationID, jobID, id string) (model.BatchProductionTemplateSelection, bool, error) {
	db, err := DB()
	if err != nil {
		return model.BatchProductionTemplateSelection{}, false, err
	}
	var selection model.BatchProductionTemplateSelection
	err = db.Where("organization_id = ? AND job_id = ? AND id = ?", organizationID, jobID, id).First(&selection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return selection, false, nil
	}
	return selection, err == nil, err
}
