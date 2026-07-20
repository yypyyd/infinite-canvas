package model

type OrganizationRole string

const (
	OrganizationRoleOwner    OrganizationRole = "owner"
	OrganizationRoleAdmin    OrganizationRole = "admin"
	OrganizationRoleMember   OrganizationRole = "member"
	OrganizationRoleReviewer OrganizationRole = "reviewer"
)

type Organization struct {
	ID        string `json:"id" gorm:"primaryKey"`
	Name      string `json:"name"`
	Slug      string `json:"slug" gorm:"uniqueIndex"`
	Status    string `json:"status" gorm:"index"`
	CreatedBy string `json:"createdBy" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type OrganizationMember struct {
	ID             string           `json:"id" gorm:"primaryKey"`
	OrganizationID string           `json:"organizationId" gorm:"uniqueIndex:idx_organization_user;index"`
	UserID         string           `json:"userId" gorm:"uniqueIndex:idx_organization_user;index"`
	Role           OrganizationRole `json:"role" gorm:"index"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

type OrganizationMemberView struct {
	ID          string           `json:"id"`
	UserID      string           `json:"userId"`
	Username    string           `json:"username"`
	DisplayName string           `json:"displayName"`
	Email       string           `json:"email"`
	AvatarURL   string           `json:"avatarUrl"`
	Role        OrganizationRole `json:"role"`
	CreatedAt   string           `json:"createdAt"`
}

type OrganizationMemberList struct {
	Items []OrganizationMemberView `json:"items"`
	Total int                      `json:"total"`
}

type OrganizationSummary struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Slug      string           `json:"slug"`
	Status    string           `json:"status"`
	Role      OrganizationRole `json:"role"`
	CreatedAt string           `json:"createdAt"`
}

type OrganizationInvitationStatus string

const (
	OrganizationInvitationPending  OrganizationInvitationStatus = "pending"
	OrganizationInvitationAccepted OrganizationInvitationStatus = "accepted"
	OrganizationInvitationRevoked  OrganizationInvitationStatus = "revoked"
	OrganizationInvitationExpired  OrganizationInvitationStatus = "expired"
)

type OrganizationInvitation struct {
	ID             string                       `json:"id" gorm:"primaryKey"`
	OrganizationID string                       `json:"organizationId" gorm:"index"`
	Email          string                       `json:"email" gorm:"index"`
	Role           OrganizationRole             `json:"role"`
	Status         OrganizationInvitationStatus `json:"status" gorm:"index"`
	InvitedBy      string                       `json:"invitedBy" gorm:"index"`
	ExpiresAt      string                       `json:"expiresAt" gorm:"index"`
	AcceptedAt     string                       `json:"acceptedAt"`
	CreatedAt      string                       `json:"createdAt"`
	UpdatedAt      string                       `json:"updatedAt"`
	OrganizationName string                     `json:"organizationName" gorm:"-"`
}

type OrganizationAuditLog struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"index"`
	UserID         string `json:"userId" gorm:"index"`
	Action         string `json:"action" gorm:"index"`
	ResourceType   string `json:"resourceType" gorm:"index"`
	ResourceID     string `json:"resourceId" gorm:"index"`
	Detail         string `json:"detail" gorm:"type:text"`
	CreatedAt      string `json:"createdAt" gorm:"index"`
}

type OrganizationEmailOutbox struct {
	ID             string           `json:"id" gorm:"primaryKey"`
	OrganizationID string           `json:"organizationId" gorm:"index"`
	UserID         string           `json:"userId" gorm:"index"`
	InvitationID   string           `json:"invitationId" gorm:"uniqueIndex"`
	Receiver       string           `json:"receiver"`
	OrganizationName string         `json:"organizationName"`
	Role           OrganizationRole `json:"role"`
	Status         string           `json:"status" gorm:"index"`
	Attempts       int              `json:"attempts"`
	LeaseToken     string           `json:"-" gorm:"index"`
	LeaseExpiresAt string           `json:"leaseExpiresAt" gorm:"index"`
	NextAttemptAt  string           `json:"nextAttemptAt" gorm:"index"`
	LastError      string           `json:"lastError" gorm:"type:text"`
	SentAt         string           `json:"sentAt"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

type OrganizationWorkspace struct {
	Organization Organization               `json:"organization"`
	Membership   OrganizationMember         `json:"membership"`
	Organizations []OrganizationSummary     `json:"organizations"`
	Stats        OrganizationWorkspaceStats `json:"stats"`
}

type OrganizationWorkspaceStats struct {
	Brands       int64 `json:"brands"`
	Products     int64 `json:"products"`
	SKUs         int64 `json:"skus"`
	BatchJobs    int64 `json:"batchJobs"`
	PendingItems int64 `json:"pendingItems"`
}

type Brand struct {
	ID              string   `json:"id" gorm:"primaryKey"`
	OrganizationID  string   `json:"organizationId" gorm:"index;uniqueIndex:idx_organization_brand_name"`
	Name            string   `json:"name" gorm:"index;uniqueIndex:idx_organization_brand_name"`
	LogoURL         string   `json:"logoUrl"`
	Colors          []string `json:"colors" gorm:"serializer:json"`
	Fonts           []string `json:"fonts" gorm:"serializer:json"`
	Tone            string   `json:"tone" gorm:"type:text"`
	Guidelines      string   `json:"guidelines" gorm:"type:text"`
	ProhibitedTerms []string `json:"prohibitedTerms" gorm:"serializer:json"`
	CreatedBy       string   `json:"createdBy" gorm:"index"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type BrandList struct {
	Items []Brand `json:"items"`
	Total int     `json:"total"`
}

type ProductStatus string

const (
	ProductStatusDraft  ProductStatus = "draft"
	ProductStatusActive ProductStatus = "active"
	ProductStatusPaused ProductStatus = "paused"
)

type Product struct {
	ID             string        `json:"id" gorm:"primaryKey"`
	OrganizationID string        `json:"organizationId" gorm:"index;uniqueIndex:idx_organization_product_code"`
	BrandID        string        `json:"brandId" gorm:"index"`
	Code           string        `json:"code" gorm:"index;uniqueIndex:idx_organization_product_code"`
	Name           string        `json:"name" gorm:"index"`
	Category       string        `json:"category" gorm:"index"`
	Description    string        `json:"description" gorm:"type:text"`
	SellingPoints  []string      `json:"sellingPoints" gorm:"serializer:json"`
	TargetAudience string        `json:"targetAudience"`
	Status         ProductStatus `json:"status" gorm:"index"`
	CreatedBy      string        `json:"createdBy" gorm:"index"`
	CreatedAt      string        `json:"createdAt"`
	UpdatedAt      string        `json:"updatedAt"`
	BrandName      string        `json:"brandName" gorm:"-"`
	SKUCount       int64         `json:"skuCount" gorm:"-"`
}

type ProductList struct {
	Items []Product `json:"items"`
	Total int       `json:"total"`
}

type ProductSKU struct {
	ID             string            `json:"id" gorm:"primaryKey"`
	OrganizationID string            `json:"organizationId" gorm:"index;uniqueIndex:idx_organization_sku_code"`
	ProductID      string            `json:"productId" gorm:"index"`
	Code           string            `json:"code" gorm:"index;uniqueIndex:idx_organization_sku_code"`
	Name           string            `json:"name"`
	Attributes     map[string]string `json:"attributes" gorm:"serializer:json"`
	ImageURLs      []string          `json:"imageUrls" gorm:"serializer:json"`
	Status         ProductStatus     `json:"status" gorm:"index"`
	CreatedBy      string            `json:"createdBy" gorm:"index"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

type ProductSKUList struct {
	Items []ProductSKU `json:"items"`
	Total int          `json:"total"`
}

type BatchProductionStatus string

const (
	BatchProductionStatusQueued    BatchProductionStatus = "queued"
	BatchProductionStatusRunning   BatchProductionStatus = "running"
	BatchProductionStatusCompleted BatchProductionStatus = "completed"
	BatchProductionStatusFailed    BatchProductionStatus = "failed"
	BatchProductionStatusCancelled BatchProductionStatus = "cancelled"
)

type BatchProductionJob struct {
	ID             string                `json:"id" gorm:"primaryKey"`
	OrganizationID string                `json:"organizationId" gorm:"index"`
	BrandID        string                `json:"brandId" gorm:"index"`
	Name           string                `json:"name"`
	PresetID       string                `json:"presetId" gorm:"index"`
	ProductIDs     []string              `json:"productIds" gorm:"serializer:json"`
	Status         BatchProductionStatus `json:"status" gorm:"index"`
	TotalItems     int                   `json:"totalItems"`
	CompletedItems int                   `json:"completedItems"`
	FailedItems    int                   `json:"failedItems"`
	CreatedBy      string                `json:"createdBy" gorm:"index"`
	CreatedAt      string                `json:"createdAt"`
	UpdatedAt      string                `json:"updatedAt"`
}

type BatchProductionSnapshot struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"index"`
	JobID          string `json:"jobId" gorm:"index"`
	Kind           string `json:"kind" gorm:"index"`
	ResourceID     string `json:"resourceId" gorm:"index"`
	Data           string `json:"-" gorm:"type:text"`
	Size           int    `json:"size"`
	CreatedAt      string `json:"createdAt"`
}

type BatchProductionItem struct {
	ID             string                `json:"id" gorm:"primaryKey"`
	OrganizationID string                `json:"organizationId" gorm:"index"`
	JobID          string                `json:"jobId" gorm:"index"`
	ProductID      string                `json:"productId" gorm:"index"`
	SKUID          string                `json:"skuId" gorm:"index"`
	Status         BatchProductionStatus `json:"status" gorm:"index"`
	ResultURL      string                `json:"resultUrl"`
	ErrorMessage   string                `json:"errorMessage" gorm:"type:text"`
	BrandSnapshotID string               `json:"-"`
	ProductSnapshotID string             `json:"-"`
	SKUSnapshotID string                 `json:"-"`
	RunNumber      int                   `json:"runNumber"`
	Attempts       int                   `json:"attempts"`
	LockedAt       string                `json:"lockedAt" gorm:"index"`
	LeaseToken     string                `json:"-" gorm:"index"`
	LeaseExpiresAt string                `json:"leaseExpiresAt" gorm:"index"`
	StartedAt      string                `json:"startedAt"`
	FinishedAt     string                `json:"finishedAt"`
	CreatedAt      string                `json:"createdAt"`
	UpdatedAt      string                `json:"updatedAt"`
}

type BatchProductionItemList struct {
	Items []BatchProductionItem `json:"items"`
	Total int                   `json:"total"`
}

type BatchProductionJobList struct {
	Items []BatchProductionJob `json:"items"`
	Total int                  `json:"total"`
}

type CreateBatchProductionJobInput struct {
	Name       string   `json:"name"`
	BrandID    string   `json:"brandId"`
	PresetID   string   `json:"presetId"`
	ProductIDs []string `json:"productIds"`
}
