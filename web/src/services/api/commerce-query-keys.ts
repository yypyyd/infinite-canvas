type CommerceListFilters = Readonly<Record<string, string | number | boolean | undefined>>;

export const commerceQueryKeys = {
    root: (organizationId: string) => ["commerce", organizationId] as const,
    workspace: (organizationId: string) => ["commerce", organizationId, "workspace"] as const,
    members: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "members", filters] as const,
    brands: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "brands", filters] as const,
    brandOptions: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "brand-options", filters] as const,
    organizationInvitations: (organizationId: string) => ["commerce", organizationId, "organization-invitations"] as const,
    audit: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "audit", filters] as const,
    products: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "products", filters] as const,
    productOptions: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "product-options", filters] as const,
    product: (organizationId: string, id: string) => ["commerce", organizationId, "product", id] as const,
    skus: (organizationId: string, productId: string, filters: CommerceListFilters) => ["commerce", organizationId, "skus", productId, filters] as const,
    previewSkus: (organizationId: string, productId: string, filters: CommerceListFilters) => ["commerce", organizationId, "preview-skus", productId, filters] as const,
    templates: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "templates", filters] as const,
    templateVersions: (organizationId: string, templateId: string) => ["commerce", organizationId, "template-versions", templateId] as const,
    deliverySpecs: (organizationId: string) => ["commerce", organizationId, "delivery-specs"] as const,
    preflight: (organizationId: string, input: Readonly<Record<string, unknown>>) => ["commerce", organizationId, "image-preflight", input] as const,
    jobsRoot: (organizationId: string) => ["commerce", organizationId, "jobs"] as const,
    jobs: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "jobs", filters] as const,
    job: (organizationId: string, id: string) => ["commerce", organizationId, "job", id] as const,
    jobItemsRoot: (organizationId: string, jobId: string) => ["commerce", organizationId, "job-items", jobId] as const,
    items: (organizationId: string, jobId: string, filters: CommerceListFilters) => ["commerce", organizationId, "job-items", jobId, filters] as const,
    videoProjects: (organizationId: string, filters: CommerceListFilters) => ["commerce", organizationId, "video-projects", filters] as const,
    videoProject: (organizationId: string, id: string) => ["commerce", organizationId, "video-project", id] as const,
};

export const userCommerceQueryKeys = {
    pendingInvitations: (userId: string) => ["user-commerce", userId, "pending-invitations"] as const,
};
