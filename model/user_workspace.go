package model

// UserProject is the authoritative account-scoped canvas document.
type UserProject struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"uniqueIndex:idx_user_project;index"`
	Title     string `json:"title"`
	Data      string `json:"-" gorm:"type:text"`
	Version   int64  `json:"version"`
	DeletedAt string `json:"deletedAt" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// UserAsset is the authoritative account-scoped personal asset.
type UserAsset struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"uniqueIndex:idx_user_asset;index"`
	Kind      string `json:"kind" gorm:"index"`
	Title     string `json:"title"`
	Data      string `json:"-" gorm:"type:text"`
	Version   int64  `json:"version"`
	DeletedAt string `json:"deletedAt" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type UserGenerationRecord struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"uniqueIndex:idx_user_generation_record;index"`
	Kind      string `json:"kind" gorm:"index"`
	Data      string `json:"-" gorm:"type:text"`
	Version   int64  `json:"version"`
	DeletedAt string `json:"deletedAt" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// UserProjectVersion keeps recoverable snapshots before a project is overwritten.
type UserProjectVersion struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"index"`
	ProjectID string `json:"projectId" gorm:"index"`
	Version   int64  `json:"version"`
	Data      string `json:"-" gorm:"type:text"`
	CreatedAt string `json:"createdAt"`
}

type UserWorkspaceState struct {
	UserID    string `json:"userId" gorm:"primaryKey"`
	UpdatedAt string `json:"updatedAt"`
}

type UserFile struct {
	ID         string `json:"id" gorm:"primaryKey"`
	UserID     string `json:"userId" gorm:"uniqueIndex:idx_user_file_storage_key;index"`
	StorageKey string `json:"storageKey" gorm:"uniqueIndex:idx_user_file_storage_key"`
	ObjectKey  string `json:"-" gorm:"uniqueIndex"`
	Hash       string `json:"hash" gorm:"index"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type UserWorkspaceMutation struct {
	RecordID string
	Domain   string
	ObjectID string
	Title    string
	Kind     string
	Data     string
	Deleted  bool
}
