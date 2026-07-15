package model

// CheckIn 记录用户每日签到和实际获得的算力点。
type CheckIn struct {
	ID            string `json:"id" gorm:"primaryKey"`
	UserID        string `json:"userId" gorm:"not null;uniqueIndex:idx_check_ins_user_date"`
	CheckInDate   string `json:"checkInDate" gorm:"size:10;not null;uniqueIndex:idx_check_ins_user_date"`
	RewardCredits int    `json:"rewardCredits"`
	CreatedAt     string `json:"createdAt"`
}

type CheckInStatus struct {
	Enabled        bool   `json:"enabled"`
	Reward         bool   `json:"reward"`
	RewardCredits  int    `json:"rewardCredits"`
	CheckedInToday bool   `json:"checkedInToday"`
	CheckInDate    string `json:"checkInDate"`
}

type CheckInResult struct {
	RewardCredits int      `json:"rewardCredits"`
	CheckInDate   string   `json:"checkInDate"`
	User          AuthUser `json:"user"`
}
