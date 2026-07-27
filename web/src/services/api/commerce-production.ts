import { apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";
import type { BatchProductionItem, BatchProductionJob, ProductionDeliverySpec } from "@/services/api/commerce";

export type ProductScope = { productId: string; skuIds: string[]; allActiveSkus: boolean };
export type TemplateSelectionInput = { templateId: string; templateVersion: number; quantity: number; deliverySpecId: string };
export type ImageProductionInput = { requestId: string; name: string; brandId?: string; productScopes: ProductScope[]; templateSelections: TemplateSelectionInput[]; previewSkuId?: string };
export type ProductionIssue = { severity: "error" | "warning"; code: string; productId?: string; skuId?: string; templateId?: string; field?: string; message: string };
export type ProductionPreview = { skuId: string; templateId: string; templateVersion: number; prompt: string; referenceStorageKeys: string[]; deliverySpec: ProductionDeliverySpec };
export type ImageProductionPreflight = { normalizedInput: ImageProductionInput; skuCount: number; templateCount: number; totalItems: number; estimatedCredits: number; canSubmit: boolean; issues: ProductionIssue[]; previews: ProductionPreview[] };
export type TemplateSelection = { id: string; templateId: string; templateVersion: number; templateType: string; quantity: number; specJson: string; deliverySpec: ProductionDeliverySpec };
export type BatchJobDetail = { job: BatchProductionJob; templateSelections: TemplateSelection[]; progress: { total: number; queued: number; running: number; succeeded: number; failed: number } };

export const preflightImageProduction = (input: Omit<ImageProductionInput, "requestId" | "name">) => apiPost<ImageProductionPreflight>("/api/commerce/production/image/preflight", input);
export const createImageProductionJob = (input: ImageProductionInput) => apiPost<BatchProductionJob>("/api/commerce/batch-jobs", input);
export const fetchBatchJobDetail = (id: string) => apiGet<BatchJobDetail>(`/api/commerce/batch-jobs/${id}`);
export const fetchBatchJobItems = (id: string, params?: ApiParams) => apiGet<{ items: BatchProductionItem[]; total: number }>(`/api/commerce/batch-jobs/${id}/items`, compactApiParams(params || {}));
