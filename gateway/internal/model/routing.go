package model

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"gorm.io/datatypes"
)

// LogicalModel is the only model identity visible to API consumers. Upstream
// protocol names stay on ModelRoute. When the same product is offered by more
// than one provider, each provider has its own public ID.
type LogicalModel struct {
	ID              string `gorm:"primaryKey;size:191"`
	Kind            string `gorm:"size:32;index;not null"`
	Name            string `gorm:"size:255;not null"`
	Enabled         bool   `gorm:"not null;default:true;index"`
	Weight          int    `gorm:"not null;default:0;index"`
	GenerationCount int64  `gorm:"not null;default:0"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ModelRoute maps one public logical model to one real provider implementation.
// RuntimeModel is the legacy/provider adapter selector. UpstreamModel is the id
// sent over the wire when it differs from RuntimeModel.
type ModelRoute struct {
	ID             string `gorm:"primaryKey;size:191"`
	LogicalModelID string `gorm:"size:191;not null;uniqueIndex:ux_route_identity,priority:1;index"`
	Provider       string `gorm:"size:64;not null;uniqueIndex:ux_route_identity,priority:2;index"`
	RuntimeModel   string `gorm:"size:255;not null;uniqueIndex:ux_route_identity,priority:3"`
	UpstreamModel  string `gorm:"size:255;not null;default:''"`
	Enabled        bool   `gorm:"not null;default:true;index"`
	// Priority/Weight are nullable overrides; NULL inherits the provider
	// default (see service/provider_priority.go).
	Priority       *int           `gorm:"index"`
	Weight         *int           `gorm:""`
	QuotaBucketKey string         `gorm:"size:128;not null;default:''"`
	QuotaCosts     datatypes.JSON `gorm:"type:jsonb;not null;default:'{}'"`
	Capabilities   datatypes.JSON `gorm:"type:jsonb;not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CapabilityProfile is deliberately evaluated as a whole. Matching each field
// against a model-wide union would invent unsupported ratio/resolution/duration
// combinations when multiple routes expose different capabilities.
type CapabilityProfile struct {
	Operations          []string `json:"operations,omitempty"`
	Ratios              []string `json:"ratios,omitempty"`
	Resolutions         []string `json:"resolutions,omitempty"`
	Durations           []string `json:"durations,omitempty"`
	MaxReferenceImages  int      `json:"max_reference_images,omitempty"`
	MaxReferenceVideos  int      `json:"max_reference_videos,omitempty"`
	MaxReferenceAudios  int      `json:"max_reference_audios,omitempty"`
	MaxReferenceMedia   int      `json:"max_reference_media,omitempty"`
	SupportsAudioOutput bool     `json:"supports_audio_output,omitempty"`
	ReferenceMode       string   `json:"reference_mode,omitempty"`
	RequiresReference   bool     `json:"requires_reference,omitempty"`
	// MaxImagesPerRequest caps images per request on this route; 0 = no cap.
	MaxImagesPerRequest int `json:"max_images_per_request,omitempty"`
}

type RouteRequirements struct {
	Operation       string
	Ratio           string
	Resolution      string
	Quality         string
	Duration        string
	ReferenceImages int
	ReferenceVideos int
	ReferenceAudios int
	GenerateAudio   bool
}

// QuotaCostPolicy describes how a route translates one normalized request
// into the provider allowance units stored in AccountQuotaBucket. Unknown is
// deliberately distinct from unmetered: both avoid inventing a reservation,
// but unknown providers are still refreshed after every real attempt.
type QuotaCostPolicy struct {
	Mode       string  `json:"mode"`                 // metered|unknown|unmetered
	Unit       string  `json:"unit,omitempty"`       // credits|points|generations
	Calculator string  `json:"calculator,omitempty"` // fixed|per_second|byteplus_image|oreate_seedance
	Value      float64 `json:"value,omitempty"`
	PerSecond  float64 `json:"per_second,omitempty"`
}

// AccountModelRoute scopes provider entitlements to an account+route pair. An
// account can be disabled for one model without removing it from the provider.
type AccountModelRoute struct {
	ID               string     `gorm:"primaryKey;size:191"`
	AccountID        string     `gorm:"size:64;not null;uniqueIndex:ux_account_route,priority:1;index"`
	ModelRouteID     string     `gorm:"size:191;not null;uniqueIndex:ux_account_route,priority:2;index"`
	Enabled          bool       `gorm:"not null;default:true;index"`
	Entitled         bool       `gorm:"not null;default:true"`
	QuotaBucketKey   string     `gorm:"size:128;not null;default:''"`
	ConsecutiveFails int        `gorm:"not null;default:0"`
	SuccessTotal     int64      `gorm:"not null;default:0"`
	CooldownUntil    *time.Time `gorm:"index"`
	LastFailureClass string     `gorm:"size:32"`
	LastFailureAt    *time.Time
	LastSuccessAt    *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// AccountQuotaBucket is the durable source of truth for schedulable upstream
// allowance. Remaining excludes Reserved; both are updated under a row lock.
type AccountQuotaBucket struct {
	ProbeAllowsReservations bool   `gorm:"-"`
	ID                      string `gorm:"primaryKey;size:191"`
	AccountID               string `gorm:"size:64;not null;uniqueIndex:ux_account_bucket,priority:1;index"`
	BucketKey               string `gorm:"size:128;not null;uniqueIndex:ux_account_bucket,priority:2;index"`
	Unit                    string `gorm:"size:32;not null;default:'credits'"`
	Total                   *float64
	Remaining               *float64
	Reserved                float64    `gorm:"not null;default:0"`
	ResetAt                 *time.Time `gorm:"index"`
	Revision                int64      `gorm:"not null;default:0"`
	RefreshedAt             *time.Time
	RefreshAfter            *time.Time
	RefreshFailures         int
	LastProbeAt             *time.Time
	ProbeID                 string
	ProbeUntil              *time.Time
	RefreshRequestedAt      *time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

type QuotaReservation struct {
	BalanceKnown      bool    `gorm:"-"`
	ID                string  `gorm:"primaryKey;size:64"`
	EventID           string  `gorm:"size:64;not null;index"`
	DispatchAttemptID string  `gorm:"size:64;index"`
	QuotaBucketID     string  `gorm:"size:191;not null;index"`
	Amount            float64 `gorm:"not null"`
	Status            string  `gorm:"size:32;not null;index"` // held|settled|released|uncertain
	CreatedAt         time.Time
	UpdatedAt         time.Time
	SettledAt         *time.Time
}

type DispatchAttempt struct {
	ID             string    `gorm:"primaryKey;size:64"`
	EventID        string    `gorm:"size:64;not null;index"`
	ModelRouteID   string    `gorm:"size:191;not null;index"`
	AccountID      string    `gorm:"size:64;index"`
	State          string    `gorm:"size:32;not null;index"` // created|submitting|accepted|unknown|succeeded|failed
	FailureClass   string    `gorm:"size:32;index"`
	UpstreamTaskID string    `gorm:"size:255"`
	Error          string    `gorm:"type:text"`
	StartedAt      time.Time `gorm:"index"`
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type CanonicalModelDefinition struct {
	Model  LogicalModel
	Routes []ModelRoute
}

func capabilityJSON(profiles ...CapabilityProfile) datatypes.JSON {
	b, err := json.Marshal(profiles)
	if err != nil {
		panic(err)
	}
	return datatypes.JSON(b)
}

func quotaCostJSON(policy QuotaCostPolicy) datatypes.JSON {
	b, err := json.Marshal(policy)
	if err != nil {
		panic(err)
	}
	return datatypes.JSON(b)
}

func route(id, logical, provider, runtime, upstream string, profiles ...CapabilityProfile) ModelRoute {
	return ModelRoute{
		ID: id, LogicalModelID: logical, Provider: provider, RuntimeModel: runtime,
		UpstreamModel: upstream, Enabled: true,
		QuotaBucketKey: routeQuotaBucket(id, logical, provider), QuotaCosts: routeQuotaCosts(id, provider),
		Capabilities: capabilityJSON(profiles...),
	}
}

func routeQuotaCosts(routeID, provider string) datatypes.JSON {
	policy := QuotaCostPolicy{Mode: "unknown", Unit: "credits"}
	switch {
	case provider == "custom":
		policy = QuotaCostPolicy{Mode: "unmetered"}
	case provider == "byteplus" && strings.HasPrefix(routeID, "image."):
		policy = QuotaCostPolicy{Mode: "metered", Unit: "points", Calculator: "byteplus_image"}
	case provider == "oreate" && strings.HasPrefix(routeID, "video."):
		policy = QuotaCostPolicy{Mode: "metered", Unit: "points", Calculator: "oreate_seedance"}
	case provider == "runway" && strings.HasPrefix(routeID, "video."):
		policy = QuotaCostPolicy{Mode: "metered", Unit: "credits", Calculator: "per_second", PerSecond: 5}
	case provider == "chatgpt" && strings.HasPrefix(routeID, "image."):
		policy = QuotaCostPolicy{Mode: "metered", Unit: "generations", Calculator: "fixed", Value: 1}
	}
	return quotaCostJSON(policy)
}

func routeQuotaBucket(routeID, logicalID, provider string) string {
	switch provider {
	case "byteplus":
		return "byteplus.computing_points"
	case "chatgpt":
		if strings.HasPrefix(routeID, "image.") {
			return "chatgpt.image"
		}
		return "chatgpt.text"
	case "adobe":
		// Adobe exposes one account-level Firefly allowance. Image and video
		// routes must share it or the same upstream credits become spendable
		// twice in the scheduler.
		return "adobe.credits"
	case "runway":
		return "runway.credits"
	case "grok":
		if strings.HasPrefix(routeID, "text.") {
			return "grok.text"
		}
		return "grok.media"
	case "oreate":
		return "oreate.points"
	case "custom":
		return "custom." + logicalID
	default:
		return provider + "." + logicalID
	}
}

func textProfile() CapabilityProfile {
	return CapabilityProfile{Operations: []string{"completion"}}
}

func imageProfile(ratios, resolutions []string, maxRefs int) CapabilityProfile {
	ops := []string{"generation"}
	if maxRefs > 0 {
		ops = append(ops, "edit")
	}
	return CapabilityProfile{Operations: ops, Ratios: ratios, Resolutions: resolutions, MaxReferenceImages: maxRefs, ReferenceMode: "asset", MaxImagesPerRequest: 1}
}

func videoProfile(ratios, resolutions, durations []string, maxImages, maxVideos, maxAudios, maxMedia int, audio bool, mode string) CapabilityProfile {
	return CapabilityProfile{Operations: []string{"generation"}, Ratios: ratios, Resolutions: resolutions, Durations: durations,
		MaxReferenceImages: maxImages, MaxReferenceVideos: maxVideos, MaxReferenceAudios: maxAudios,
		MaxReferenceMedia: maxMedia, SupportsAudioOutput: audio, ReferenceMode: mode}
}

func secondsRange(first, last int) []string {
	out := make([]string, 0, last-first+1)
	for i := first; i <= last; i++ {
		out = append(out, strconv.Itoa(i)+"s")
	}
	return out
}

// CanonicalRoutingCatalog is the closed public model set. Runtime and upstream
// ids stay on ModelRoute. Multi-provider products use `{provider}-{product}`
// public IDs so clients select a channel explicitly; ChatGPT's GPT Image 2 uses
// the unprefixed `gpt-image-2` ID. Oreate, Runway, and Custom are not public.
func CanonicalRoutingCatalog() []CanonicalModelDefinition {
	commonImageRatios := []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	chatGPTImageRatios := []string{"1:1", "16:9", "9:16", "4:3", "3:4", "3:2"}
	adobeWideRatios := []string{"1:1", "5:4", "9:16", "21:9", "16:9", "4:3", "3:2", "4:5", "3:4", "2:3"}
	return []CanonicalModelDefinition{
		{Model: LogicalModel{ID: "gpt-6", Kind: "text", Name: "GPT-6", Enabled: true}, Routes: []ModelRoute{
			route("text.gpt-6.chatgpt", "gpt-6", "chatgpt", "gpt-6", "gpt-6", textProfile()),
		}},
		{Model: LogicalModel{ID: "gpt-6-t-mini", Kind: "text", Name: "Thinking mini", Enabled: true}, Routes: []ModelRoute{
			route("text.gpt-6-t-mini.chatgpt", "gpt-6-t-mini", "chatgpt", "gpt-6-t-mini", "gpt-6-t-mini", textProfile()),
		}},
		{Model: LogicalModel{ID: "grok-4.5", Kind: "text", Name: "Grok 4.5", Enabled: true}, Routes: []ModelRoute{
			route("text.grok-4.5.grok", "grok-4.5", "grok", "grok-4.5", "grok-4.5", textProfile()),
		}},
		{Model: LogicalModel{ID: "grok-chat-fast", Kind: "text", Name: "Grok Chat Fast", Enabled: true}, Routes: []ModelRoute{
			route("text.grok-chat-fast.grok", "grok-chat-fast", "grok", "grok-chat-fast", "grok-chat-fast", textProfile()),
		}},

		{Model: LogicalModel{ID: "gpt-image-2", Kind: "image", Name: "GPT Image 2", Enabled: true}, Routes: []ModelRoute{
			route("image.gpt-image-2.chatgpt", "gpt-image-2", "chatgpt", "gpt-image-2", "gpt-image-2", imageProfile(chatGPTImageRatios, []string{"1K"}, 6)),
			route("image.gpt-image-2.byteplus", "gpt-image-2", "byteplus", "lumina-gpt-image-2", "6824519374061285743", imageProfile(commonImageRatios, []string{"1K", "2K", "4K"}, 14)),
			// Adobe free Firefly supply only exposes GPT Image 2 at 1K/2K.
			route("image.gpt-image-2.adobe", "gpt-image-2", "adobe", "firefly-gpt-image-2", "", imageProfile(adobeWideRatios, []string{"1K", "2K"}, 6)),
		}},
		// Adobe free Firefly also exposes Kling 4.0 at 720p / 8s (Auto + widescreen ratios).
		{Model: LogicalModel{ID: "kling-4.0", Kind: "video", Name: "Kling 4.0", Enabled: true}, Routes: []ModelRoute{
			route("video.kling-4.0.adobe", "kling-4.0", "adobe", "firefly-kling-4", "", videoProfile([]string{"auto", "21:9", "16:9", "1:1", "9:16"}, []string{"720p"}, []string{"8s"}, 1, 0, 0, 0, true, "frame")),
		}},
		{Model: LogicalModel{ID: "seedream-5.0-pro", Kind: "image", Name: "Seedream 5.0 Pro", Enabled: true}, Routes: []ModelRoute{
			route("image.seedream-5.0-pro.byteplus", "seedream-5.0-pro", "byteplus", "lumina-seedream-5.0-pro", "7657401949175693322", imageProfile(commonImageRatios, []string{"1K", "2K"}, 10)),
		}},
		{Model: LogicalModel{ID: "seedream-5.0-lite", Kind: "image", Name: "Seedream 5.0 Lite", Enabled: true}, Routes: []ModelRoute{
			route("image.seedream-5.0-lite.byteplus", "seedream-5.0-lite", "byteplus", "lumina-seedream-5.0-lite", "7604761017696141358", imageProfile(commonImageRatios, []string{"2K"}, 10)),
		}},
		{Model: LogicalModel{ID: "nano-banana-2", Kind: "image", Name: "Nano Banana 2", Enabled: true}, Routes: []ModelRoute{
			route("image.nano-banana-2.byteplus", "nano-banana-2", "byteplus", "lumina-nano-banana-2", "8162745039814627354", imageProfile(commonImageRatios, []string{"1K", "2K", "4K"}, 14)),
		}},
		{Model: LogicalModel{ID: "nano-banana-pro", Kind: "image", Name: "Nano Banana Pro", Enabled: true}, Routes: []ModelRoute{
			route("image.nano-banana-pro.byteplus", "nano-banana-pro", "byteplus", "lumina-nano-banana-pro", "8162745039814627353", imageProfile(commonImageRatios, []string{"1K", "2K", "4K"}, 14)),
		}},
		{Model: LogicalModel{ID: "grok-imagine-image", Kind: "image", Name: "Grok Imagine Image", Enabled: true}, Routes: []ModelRoute{
			route("image.grok-imagine-image.grok", "grok-imagine-image", "grok", "grok-imagine-image", "grok-imagine-image", imageProfile([]string{"2:3", "3:2", "1:1", "9:16", "16:9"}, []string{"1K"}, 0)),
		}},
		{Model: LogicalModel{ID: "grok-imagine-video", Kind: "video", Name: "Grok Imagine Video", Enabled: true}, Routes: []ModelRoute{
			route("video.grok-imagine-video.grok", "grok-imagine-video", "grok", "grok-video", "grok-imagine-video", videoProfile([]string{"2:3", "3:2", "1:1", "9:16", "16:9"}, []string{"720p"}, []string{"6s", "10s"}, 6, 0, 0, 0, false, "asset")),
		}},
	}
}

func IsCanonicalModelID(id string) bool {
	id = strings.TrimSpace(id)
	for _, definition := range CanonicalRoutingCatalog() {
		if definition.Model.ID == id {
			return true
		}
	}
	return false
}

func IsCanonicalRoute(route ModelRoute) bool {
	switch route.Provider {
	case "oreate", "runway", "custom", "dola":
		return false
	}
	for _, definition := range CanonicalRoutingCatalog() {
		for _, candidate := range definition.Routes {
			if candidate.ID == route.ID && candidate.LogicalModelID == route.LogicalModelID && candidate.Provider == route.Provider {
				return true
			}
		}
	}
	return false
}

func DecodeCapabilityProfiles(raw datatypes.JSON) []CapabilityProfile {
	var profiles []CapabilityProfile
	if len(raw) == 0 || json.Unmarshal(raw, &profiles) != nil {
		return nil
	}
	return profiles
}

func DecodeQuotaCostPolicy(raw datatypes.JSON) (QuotaCostPolicy, bool) {
	var policy QuotaCostPolicy
	if len(raw) == 0 || json.Unmarshal(raw, &policy) != nil {
		return policy, false
	}
	policy.Mode = strings.ToLower(strings.TrimSpace(policy.Mode))
	policy.Calculator = strings.ToLower(strings.TrimSpace(policy.Calculator))
	return policy, policy.Mode == "metered" || policy.Mode == "unknown" || policy.Mode == "unmetered"
}

func (r ModelRoute) Supports(req RouteRequirements) bool {
	for _, profile := range DecodeCapabilityProfiles(r.Capabilities) {
		if capabilitySupports(profile, req) {
			return true
		}
	}
	return false
}

// Supports reports whether this single profile accepts the request.
func (p CapabilityProfile) Supports(req RouteRequirements) bool { return capabilitySupports(p, req) }

func capabilitySupports(profile CapabilityProfile, req RouteRequirements) bool {
	contains := func(values []string, wanted string) bool {
		if strings.TrimSpace(wanted) == "" {
			return true
		}
		for _, value := range values {
			if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(wanted)) {
				return true
			}
		}
		return false
	}
	if !contains(profile.Operations, req.Operation) || !contains(profile.Ratios, req.Ratio) ||
		!contains(profile.Resolutions, req.Resolution) || !contains(profile.Durations, req.Duration) {
		return false
	}
	if req.ReferenceImages > profile.MaxReferenceImages || req.ReferenceVideos > profile.MaxReferenceVideos || req.ReferenceAudios > profile.MaxReferenceAudios {
		return false
	}
	if profile.MaxReferenceMedia > 0 && req.ReferenceImages+req.ReferenceVideos+req.ReferenceAudios > profile.MaxReferenceMedia {
		return false
	}
	if profile.RequiresReference && req.ReferenceImages+req.ReferenceVideos+req.ReferenceAudios == 0 {
		return false
	}
	return !req.GenerateAudio || profile.SupportsAudioOutput
}
