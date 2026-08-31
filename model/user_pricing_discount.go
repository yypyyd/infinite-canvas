package model

// UserPricingDiscount stores one user-specific pricing ratio for an exact model specification.
type UserPricingDiscount struct {
	ID             string  `json:"id" gorm:"size:191;primaryKey"`
	UserID         string  `json:"userId" gorm:"size:191;not null;index;uniqueIndex:idx_user_pricing_spec,priority:1"`
	Model          string  `json:"model" gorm:"size:191;not null;uniqueIndex:idx_user_pricing_spec,priority:2"`
	Modality       string  `json:"modality" gorm:"size:32;not null;uniqueIndex:idx_user_pricing_spec,priority:3"`
	Operation      string  `json:"operation" gorm:"size:32;not null;uniqueIndex:idx_user_pricing_spec,priority:4"`
	Unit           string  `json:"unit" gorm:"size:32;not null;uniqueIndex:idx_user_pricing_spec,priority:5"`
	ResolutionTier string  `json:"resolutionTier" gorm:"size:32;not null;uniqueIndex:idx_user_pricing_spec,priority:6"`
	Ratio          float64 `json:"ratio" gorm:"not null"`
	Remark         string  `json:"remark" gorm:"size:255"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}
