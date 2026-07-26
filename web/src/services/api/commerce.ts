import { apiDelete, apiGet, apiPost, compactApiParams, organizationHeaders, type ApiParams } from "@/services/api/request";

export type OrganizationRole = "owner" | "admin" | "member" | "reviewer";
export type OrganizationCreditMode = "personal" | "shared";
export type OrganizationCreditSummary = { mode: OrganizationCreditMode; balance: number; personalBalance: number; monthlyBudget: number; monthlyUsed: number; alertThreshold: number; warning: boolean };
export type Organization = { id: string; name: string; slug: string; status: string; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type OrganizationSummary = Pick<Organization, "id" | "name" | "slug" | "status" | "createdAt"> & { role: OrganizationRole };
export type OrganizationMember = { id: string; userId: string; username: string; displayName: string; email: string; avatarUrl: string; role: OrganizationRole; version: number; createdAt: string };
export type OrganizationInvitation = { id: string; organizationId: string; organizationName: string; email: string; role: OrganizationRole; status: "pending" | "accepted" | "revoked" | "expired"; invitedBy: string; expiresAt: string; acceptedAt: string; createdAt: string; updatedAt: string };
export type OrganizationWorkspace = {
    organization: Organization;
    membership: { id: string; organizationId: string; userId: string; role: OrganizationRole; version: number; createdAt: string; updatedAt: string };
    organizations: OrganizationSummary[];
    stats: { brands: number; products: number; skus: number; batchJobs: number; pendingItems: number };
    creditSummary: OrganizationCreditSummary;
};

export type Brand = { id: string; organizationId: string; name: string; logoStorageKey: string; colors: string[]; fonts: string[]; tone: string; guidelines: string; prohibitedTerms: string[]; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductStatus = "draft" | "active" | "paused";
export type Product = { id: string; organizationId: string; brandId: string; brandName: string; code: string; name: string; category: string; description: string; sellingPoints: string[]; targetAudience: string; status: ProductStatus; skuCount: number; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductSKU = { id: string; organizationId: string; productId: string; code: string; name: string; attributes: Record<string, string>; imageStorageKeys: string[]; status: ProductStatus; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductionTemplate = { id: string; organizationId: string; name: string; description: string; source: string; mediaType: string; templateType: string; platform: string; status: "draft" | "active" | "disabled"; currentVersion: number; currentPrompt: string; currentSpec: string; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductionTemplateVersion = { id: string; templateId: string; version: number; prompt: string; specJson: string; createdBy: string; createdAt: string };
export type SaveProductionTemplateInput = { id?: string; name: string; description?: string; source: string; mediaType: string; templateType: string; platform: string; status: "draft" | "disabled"; prompt: string; specJson: string; version?: number };
export type ProductionDeliverySpec = { id: string; platform: string; name: string; width: number; height: number; format: string; quality: number; filenamePattern: string };
export type BatchProductionStatus = "queued" | "running" | "pending_review" | "partial_success" | "completed" | "failed" | "cancelled";
export type BatchProductionReviewStatus = "" | "pending" | "approved" | "rejected";
export type BatchProductionJob = { id: string; organizationId: string; requestId: string; brandId: string; name: string; kind: string; presetId: string; presetVersion: number; deliverySpec: ProductionDeliverySpec; productIds: string[]; status: BatchProductionStatus; totalItems: number; completedItems: number; failedItems: number; queuedItems: number; runningItems: number; estimatedCredits: number; reservedCredits: number; actualCredits: number; createdBy: string; createdAt: string; updatedAt: string };
export type BatchProductionQualityContext = { brand?: Brand; product: Product; sku?: ProductSKU };
export type BatchProductionErrorCategory = "" | "validation_input" | "pricing_credit" | "upstream_transient" | "upstream_permanent" | "storage_archive" | "cancelled_lease_lost" | "internal";
export type BatchProductionItem = { id: string; jobId: string; productId: string; skuId: string; templateSelectionId: string; templateId: string; templateVersion: number; templateType: string; variantIndex: number; estimatedCredits: number; errorCode: BatchProductionErrorCategory; retryable: boolean; nextAttemptAt: string; status: BatchProductionStatus; resultStorageKey: string; resultMimeType: string; resultSize: number; qualityContext?: BatchProductionQualityContext; errorMessage: string; reviewStatus: BatchProductionReviewStatus; reviewComment: string; reviewedBy: string; reviewedAt: string; isPrimary: boolean; runNumber: number; attempts: number; lockedAt: string; leaseExpiresAt: string; startedAt: string; finishedAt: string; createdAt: string; updatedAt: string };
export type AuditLog = { id: string; userId: string; action: string; resourceType: string; resourceId: string; detail: string; createdAt: string };
type ListResponse<T> = { items: T[]; total: number };

export const fetchCommerceWorkspace = () => apiGet<OrganizationWorkspace>("/api/commerce/workspace");
export const createOrganization = (name: string) => apiPost<Organization>("/api/commerce/organizations", { name });
export const updateOrganization = (name: string, version: number) => apiPost<Organization>("/api/commerce/organizations/current", { name, version });
export const updateOrganizationCreditSettings = (input: { mode: OrganizationCreditMode; monthlyBudget: number; alertThreshold: number; version: number }) => apiPost<OrganizationCreditSummary>("/api/commerce/organization-credit-settings", input);
export const transferOrganizationCredits = (amount: number) => apiPost<OrganizationCreditSummary>("/api/commerce/organization-credits/transfer", { amount });
export const switchOrganization = (organizationId: string) => apiPost<boolean>("/api/commerce/organizations/switch", { organizationId });
export const fetchPendingInvitations = () => apiGet<OrganizationInvitation[]>("/api/commerce/invitations");
export const fetchCurrentOrganizationInvitations = () => apiGet<OrganizationInvitation[]>("/api/commerce/organization-invitations");
export const fetchOrganizationMembers = (params?: ApiParams) => apiGet<ListResponse<OrganizationMember>>("/api/commerce/members", compactApiParams(params || {}));
export const inviteOrganizationMember = (email: string, role: OrganizationRole) => apiPost<OrganizationInvitation>("/api/commerce/invitations", { email, role });
export const acceptOrganizationInvitation = (id: string) => apiPost<boolean>(`/api/commerce/invitations/${id}/accept`);
export const revokeOrganizationInvitation = (id: string) => apiDelete<boolean>(`/api/commerce/invitations/${id}`);
export const updateOrganizationMember = (id: string, role: OrganizationRole, version: number) => apiPost<OrganizationMember>(`/api/commerce/members/${id}`, { role, version });
export const removeOrganizationMember = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/members/${id}?expectedVersion=${expectedVersion}`);
export const transferOrganizationOwnership = (id: string, expectedVersion: number) => apiPost<boolean>(`/api/commerce/members/${id}/transfer-owner?expectedVersion=${expectedVersion}`);

export const fetchBrands = (params?: ApiParams) => apiGet<ListResponse<Brand>>("/api/commerce/brands", compactApiParams(params || {}));
export const saveBrand = (item: Partial<Brand>) => apiPost<Brand>("/api/commerce/brands", item);
export const deleteBrand = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/brands/${id}?expectedVersion=${expectedVersion}`);
export const fetchProducts = (params?: ApiParams) => apiGet<ListResponse<Product>>("/api/commerce/products", compactApiParams(params || {}));
export const saveProduct = (item: Partial<Product>) => apiPost<Product>("/api/commerce/products", item);
export const deleteProduct = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/products/${id}?expectedVersion=${expectedVersion}`);
export const fetchProductSKUs = (productId: string, params?: ApiParams) => apiGet<ListResponse<ProductSKU>>(`/api/commerce/products/${productId}/skus`, compactApiParams(params || {}));
export const saveProductSKU = (item: Partial<ProductSKU>) => apiPost<ProductSKU>("/api/commerce/skus", item);
export const deleteProductSKU = (id: string, expectedVersion: number) => apiDelete<boolean>(`/api/commerce/skus/${id}?expectedVersion=${expectedVersion}`);
export const fetchProductionTemplates = (params?: ApiParams) => apiGet<ListResponse<ProductionTemplate>>("/api/commerce/production-templates", compactApiParams(params || {}));
export const saveProductionTemplate = (input: SaveProductionTemplateInput) => apiPost<ProductionTemplate>("/api/commerce/production-templates", input);
export const publishProductionTemplate = (id: string, expectedVersion: number) => apiPost<ProductionTemplate>(`/api/commerce/production-templates/${id}/publish`, { expectedVersion });
export const fetchProductionTemplateVersions = (id: string) => apiGet<ProductionTemplateVersion[]>(`/api/commerce/production-templates/${id}/versions`);
export const fetchProductionDeliverySpecs = () => apiGet<ProductionDeliverySpec[]>("/api/commerce/production-delivery-specs");
export const previewProductionPrompt = (input: { presetId: string; presetVersion: number; deliverySpecId: string; brandId?: string; productId: string; skuId?: string }) => apiPost<{ prompt: string }>("/api/commerce/production-templates/preview", input);
export const fetchBatchProductionJobs = (params?: ApiParams) => apiGet<ListResponse<BatchProductionJob>>("/api/commerce/batch-jobs", compactApiParams(params || {}));
export const fetchBatchProductionJob = (id: string) => apiGet<{ job: BatchProductionJob; templateSelections: Array<{ id: string; templateId: string; templateVersion: number; templateType: string; quantity: number; specJson: string; deliverySpec: ProductionDeliverySpec }>; progress: { total: number; queued: number; running: number; succeeded: number; failed: number } }>(`/api/commerce/batch-jobs/${id}`);
export const createBatchProductionJob = (input: { requestId: string; name: string; brandId?: string; presetId: string; presetVersion: number; deliverySpecId: string; productIds: string[] }) => apiPost<BatchProductionJob>("/api/commerce/batch-jobs", input);
export const fetchBatchProductionItems = (id: string, params?: ApiParams) => apiGet<ListResponse<BatchProductionItem>>(`/api/commerce/batch-jobs/${id}/items`, compactApiParams(params || {}));
export async function downloadBatchProductionArchive(id: string) {
    const response = await fetch(`/api/commerce/batch-jobs/${id}/archive`, { credentials: "include", headers: organizationHeaders() });
    const contentType = response.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
        const payload = await response.json() as { msg?: string };
        throw new Error(payload.msg || "下载批量结果失败");
    }
    if (!response.ok) throw new Error("下载批量结果失败");
    const disposition = response.headers.get("content-disposition") || "";
    const encodedName = disposition.match(/filename\*=utf-8''([^;]+)/i)?.[1];
    const quotedName = disposition.match(/filename="([^"]+)"/i)?.[1];
    const plainName = disposition.match(/filename=([^;\s]+)/i)?.[1];
    let filename = "batch-results.zip";
    try { filename = decodeURIComponent((encodedName || quotedName || plainName || filename).trim()); } catch { filename = "batch-results.zip"; }
    return { blob: await response.blob(), filename };
}
export const reviewBatchProductionItem = (jobId: string, itemId: string, input: { runNumber: number; status: "approved" | "rejected"; comment: string }) => apiPost<BatchProductionItem>(`/api/commerce/batch-jobs/${jobId}/items/${itemId}/review`, input);
export const retryBatchProductionItem = (jobId: string, itemId: string, runNumber: number) => apiPost<boolean>(`/api/commerce/batch-jobs/${jobId}/items/${itemId}/retry`, { runNumber });
export const setBatchProductionItemPrimary = (jobId: string, itemId: string, runNumber: number) => apiPost<BatchProductionItem>(`/api/commerce/batch-jobs/${jobId}/items/${itemId}/primary`, { runNumber });
export const cancelBatchProductionJob = (id: string) => apiPost<boolean>(`/api/commerce/batch-jobs/${id}/cancel`);
export const retryBatchProductionJob = (id: string) => apiPost<boolean>(`/api/commerce/batch-jobs/${id}/retry`);
export const fetchAuditLogs = (params?: ApiParams) => apiGet<ListResponse<AuditLog>>("/api/commerce/audit-logs", compactApiParams(params || {}));
