package model

// UserProject is the authoritative account-scoped canvas document.
type UserProject struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"primaryKey;index"`
	UserID         string `json:"userId" gorm:"index"`
	Title          string `json:"title"`
	Data           string `json:"-" gorm:"type:text"`
	Version        int64  `json:"version"`
	DeletedAt      string `json:"deletedAt" gorm:"index"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// UserAsset is the authoritative account-scoped personal asset.
type UserAsset struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"primaryKey;index"`
	UserID         string `json:"userId" gorm:"index"`
	Kind           string `json:"kind" gorm:"index"`
	Title          string `json:"title"`
	Data           string `json:"-" gorm:"type:text"`
	Version        int64  `json:"version"`
	DeletedAt      string `json:"deletedAt" gorm:"index"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type UserGenerationRecord struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"primaryKey;index"`
	UserID         string `json:"userId" gorm:"index"`
	Kind           string `json:"kind" gorm:"index"`
	Data           string `json:"-" gorm:"type:text"`
	Version        int64  `json:"version"`
	DeletedAt      string `json:"deletedAt" gorm:"index"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

// UserProjectVersion keeps recoverable snapshots before a project is overwritten.
type UserProjectVersion struct {
	ID             string `json:"id" gorm:"primaryKey"`
	UserID         string `json:"userId" gorm:"index"`
	OrganizationID string `json:"organizationId" gorm:"index"`
	ProjectID      string `json:"projectId" gorm:"index"`
	Version        int64  `json:"version"`
	Data           string `json:"-" gorm:"type:text"`
	CreatedAt      string `json:"createdAt"`
}

type UserWorkspaceState struct {
	OrganizationID string `json:"organizationId" gorm:"primaryKey"`
	UpdatedAt      string `json:"updatedAt"`
}

type UserFile struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"uniqueIndex:idx_organization_file_storage_key;index;index:idx_workspace_file_gc,priority:1"`
	UserID         string `json:"userId" gorm:"index"`
	StorageKey     string `json:"storageKey" gorm:"size:191;uniqueIndex:idx_organization_file_storage_key"`
	ObjectKey      string `json:"-" gorm:"size:512;uniqueIndex"`
	Hash           string `json:"hash" gorm:"index"`
	MimeType       string `json:"mimeType"`
	Size           int64  `json:"size"`
	UnreferencedAt string `json:"-" gorm:"index;index:idx_workspace_file_gc,priority:2"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type UserFileReference struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"organizationId" gorm:"uniqueIndex:idx_workspace_file_reference;index"`
	Domain         string `json:"domain" gorm:"uniqueIndex:idx_workspace_file_reference"`
	ObjectID       string `json:"objectId" gorm:"uniqueIndex:idx_workspace_file_reference"`
	StorageKey     string `json:"storageKey" gorm:"size:191;uniqueIndex:idx_workspace_file_reference;index"`
	CreatedAt      string `json:"createdAt"`
}

type UserFileUploadReservation struct {
	ID                   string `json:"id" gorm:"primaryKey"`
	OrganizationID       string `json:"organizationId" gorm:"uniqueIndex:idx_workspace_upload_storage_key;index;index:idx_workspace_upload_expiry,priority:1"`
	UserID               string `json:"userId" gorm:"index"`
	StorageKey           string `json:"storageKey" gorm:"size:191;uniqueIndex:idx_workspace_upload_storage_key"`
	ObjectKey            string `json:"objectKey" gorm:"size:512;uniqueIndex"`
	MimeType             string `json:"mimeType"`
	Size                 int64  `json:"size"`
	ReservedBytes        int64  `json:"reservedBytes"`
	CleanupReservedBytes int64  `json:"-"`
	ReplaceExisting      bool   `json:"-"`
	ReplaceObjectKey     string `json:"-" gorm:"size:512"`
	ExpiresAt            string `json:"expiresAt" gorm:"index;index:idx_workspace_upload_expiry,priority:2"`
	CleanupAfter         string `json:"-"`
	CreatedAt            string `json:"createdAt"`
}

type UserFileUploadRateLimit struct {
	OrganizationID  string `json:"-" gorm:"primaryKey"`
	Scope           string `json:"-" gorm:"primaryKey;size:128"`
	WindowStartedAt string `json:"-"`
	Requests        int    `json:"-"`
	UpdatedAt       string `json:"-"`
}

type UserObjectDeletion struct {
	ID             string `json:"id" gorm:"primaryKey"`
	OrganizationID string `json:"-" gorm:"index;index:idx_workspace_object_cleanup,priority:1"`
	UserID         string `json:"-" gorm:"index"`
	ObjectKey      string `json:"objectKey" gorm:"size:512;uniqueIndex"`
	Size           int64  `json:"-"`
	Status         string `json:"status" gorm:"index;index:idx_workspace_object_cleanup,priority:2"`
	Attempts       int    `json:"attempts"`
	LeaseToken     string `json:"-" gorm:"index"`
	LeaseExpiresAt string `json:"leaseExpiresAt" gorm:"index"`
	NextAttemptAt  string `json:"nextAttemptAt" gorm:"index;index:idx_workspace_object_cleanup,priority:3"`
	LastError      string `json:"lastError" gorm:"type:text"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

type UserWorkspaceMutation struct {
	RecordID        string
	Domain          string
	ObjectID        string
	Title           string
	Kind            string
	Data            string
	Deleted         bool
	ExpectedVersion int64
	StorageKeys     []string
}
