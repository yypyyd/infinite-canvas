package model

// ReferralSetting controls one-level invitation commission settlement.
type ReferralSetting struct {
	Enabled        bool `json:"enabled"`
	CommissionRate int  `json:"commissionRate"`
}
