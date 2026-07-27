package model

type UserRole string

const (
	UserRoleGuest UserRole = "guest"
	UserRoleUser  UserRole = "user"
	UserRoleAdmin UserRole = "admin"
)

type UserStatus string

const (
	UserStatusActive UserStatus = "active"
	UserStatusBan    UserStatus = "ban"
)

// User 系统用户。
type User struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	Username    string     `json:"username" gorm:"uniqueIndex"`
	Password    string     `json:"password,omitempty"`
	Email       string     `json:"email" gorm:"index"`
	EmailKey    *string    `json:"-" gorm:"uniqueIndex"`
	DisplayName string     `json:"displayName"`
	AvatarURL   string     `json:"avatarUrl"`
	OrganizationID string  `json:"organizationId" gorm:"index"`
	Role        UserRole   `json:"role"`
	Group       string     `json:"group"`
	Credits         int        `json:"credits"`
	ReservedCredits int        `json:"-" gorm:"not null;default:0"`
	AffCode         string     `json:"affCode" gorm:"uniqueIndex"`
	AffCount    int        `json:"affCount"`
	InviterID   string     `json:"inviterId"`
	GithubID    string     `json:"githubId"`
	LinuxDoID   string     `json:"linuxDoId" gorm:"index"`
	WechatID    string     `json:"wechatId"`
	Status      UserStatus `json:"status"`
	LastLoginAt string     `json:"lastLoginAt"`
	Extra       string     `json:"extra" gorm:"type:text"`
	CreatedAt   string     `json:"createdAt"`
	UpdatedAt   string     `json:"updatedAt"`
}

// UserList 用户分页结果。
type UserList struct {
	Items []User `json:"items"`
	Total int    `json:"total"`
}

// AuthUser 用户公开信息。
type AuthUser struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	Email       string   `json:"email"`
	DisplayName string   `json:"displayName"`
	AvatarURL   string   `json:"avatarUrl"`
	OrganizationID string `json:"organizationId"`
	Role        UserRole `json:"role"`
	Group       string   `json:"group"`
	Credits     int      `json:"credits"`
	EffectiveCredits int `json:"effectiveCredits"`
	CreditMode OrganizationCreditMode `json:"creditMode"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}

// AuthSession 登录会话信息。
type AuthSession struct {
	Token string   `json:"token"`
	User  AuthUser `json:"user"`
}

func PublicUser(user User) AuthUser {
	return AuthUser{
		ID:          user.ID,
		Username:    user.Username,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		AvatarURL:   user.AvatarURL,
		OrganizationID: user.OrganizationID,
		Role:        user.Role,
		Group:       user.Group,
		Credits:     user.Credits,
		EffectiveCredits: user.Credits,
		CreditMode: OrganizationCreditModePersonal,
		CreatedAt:   user.CreatedAt,
		UpdatedAt:   user.UpdatedAt,
	}
}

type CreditLogType string

const (
	CreditLogTypeAdminAdjust CreditLogType = "admin_adjust"
	CreditLogTypeAIConsume   CreditLogType = "ai_consume"
	CreditLogTypeAIRefund    CreditLogType = "ai_refund"
	CreditLogTypeRedeem      CreditLogType = "redeem_code"
	CreditLogTypeCheckIn     CreditLogType = "daily_check_in"
	CreditLogTypeNewUser     CreditLogType = "new_user_reward"
	CreditLogTypeOrganizationTransferOut CreditLogType = "organization_transfer_out"
	CreditLogTypeOrganizationTransferIn  CreditLogType = "organization_transfer_in"
)

// CreditLog 用户算力点变更流水。
type CreditLog struct {
	ID        string        `json:"id" gorm:"primaryKey"`
	UserID    string        `json:"userId" gorm:"index"`
	OrganizationID string   `json:"organizationId" gorm:"index"`
	CreditSource CreditSource `json:"creditSource" gorm:"size:16;not null;default:personal;index"`
	Type      CreditLogType `json:"type"`
	Amount    int           `json:"amount"`
	Balance   int           `json:"balance"`
	RelatedID string        `json:"relatedId"`
	Remark    string        `json:"remark"`
	Extra     string        `json:"extra" gorm:"type:text"`
	CreatedAt string        `json:"createdAt"`
}

type CreditLogList struct {
	Items []CreditLog `json:"items"`
	Total int         `json:"total"`
}
