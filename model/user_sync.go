package model

// UserSyncRecord stores one account-scoped cloud document.
type UserSyncRecord struct {
	ID        string `json:"id" gorm:"primaryKey"`
	UserID    string `json:"userId" gorm:"uniqueIndex:idx_user_sync_object;index"`
	Domain    string `json:"domain" gorm:"uniqueIndex:idx_user_sync_object;index"`
	ObjectID  string `json:"objectId" gorm:"uniqueIndex:idx_user_sync_object"`
	Data      string `json:"-" gorm:"type:text"`
	Revision  int64  `json:"revision"`
	ChangeSeq int64  `json:"changeSeq" gorm:"index"`
	DeletedAt string `json:"deletedAt" gorm:"index"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type UserSyncState struct {
	UserID    string `json:"userId" gorm:"primaryKey"`
	Cursor    int64  `json:"cursor"`
	UpdatedAt string `json:"updatedAt"`
}

type UserFile struct {
	ID         string `json:"id" gorm:"primaryKey"`
	UserID     string `json:"userId" gorm:"uniqueIndex:idx_user_file_storage_key;index"`
	StorageKey string `json:"storageKey" gorm:"uniqueIndex:idx_user_file_storage_key"`
	SHA256     string `json:"sha256" gorm:"index"`
	MimeType   string `json:"mimeType"`
	Size       int64  `json:"size"`
	Path       string `json:"-"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type UserSyncMutation struct {
	RecordID     string
	Domain       string
	ObjectID     string
	Data         string
	BaseRevision int64
	Deleted      bool
}
