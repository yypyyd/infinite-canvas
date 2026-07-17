package model

// UserPreference stores small account-scoped UI and creation preferences.
type UserPreference struct {
	UserID    string `json:"userId" gorm:"primaryKey"`
	Data      string `json:"-" gorm:"type:text"`
	UpdatedAt string `json:"updatedAt"`
}
