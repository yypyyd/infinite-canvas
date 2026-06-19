package model

import "encoding/json"

type SettingKey string

const (
	SettingKeyPublic  SettingKey = "public"
	SettingKeyPrivate SettingKey = "private"
)

// ModelChannel stores a private upstream channel.
type ModelChannel struct {
	Protocol string   `json:"protocol"`
	Name     string   `json:"name"`
	BaseURL  string   `json:"baseUrl"`
	APIKey   string   `json:"apiKey"`
	Models   []string `json:"models"`
	Weight   int      `json:"weight"`
	Enabled  bool     `json:"enabled"`
	Remark   string   `json:"remark"`
}

// ModelDefinition stores public model management metadata.
type ModelDefinition struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Modality        string   `json:"modality"`
	Enabled         bool     `json:"enabled"`
	Sort            int      `json:"sort"`
	AspectRatios    []string `json:"aspectRatios"`
	ResolutionTiers []string `json:"resolutionTiers"`
	Remark          string   `json:"remark"`
}

// PricingRule stores model credit billing rules.
type PricingRule struct {
	Model           string  `json:"model"`
	Modality        string  `json:"modality"`
	Operation       string  `json:"operation"`
	Unit            string  `json:"unit"`
	ResolutionTier  string  `json:"resolutionTier"`
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
	AllowCustomChannel *bool               `json:"allowCustomChannel"`
}

// PublicSetting stores frontend-visible settings.
type PublicSetting struct {
	ModelChannel PublicModelChannelSetting `json:"modelChannel"`
	Auth         PublicAuthSetting         `json:"auth"`
}

type PublicAuthSetting struct {
	AllowRegister *bool                    `json:"allowRegister"`
	LinuxDo       PublicLinuxDoAuthSetting `json:"linuxDo"`
}

type PublicLinuxDoAuthSetting struct {
	Enabled bool `json:"enabled"`
}

// PrivateSetting stores backend-only settings.
type PrivateSetting struct {
	Channels   []ModelChannel     `json:"channels"`
	PromptSync PromptSyncSetting  `json:"promptSync"`
	Auth       PrivateAuthSetting `json:"auth"`
}

type PromptSyncSetting struct {
	Enabled *bool  `json:"enabled"`
	Cron    string `json:"cron"`
}

type PrivateAuthSetting struct {
	LinuxDo PrivateLinuxDoAuthSetting `json:"linuxDo"`
}

type PrivateLinuxDoAuthSetting struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
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
