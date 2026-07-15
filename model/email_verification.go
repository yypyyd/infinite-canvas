package model

// EmailVerification stores the latest registration code for one normalized email.
type EmailVerification struct {
	ID        string `json:"id" gorm:"primaryKey"`
	Email     string `json:"email" gorm:"index"`
	CodeHash  string `json:"-"`
	RequestIP string `json:"-" gorm:"index"`
	ExpiresAt string `json:"expiresAt" gorm:"index"`
	SentAt    string `json:"sentAt" gorm:"index"`
	Attempts  int    `json:"attempts"`
	UsedAt    string `json:"usedAt"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}
