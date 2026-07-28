package model

import "encoding/json"

type SettingKey string

const (
	SettingKeyPublic  SettingKey = "public"
	SettingKeyPrivate SettingKey = "private"
)

// ModelChannel stores a private upstream channel.
type ModelChannel struct {
	Protocol string         `json:"protocol"`
	Name     string         `json:"name"`
	BaseURL  string         `json:"baseUrl"`
	APIKey   string         `json:"apiKey"`
	Models   []ChannelModel `json:"models"`
	Weight   int            `json:"weight"`
	Enabled  bool           `json:"enabled"`
	Remark   string         `json:"remark"`
}

// ChannelModel stores one public model's upstream mapping and channel-specific capabilities.
type ChannelModel struct {
	Model           string   `json:"model"`
	UpstreamModel   string   `json:"upstreamModel"`
	Modality        string   `json:"modality"`
	Operations      []string `json:"operations"`
	AspectRatios    []string `json:"aspectRatios"`
	ResolutionTiers []string `json:"resolutionTiers"`
	Durations       []int    `json:"durations"`
}

type DiscoveredModel struct {
	ID                   string   `json:"id"`
	Kind                 string   `json:"kind"`
	Modality             string   `json:"modality"`
	SupportedRatios      []string `json:"supportedRatios"`
	SupportedResolutions []string `json:"supportedResolutions"`
	SupportedDurations   []int    `json:"supportedDurations"`
}

// ModelDefinition stores public model management metadata.
type ModelDefinition struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Modality        string   `json:"modality"`
	Operations      []string `json:"operations"`
	Enabled         bool     `json:"enabled"`
	Sort            int      `json:"sort"`
	AspectRatios    []string `json:"aspectRatios"`
	ResolutionTiers []string `json:"resolutionTiers"`
	Durations       []int    `json:"durations"`
	Remark          string   `json:"remark"`
}

// PricingRule stores model credit billing rules.
type PricingRule struct {
	Model           string  `json:"model"`
	Modality        string  `json:"modality"`
	Operation       string  `json:"operation"`
	Unit            string  `json:"unit"`
	ResolutionTier  string  `json:"resolutionTier"`
	DurationSeconds int     `json:"durationSeconds"`
	BillingMode     string  `json:"billingMode"`
	Credits         int     `json:"credits"`
	MinCredits      int     `json:"minCredits"`
	ModelRatio      float64 `json:"modelRatio"`
	CompletionRatio float64 `json:"completionRatio"`
	Enabled         bool    `json:"enabled"`
	Remark          string  `json:"remark"`
}

// PublicModelChannelSetting stores frontend-visible model channel settings.
type PublicModelChannelSetting struct {
	AvailableModels    []string            `json:"availableModels"`
	Models             []ModelDefinition   `json:"models"`
	PricingRules       []PricingRule       `json:"pricingRules"`
	GroupRatios        map[string]float64  `json:"groupRatios"`
	ModelAspectRatios  map[string][]string `json:"modelAspectRatios"`
	DefaultModel       string              `json:"defaultModel"`
	DefaultImageModel  string              `json:"defaultImageModel"`
	DefaultVideoModel  string              `json:"defaultVideoModel"`
	DefaultTextModel   string              `json:"defaultTextModel"`
	SystemPrompt       string              `json:"systemPrompt"`
}

// PublicSetting stores frontend-visible settings.
type PublicSetting struct {
	ModelChannel  PublicModelChannelSetting `json:"modelChannel"`
	Auth          PublicAuthSetting         `json:"auth"`
	Access        PublicAccessSetting       `json:"access"`
	Announcements AnnouncementSetting       `json:"announcements"`
	CheckIn       CheckInSetting            `json:"checkIn"`
}

type PublicAccessSetting struct {
	BlockChina bool `json:"blockChina"`
}

type PublicAuthSetting struct {
	AllowRegister          *bool    `json:"allowRegister"`
	EmailVerification      bool     `json:"emailVerification"`
	EmailDomainRestriction bool     `json:"emailDomainRestriction"`
	EmailDomains           []string `json:"emailDomains"`
	NewUserReward          bool     `json:"newUserReward"`
	NewUserRewardCredits   int      `json:"newUserRewardCredits"`
}

type Announcement struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	PublishAt string `json:"publishAt"`
	Enabled   bool   `json:"enabled"`
}

type AnnouncementSetting struct {
	Enabled bool           `json:"enabled"`
	Items   []Announcement `json:"items"`
}

type CheckInSetting struct {
	Enabled       bool `json:"enabled"`
	Reward        bool `json:"reward"`
	RewardCredits int  `json:"rewardCredits"`
}

// PrivateSetting stores backend-only settings.
type PrivateSetting struct {
	Channels         []ModelChannel         `json:"channels"`
	PromptSync       PromptSyncSetting      `json:"promptSync"`
	Email            EmailSetting           `json:"email"`
	OperationsAlerts OperationsAlertSetting `json:"operationsAlerts"`
}

type OperationsAlertSetting struct {
	Enabled                             *bool  `json:"enabled"`
	BatchQueuedThreshold                *int64 `json:"batchQueuedThreshold"`
	BatchExpiredLeasesThreshold         *int64 `json:"batchExpiredLeasesThreshold"`
	EmailPendingThreshold               *int64 `json:"emailPendingThreshold"`
	EmailFailedThreshold                *int64 `json:"emailFailedThreshold"`
	EmailExpiredLeasesThreshold         *int64 `json:"emailExpiredLeasesThreshold"`
	ObjectDeletionPendingThreshold      *int64 `json:"objectDeletionPendingThreshold"`
	ObjectDeletionFailedThreshold       *int64 `json:"objectDeletionFailedThreshold"`
	ObjectDeletionExpiredLeasesThreshold *int64 `json:"objectDeletionExpiredLeasesThreshold"`
}

type EmailSetting struct {
	SMTPHost           string `json:"smtpHost"`
	SMTPPort           int    `json:"smtpPort"`
	SMTPUsername       string `json:"smtpUsername"`
	SMTPPassword       string `json:"smtpPassword"`
	SMTPFromEmail      string `json:"smtpFromEmail"`
	SMTPFromName       string `json:"smtpFromName"`
	SMTPSecurity       string `json:"smtpSecurity"`
	PasswordConfigured bool  `json:"passwordConfigured"`
}

type PromptSyncSetting struct {
	Enabled *bool  `json:"enabled"`
	Cron    string `json:"cron"`
}

// Setting stores a JSON settings row.
type Setting struct {
	Key       SettingKey      `json:"key" gorm:"primaryKey"`
	Value     json.RawMessage `json:"value" gorm:"serializer:json"`
	CreatedAt string          `json:"createdAt"`
	UpdatedAt string          `json:"updatedAt"`
}

// Settings stores public and private settings together.
type Settings struct {
	Public  PublicSetting  `json:"public"`
	Private PrivateSetting `json:"private"`
}
