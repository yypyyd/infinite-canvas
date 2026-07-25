package model

type OrganizationRole string
type OrganizationCreditMode string

const (
	OrganizationRoleOwner    OrganizationRole = "owner"
	OrganizationRoleAdmin    OrganizationRole = "admin"
	OrganizationRoleMember   OrganizationRole = "member"
	OrganizationRoleReviewer OrganizationRole = "reviewer"
)

const (
	OrganizationCreditModePersonal OrganizationCreditMode = "personal"
	OrganizationCreditModeShared   OrganizationCreditMode = "shared"
)

type Organization struct {
	ID        string `json:"id" gorm:"primaryKey;index:idx_batch_claim_organizations,priority:3"`
	Name      string `json:"name"`
	Slug      string `json:"slug" gorm:"uniqueIndex"`
	Status    string `json:"status" gorm:"index;index:idx_batch_claim_organizations,priority:1"`
	Version   int64  `json:"version"`
	CreditMode           OrganizationCreditMode `json:"-" gorm:"size:16;not null;default:personal"`
	Credits              int                    `json:"-" gorm:"not null;default:0"`
	MonthlyCreditBudget  int                    `json:"-" gorm:"not null;default:0"`
	MonthlyCreditsUsed   int                    `json:"-" gorm:"not null;default:0"`
	CreditBudgetMonth    string                 `json:"-" gorm:"size:7;not null;default:''"`
	CreditAlertThreshold int                    `json:"-" gorm:"not null;default:80"`
	BatchClaimCursor string `json:"-" gorm:"size:128;index;not null;default:'';index:idx_batch_claim_organizations,priority:2"`
	CreatedBy string `json:"createdBy" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type OrganizationMember struct {
	ID             string           `json:"id" gorm:"primaryKey"`
	OrganizationID string           `json:"organizationId" gorm:"uniqueIndex:idx_organization_user;index"`
	UserID         string           `json:"userId" gorm:"uniqueIndex:idx_organization_user;index"`
	Role           OrganizationRole `json:"role" gorm:"index"`
	Version        int64            `json:"version"`
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
	Version     int64            `json:"version"`
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
	CreditSummary OrganizationCreditSummary `json:"creditSummary"`
}

type OrganizationCreditSummary struct {
	Mode            OrganizationCreditMode `json:"mode"`
	Balance         int                    `json:"balance"`
	PersonalBalance int                    `json:"personalBalance"`
	MonthlyBudget   int                    `json:"monthlyBudget"`
	MonthlyUsed     int                    `json:"monthlyUsed"`
	AlertThreshold  int                    `json:"alertThreshold"`
	Warning         bool                   `json:"warning"`
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
	LogoStorageKey  string   `json:"logoStorageKey"`
	Colors          []string `json:"colors" gorm:"serializer:json"`
	Fonts           []string `json:"fonts" gorm:"serializer:json"`
	Tone            string   `json:"tone" gorm:"type:text"`
	Guidelines      string   `json:"guidelines" gorm:"type:text"`
	ProhibitedTerms []string `json:"prohibitedTerms" gorm:"serializer:json"`
	Version         int64    `json:"version"`
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
	Version        int64         `json:"version"`
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
	ImageStorageKeys []string        `json:"imageStorageKeys" gorm:"serializer:json"`
	Status         ProductStatus     `json:"status" gorm:"index"`
	Version        int64             `json:"version"`
	CreatedBy      string            `json:"createdBy" gorm:"index"`
	CreatedAt      string            `json:"createdAt"`
	UpdatedAt      string            `json:"updatedAt"`
}

type ProductSKUList struct {
	Items []ProductSKU `json:"items"`
	Total int          `json:"total"`
}

type ProductionTemplateStatus string

const (
	ProductionTemplateStatusActive   ProductionTemplateStatus = "active"
	ProductionTemplateStatusDisabled ProductionTemplateStatus = "disabled"
)

type ProductionTemplate struct {
	ID             string                   `json:"id" gorm:"primaryKey"`
	OrganizationID string                   `json:"organizationId" gorm:"index;uniqueIndex:idx_organization_template_name"`
	Name           string                   `json:"name" gorm:"uniqueIndex:idx_organization_template_name"`
	Description    string                   `json:"description"`
	Status         ProductionTemplateStatus `json:"status" gorm:"index"`
	CurrentVersion int                      `json:"currentVersion"`
	CurrentPrompt  string                   `json:"currentPrompt" gorm:"-"`
	Version        int64                    `json:"version"`
	CreatedBy      string                   `json:"createdBy" gorm:"index"`
	CreatedAt      string                   `json:"createdAt"`
	UpdatedAt      string                   `json:"updatedAt"`
}

type ProductionTemplateVersion struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"index"`
	TemplateID     string `json:"templateId" gorm:"index;uniqueIndex:idx_template_version"`
	Version        int    `json:"version" gorm:"uniqueIndex:idx_template_version"`
	Prompt         string `json:"prompt" gorm:"type:text"`
	CreatedBy      string `json:"createdBy" gorm:"index"`
	CreatedAt      string `json:"createdAt"`
}

type ProductionTemplateList struct {
	Items []ProductionTemplate `json:"items"`
	Total int                  `json:"total"`
}

type SaveProductionTemplateInput struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Status      ProductionTemplateStatus `json:"status"`
	Prompt      string                   `json:"prompt"`
	Version     int64                    `json:"version"`
}

type PreviewProductionPromptInput struct {
	PresetID      string `json:"presetId"`
	PresetVersion int    `json:"presetVersion"`
	DeliverySpecID string `json:"deliverySpecId"`
	BrandID       string `json:"brandId"`
	ProductID     string `json:"productId"`
	SKUID         string `json:"skuId"`
}

type ProductionPromptPreview struct {
	Prompt string `json:"prompt"`
}

type ProductionDeliverySpec struct {
	ID              string `json:"id"`
	Platform        string `json:"platform"`
	Name            string `json:"name"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	Format          string `json:"format"`
	Quality         int    `json:"quality"`
	FilenamePattern string `json:"filenamePattern"`
}

type BatchProductionStatus string

const (
	BatchProductionStatusQueued    BatchProductionStatus = "queued"
	BatchProductionStatusRunning   BatchProductionStatus = "running"
	BatchProductionStatusCompleted BatchProductionStatus = "completed"
	BatchProductionStatusFailed    BatchProductionStatus = "failed"
	BatchProductionStatusCancelled BatchProductionStatus = "cancelled"
)

type BatchProductionReviewStatus string

const (
	BatchProductionReviewPending  BatchProductionReviewStatus = "pending"
	BatchProductionReviewApproved BatchProductionReviewStatus = "approved"
	BatchProductionReviewRejected BatchProductionReviewStatus = "rejected"
)

type BatchProductionJob struct {
	ID             string                `json:"id" gorm:"primaryKey"`
	OrganizationID string                `json:"organizationId" gorm:"index;uniqueIndex:idx_organization_batch_request"`
	RequestID      string                `json:"requestId" gorm:"uniqueIndex:idx_organization_batch_request"`
	RequestHash    string                `json:"-" gorm:"size:64"`
	ArchiveToken   string                `json:"-" gorm:"size:64"`
	BrandID        string                `json:"brandId" gorm:"index"`
	Name           string                `json:"name"`
	PresetID       string                `json:"presetId" gorm:"index"`
	PresetVersion  int                   `json:"presetVersion"`
	PresetPrompt   string                `json:"-" gorm:"type:text"`
	DeliverySpec   ProductionDeliverySpec `json:"deliverySpec" gorm:"embedded;embeddedPrefix:delivery_"`
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
	ID             string                `json:"id" gorm:"primaryKey;index:idx_batch_ready,priority:4"`
	OrganizationID string                `json:"organizationId" gorm:"index;index:idx_batch_ready,priority:1;index:idx_batch_retry,priority:1;index:idx_batch_running,priority:1;index:idx_batch_expired,priority:4"`
	JobID          string                `json:"jobId" gorm:"index"`
	ProductID      string                `json:"productId" gorm:"index"`
	SKUID          string                `json:"skuId" gorm:"index"`
	Status         BatchProductionStatus `json:"status" gorm:"index;index:idx_batch_ready,priority:2;index:idx_batch_retry,priority:2;index:idx_batch_running,priority:2;index:idx_batch_expired,priority:1"`
	ResultStorageKey string              `json:"resultStorageKey" gorm:"size:191"`
	ErrorMessage   string                `json:"errorMessage" gorm:"type:text"`
	ReviewStatus   BatchProductionReviewStatus `json:"reviewStatus" gorm:"index"`
	ReviewComment  string                `json:"reviewComment" gorm:"type:text"`
	ReviewedBy     string                `json:"reviewedBy" gorm:"index"`
	ReviewedAt     string                `json:"reviewedAt"`
	IsPrimary      bool                  `json:"isPrimary" gorm:"index"`
	BrandSnapshotID string               `json:"-"`
	ProductSnapshotID string             `json:"-"`
	SKUSnapshotID string                 `json:"-"`
	RunNumber      int                   `json:"runNumber"`
	Attempts       int                   `json:"attempts" gorm:"index:idx_batch_retry,priority:4;index:idx_batch_expired,priority:3"`
	LockedAt       string                `json:"lockedAt" gorm:"index"`
	LeaseToken     string                `json:"-" gorm:"index"`
	LeaseExpiresAt string                `json:"leaseExpiresAt" gorm:"index;index:idx_batch_retry,priority:3;index:idx_batch_running,priority:3;index:idx_batch_expired,priority:2"`
	StartedAt      string                `json:"startedAt"`
	FinishedAt     string                `json:"finishedAt"`
	CreatedAt      string                `json:"createdAt" gorm:"index:idx_batch_ready,priority:3"`
	UpdatedAt      string                `json:"updatedAt"`
	ResultMimeType string                `json:"resultMimeType" gorm:"-"`
	ResultSize     int64                 `json:"resultSize" gorm:"-"`
	QualityContext *BatchProductionQualityContext `json:"qualityContext,omitempty" gorm:"-"`
}

type BatchProductionQualityContext struct {
	Brand   *Brand      `json:"brand,omitempty"`
	Product Product     `json:"product"`
	SKU     *ProductSKU `json:"sku,omitempty"`
}

type BatchProductionItemList struct {
	Items []BatchProductionItem `json:"items"`
	Total int                   `json:"total"`
}

type BatchProductionArchiveItem struct {
	ID               string `json:"-"`
	ProductID        string `json:"-"`
	ProductCode      string `json:"-"`
	SKUID            string `json:"-"`
	SKUCode          string `json:"-"`
	ResultStorageKey string `json:"-"`
	MimeType         string `json:"-"`
	Size             int64  `json:"-"`
	IsPrimary        bool   `json:"-"`
}

type BatchProductionJobList struct {
	Items []BatchProductionJob `json:"items"`
	Total int                  `json:"total"`
}

type CreateBatchProductionJobInput struct {
	RequestID  string   `json:"requestId"`
	Name       string   `json:"name"`
	BrandID    string   `json:"brandId"`
	PresetID   string   `json:"presetId"`
	PresetVersion int   `json:"presetVersion"`
	DeliverySpecID string `json:"deliverySpecId"`
	ProductIDs []string `json:"productIds"`
}

type ReviewBatchProductionItemInput struct {
	RunNumber int                         `json:"runNumber"`
	Status    BatchProductionReviewStatus `json:"status"`
	Comment   string                      `json:"comment"`
}

type BatchProductionItemRunInput struct {
	RunNumber int `json:"runNumber"`
}
