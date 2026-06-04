package model

type RedemptionCodeStatus string

const (
	RedemptionCodeStatusActive   RedemptionCodeStatus = "active"
	RedemptionCodeStatusUsed     RedemptionCodeStatus = "used"
	RedemptionCodeStatusDisabled RedemptionCodeStatus = "disabled"
)

// RedemptionCode is a one-time code users can redeem for credits.
type RedemptionCode struct {
	ID        string               `json:"id" gorm:"primaryKey"`
	Code      string               `json:"code" gorm:"uniqueIndex"`
	Credits   int                  `json:"credits"`
	Status    RedemptionCodeStatus `json:"status" gorm:"index"`
	UsedBy    string               `json:"usedBy" gorm:"index"`
	UsedAt    string               `json:"usedAt"`
	Remark    string               `json:"remark"`
	CreatedAt string               `json:"createdAt"`
	UpdatedAt string               `json:"updatedAt"`
}

type RedemptionCodeList struct {
	Items []RedemptionCode `json:"items"`
	Total int              `json:"total"`
}
