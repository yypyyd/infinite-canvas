package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

type resolvedImageProduction struct {
	Preflight model.ProductionPreflight
	Products map[string]model.Product
	SKUs map[string][]model.ProductSKU
	Selections []model.BatchProductionTemplateSelection
	ItemEstimatedCredits map[string]int
}

func PreflightImageProduction(user model.AuthUser, input model.CreateBatchProductionJobInput) (model.ProductionPreflight, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil { return model.ProductionPreflight{}, err }
	resolved, err := resolveImageProduction(organization.ID, user.Group, input)
	return resolved.Preflight, err
}

func resolveImageProduction(organizationID string, userGroup string, input model.CreateBatchProductionJobInput) (resolvedImageProduction, error) {
	input.Name, input.BrandID, input.PreviewSKUID = strings.TrimSpace(input.Name), strings.TrimSpace(input.BrandID), strings.TrimSpace(input.PreviewSKUID)
	issues := []model.ProductionPreflightIssue{}
	if len(input.ProductScopes) == 0 && len(input.ProductIDs) > 0 { for _, id := range input.ProductIDs { input.ProductScopes = append(input.ProductScopes, model.BatchProductionProductScope{ProductID: id, AllActiveSKUs: true}) } }
	if len(input.TemplateSelections) == 0 && input.PresetID != "" { input.TemplateSelections = []model.BatchProductionTemplateSelectionInput{{TemplateID: input.PresetID, TemplateVersion: input.PresetVersion, Quantity: 1, DeliverySpecID: input.DeliverySpecID}} }
	if len(input.ProductScopes) == 0 || len(input.ProductScopes) > 200 { return resolvedImageProduction{}, safeMessageError{message: "请选择 1 到 200 个商品"} }
	if len(input.TemplateSelections) == 0 { return resolvedImageProduction{}, safeMessageError{message: "请至少选择一个图片模板"} }
	scopeByProduct := map[string]model.BatchProductionProductScope{}
	for _, scope := range input.ProductScopes {
		scope.ProductID = strings.TrimSpace(scope.ProductID)
		if scope.ProductID == "" { continue }
		if existing, ok := scopeByProduct[scope.ProductID]; ok {
			existing.SKUIDs = append(existing.SKUIDs, scope.SKUIDs...)
			existing.AllActiveSKUs = existing.AllActiveSKUs || scope.AllActiveSKUs
			scope = existing
		}
		scopeByProduct[scope.ProductID] = scope
	}
	productIDs := make([]string, 0, len(scopeByProduct))
	for productID, scope := range scopeByProduct {
		seen := map[string]bool{}
		ids := make([]string, 0, len(scope.SKUIDs))
		for _, id := range scope.SKUIDs {
			id = strings.TrimSpace(id)
			if id != "" && !seen[id] { seen[id] = true; ids = append(ids, id) }
		}
		sort.Strings(ids)
		scope.SKUIDs = ids
		scopeByProduct[productID] = scope
		productIDs = append(productIDs, productID)
	}
	sort.Strings(productIDs)
	input.ProductScopes = input.ProductScopes[:0]; input.ProductIDs = append([]string(nil), productIDs...)
	products := map[string]model.Product{}; skusByProduct := map[string][]model.ProductSKU{}
	for _, productID := range productIDs {
		product, ok, err := repository.GetProduct(organizationID, productID); if err != nil { return resolvedImageProduction{}, err }; if !ok { return resolvedImageProduction{}, safeMessageError{message: "任务包含不属于当前企业的商品"} }; products[productID] = product
		scope := scopeByProduct[productID]; input.ProductScopes = append(input.ProductScopes, scope)
		items, _, err := repository.ListProductSKUs(organizationID, productID, model.Query{Page: 1, PageSize: model.MaxPageSize}); if err != nil { return resolvedImageProduction{}, err }
		requested := map[string]bool{}; for _, id := range scope.SKUIDs { requested[id] = true }
		for _, sku := range items { if (scope.AllActiveSKUs && sku.Status == model.ProductStatusActive) || requested[sku.ID] { skusByProduct[productID] = append(skusByProduct[productID], sku); delete(requested, sku.ID) } }
		if len(requested) > 0 { return resolvedImageProduction{}, safeMessageError{message: "任务包含不属于所选商品的 SKU"} }
		if len(skusByProduct[productID]) == 0 { issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SKU_REQUIRED", ProductID: productID, Field: "skuIds", Message: "商品没有符合条件的 SKU"}) }
	}
	type resolvedSelection struct {
		input     model.BatchProductionTemplateSelectionInput
		selection model.BatchProductionTemplateSelection
	}
	selectionKey := map[string]bool{}
	resolvedSelections := make([]resolvedSelection, 0, len(input.TemplateSelections))
	for _, selected := range input.TemplateSelections {
		selected.TemplateID, selected.DeliverySpecID = strings.TrimSpace(selected.TemplateID), strings.TrimSpace(selected.DeliverySpecID); if selected.Quantity == 0 { selected.Quantity = 1 }; if selected.DeliverySpecID == "" { selected.DeliverySpecID = "original" }
		if selected.Quantity < 1 || selected.Quantity > 10 { return resolvedImageProduction{}, safeMessageError{message: "每个模板的结果数量必须为 1 到 10"} }
		prompt, version, err := resolveProductionPreset(organizationID, selected.TemplateID, selected.TemplateVersion); if err != nil { return resolvedImageProduction{}, err }
		selected.TemplateVersion = version
		templateType, specJSON := model.ProductionTemplateType(""), ""
		if definition, builtin := findBuiltinProductionTemplate(selected.TemplateID); builtin {
			templateType, specJSON = definition.TemplateType, definition.SpecJSON
		} else {
			template, templateVersion, ok, err := repository.GetProductionTemplateVersion(organizationID, selected.TemplateID, version)
			if err != nil { return resolvedImageProduction{}, err }
			if !ok || template.MediaType != model.ProductionTemplateMediaTypeImage || template.Status != model.ProductionTemplateStatusActive || template.CurrentVersion <= 0 { return resolvedImageProduction{}, safeMessageError{message: "所选图片模板不存在、未发布或已停用"} }
			templateType, specJSON = template.TemplateType, templateVersion.SpecJSON
		}
		if templateType == "" { templateType = model.ProductionTemplateTypeCustom }
		_, spec, err := normalizeProductionTemplateSpec(specJSON); if err != nil { return resolvedImageProduction{}, err }
		delivery, err := resolveProductionDeliverySpec(selected.DeliverySpecID); if err != nil { return resolvedImageProduction{}, err }
		selected.DeliverySpecID = delivery.ID
		key := fmt.Sprintf("%s\x00%d\x00%s", selected.TemplateID, version, selected.DeliverySpecID)
		if selectionKey[key] { return resolvedImageProduction{}, safeMessageError{message: "同一模板版本和交付规格不能重复选择"} }
		selectionKey[key] = true
		resolvedSelections = append(resolvedSelections, resolvedSelection{input: selected, selection: model.BatchProductionTemplateSelection{TemplateID: selected.TemplateID, TemplateVersion: version, TemplateType: templateType, Quantity: selected.Quantity, Prompt: prompt, SpecJSON: specJSON, DeliverySpec: delivery}})
		for productID, skuItems := range skusByProduct { for _, sku := range skuItems { if spec.RequireReference && len(sku.ImageStorageKeys) == 0 { issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "REFERENCE_REQUIRED", ProductID: productID, SKUID: sku.ID, TemplateID: selected.TemplateID, Field: "imageStorageKeys", Message: "该 SKU 缺少模板要求的参考图"}) }; if spec.RequireSellingPoints && len(products[productID].SellingPoints) == 0 { issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SELLING_POINTS_REQUIRED", ProductID: productID, SKUID: sku.ID, TemplateID: selected.TemplateID, Field: "sellingPoints", Message: "商品缺少模板要求的卖点"}) } } }
	}
	sort.Slice(resolvedSelections, func(left, right int) bool {
		a, b := resolvedSelections[left].input, resolvedSelections[right].input
		if a.TemplateID != b.TemplateID { return a.TemplateID < b.TemplateID }
		if a.TemplateVersion != b.TemplateVersion { return a.TemplateVersion < b.TemplateVersion }
		if a.DeliverySpecID != b.DeliverySpecID { return a.DeliverySpecID < b.DeliverySpecID }
		return a.Quantity < b.Quantity
	})
	selections := make([]model.BatchProductionTemplateSelection, 0, len(resolvedSelections))
	normalizedSelections := make([]model.BatchProductionTemplateSelectionInput, 0, len(resolvedSelections))
	for index, resolved := range resolvedSelections {
		resolved.selection.SelectionIndex = index + 1
		resolved.selection.ID = fmt.Sprintf("selection-%d-%s-v%d-%s", index+1, resolved.input.TemplateID, resolved.input.TemplateVersion, resolved.input.DeliverySpecID)
		selections = append(selections, resolved.selection)
		normalizedSelections = append(normalizedSelections, resolved.input)
	}
	input.TemplateSelections = normalizedSelections
	totalSKUs := 0; for _, items := range skusByProduct { totalSKUs += len(items) }; totalItems := 0; for _, selection := range selections { totalItems += totalSKUs * selection.Quantity }
	if totalItems > 5000 { return resolvedImageProduction{}, safeMessageError{message: "任务展开后超过 5000 个生产项，请拆分任务"} }
	settings, err := PublicSettings()
	if err != nil { return resolvedImageProduction{}, err }
	modelName := strings.TrimSpace(settings.ModelChannel.DefaultImageModel)
	if modelName == "" { return resolvedImageProduction{}, safeMessageError{message: "默认图片模型未配置，无法估算任务价格"} }
	itemEstimatedCredits := make(map[string]int, totalSKUs*len(selections))
	estimatedCredits := 0
	for _, skuItems := range skusByProduct {
		for _, sku := range skuItems {
			operation := "generation"
			if len(sku.ImageStorageKeys) > 0 { operation = "edit" }
			for _, selection := range selections {
				credits, err := CalculateRequestCreditsForGroup(standardBatchPricingRequest(modelName, operation, selection.DeliverySpec), userGroup)
				if err != nil { return resolvedImageProduction{}, err }
				itemEstimatedCredits[imageProductionEstimateKey(sku.ID, selection.SelectionIndex)] = credits
				estimatedCredits += credits * selection.Quantity
			}
		}
	}
	previews := []model.ProductionPreflightPreview{}
	for productID, skuItems := range skusByProduct { for _, sku := range skuItems { if input.PreviewSKUID != "" && input.PreviewSKUID != sku.ID { continue }; for _, selection := range selections { job := model.BatchProductionJob{PresetPrompt: selection.Prompt, DeliverySpec: selection.DeliverySpec}; prompt, _ := batchProductionPrompt(BatchProductionExecution{Job: job, Product: products[productID], SKU: &sku}); previews = append(previews, model.ProductionPreflightPreview{SKUID: sku.ID, TemplateID: selection.TemplateID, TemplateVersion: selection.TemplateVersion, Prompt: prompt, ReferenceStorageKeys: sku.ImageStorageKeys, DeliverySpec: selection.DeliverySpec}) }; if len(previews) > 0 { break } }; if len(previews) > 0 { break } }
	result := model.ProductionPreflight{NormalizedInput: input, SKUCount: totalSKUs, TemplateCount: len(selections), TotalItems: totalItems, EstimatedCredits: estimatedCredits, CanSubmit: len(issues) == 0 && totalItems > 0, Issues: issues, Previews: previews}
	return resolvedImageProduction{Preflight: result, Products: products, SKUs: skusByProduct, Selections: selections, ItemEstimatedCredits: itemEstimatedCredits}, nil
}

func imageProductionEstimateKey(skuID string, selectionIndex int) string {
	return fmt.Sprintf("%s\x00%d", skuID, selectionIndex)
}
