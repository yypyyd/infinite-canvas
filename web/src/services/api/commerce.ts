import { apiDelete, apiGet, apiPost, compactApiParams, type ApiParams } from "@/services/api/request";

export type OrganizationRole = "owner" | "admin" | "member" | "reviewer";
export type Organization = { id: string; name: string; slug: string; status: string; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type OrganizationSummary = Pick<Organization, "id" | "name" | "slug" | "status" | "createdAt"> & { role: OrganizationRole };
export type OrganizationMember = { id: string; userId: string; username: string; displayName: string; email: string; avatarUrl: string; role: OrganizationRole; version: number; createdAt: string };
export type OrganizationInvitation = { id: string; organizationId: string; organizationName: string; email: string; role: OrganizationRole; status: "pending" | "accepted" | "revoked" | "expired"; invitedBy: string; expiresAt: string; acceptedAt: string; createdAt: string; updatedAt: string };
export type OrganizationWorkspace = {
    organization: Organization;
    membership: { id: string; organizationId: string; userId: string; role: OrganizationRole; version: number; createdAt: string; updatedAt: string };
    organizations: OrganizationSummary[];
    stats: { brands: number; products: number; skus: number; batchJobs: number; pendingItems: number };
};

export type Brand = { id: string; organizationId: string; name: string; logoStorageKey: string; colors: string[]; fonts: string[]; tone: string; guidelines: string; prohibitedTerms: string[]; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductStatus = "draft" | "active" | "paused";
export type Product = { id: string; organizationId: string; brandId: string; brandName: string; code: string; name: string; category: string; description: string; sellingPoints: string[]; targetAudience: string; status: ProductStatus; skuCount: number; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type ProductSKU = { id: string; organizationId: string; productId: string; code: string; name: string; attributes: Record<string, string>; imageStorageKeys: string[]; status: ProductStatus; version: number; createdBy: string; createdAt: string; updatedAt: string };
export type BatchProductionStatus = "queued" | "running" | "completed" | "failed" | "cancelled";
export type BatchProductionJob = { id: string; organizationId: string; requestId: string; brandId: string; name: string; presetId: string; productIds: string[]; status: BatchProductionStatus; totalItems: number; completedItems: number; failedItems: number; createdBy: string; createdAt: string; updatedAt: string };
export type BatchProductionItem = { id: string; jobId: string; productId: string; skuId: string; status: BatchProductionStatus; resultStorageKey: string; errorMessage: string; runNumber: number; attempts: number; createdAt: string; updatedAt: string };
export type AuditLog = { id: string; userId: string; action: string; resourceType: string; resourceId: string; detail: string; createdAt: string };
type ListResponse<T> = { items: T[]; total: number };

export const fetchCommerceWorkspace = () => apiGet<OrganizationWorkspace>("/api/commerce/workspace");
export const createOrganization = (name: string) => apiPost<Organization>("/api/commerce/organizations", { name });
export const updateOrganization = (name: string, version: number) => apiPost<Organization>("/api/commerce/organizations/current", { name, version });
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
export const fetchBatchProductionJobs = (params?: ApiParams) => apiGet<ListResponse<BatchProductionJob>>("/api/commerce/batch-jobs", compactApiParams(params || {}));
export const createBatchProductionJob = (input: { requestId: string; name: string; brandId?: string; presetId: string; productIds: string[] }) => apiPost<BatchProductionJob>("/api/commerce/batch-jobs", input);
export const fetchBatchProductionItems = (id: string, params?: ApiParams) => apiGet<ListResponse<BatchProductionItem>>(`/api/commerce/batch-jobs/${id}/items`, compactApiParams(params || {}));
export const cancelBatchProductionJob = (id: string) => apiPost<boolean>(`/api/commerce/batch-jobs/${id}/cancel`);
export const retryBatchProductionJob = (id: string) => apiPost<boolean>(`/api/commerce/batch-jobs/${id}/retry`);
export const fetchAuditLogs = (params?: ApiParams) => apiGet<ListResponse<AuditLog>>("/api/commerce/audit-logs", compactApiParams(params || {}));
