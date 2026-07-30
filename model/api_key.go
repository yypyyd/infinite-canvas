package model

type UserAPIKeyStatus string

const (
	UserAPIKeyStatusActive  UserAPIKeyStatus = "active"
	UserAPIKeyStatusRevoked UserAPIKeyStatus = "revoked"
)

// UserAPIKey 保存用户 API 密钥的不可逆摘要，原始密钥只在创建时返回一次。
type UserAPIKey struct {
	ID             string           `json:"id" gorm:"primaryKey"`
	OrganizationID string           `json:"organizationId" gorm:"not null;index;index:idx_user_api_key_owner,priority:1"`
	UserID         string           `json:"userId" gorm:"not null;index;index:idx_user_api_key_owner,priority:2"`
	Name           string           `json:"name" gorm:"size:100;not null"`
	Prefix         string           `json:"prefix" gorm:"size:32;not null"`
	KeyHash        string           `json:"-" gorm:"size:64;not null;uniqueIndex"`
	Status         UserAPIKeyStatus `json:"status" gorm:"size:16;not null;default:active;index"`
	LastUsedAt     string           `json:"lastUsedAt" gorm:"index"`
	RevokedAt      string           `json:"revokedAt"`
	CreatedAt      string           `json:"createdAt"`
	UpdatedAt      string           `json:"updatedAt"`
}

type UserAPIKeyCredential struct {
	UserAPIKey
	Secret string `json:"secret"`
}
