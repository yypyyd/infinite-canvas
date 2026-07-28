package model

type VideoProjectStatus string

const (
	VideoProjectStatusDraft     VideoProjectStatus = "draft"
	VideoProjectStatusVersioned VideoProjectStatus = "versioned"
)

type VideoTimelineSource struct {
	StorageKey string `json:"storageKey"`
	Kind       string `json:"kind"`
	SourceType string `json:"sourceType"`
}

type VideoTimelineTransition struct {
	Type       string `json:"type"`
	DurationMs int    `json:"durationMs"`
}

type VideoTimelineShot struct {
	ID               string                  `json:"id"`
	Source           VideoTimelineSource     `json:"source"`
	StartMs          int                     `json:"startMs"`
	DurationMs       int                     `json:"durationMs"`
	TrimStartMs      int                     `json:"trimStartMs"`
	CropMode         string                  `json:"cropMode"`
	TransitionToNext VideoTimelineTransition `json:"transitionToNext"`
}

type VideoTimelineSubtitle struct {
	ID        string `json:"id"`
	Text      string `json:"text"`
	StartMs   int    `json:"startMs"`
	EndMs     int    `json:"endMs"`
	Style     string `json:"style"`
	PositionY int    `json:"positionY"`
}

type VideoTimelineBGM struct {
	StorageKey     string `json:"storageKey"`
	Volume         int    `json:"volume"`
	Loop           bool   `json:"loop"`
	TrimStartMs    int    `json:"trimStartMs"`
	FadeInMs       int    `json:"fadeInMs"`
	FadeOutMs      int    `json:"fadeOutMs"`
	RightsConfirmed bool  `json:"rightsConfirmed"`
}

type VideoOutputSpec struct {
	Ratio      string `json:"ratio"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPS        int    `json:"fps"`
	Format     string `json:"format"`
	VideoCodec string `json:"videoCodec"`
	AudioCodec string `json:"audioCodec"`
}

type VideoTimeline struct {
	Shots     []VideoTimelineShot     `json:"shots"`
	Subtitles []VideoTimelineSubtitle `json:"subtitles"`
	BGM       *VideoTimelineBGM       `json:"bgm,omitempty"`
	Output    VideoOutputSpec         `json:"output"`
}

type VideoProject struct {
	ID                string             `json:"id" gorm:"primaryKey"`
	OrganizationID    string             `json:"organizationId" gorm:"not null;index"`
	ProductID         string             `json:"productId" gorm:"index"`
	SKUID             string             `json:"skuId" gorm:"column:sku_id;index"`
	Name              string             `json:"name" gorm:"index"`
	Description       string             `json:"description" gorm:"type:text"`
	DraftTimelineJSON string             `json:"-" gorm:"type:text"`
	DraftTimeline     VideoTimeline      `json:"draftTimeline" gorm:"-"`
	Status            VideoProjectStatus `json:"status" gorm:"index"`
	Version           int64              `json:"version"`
	CurrentVersion    int                `json:"currentVersion"`
	CreatedBy         string             `json:"createdBy" gorm:"index"`
	CreatedAt         string             `json:"createdAt"`
	UpdatedAt         string             `json:"updatedAt"`
}

type VideoProjectVersion struct {
	ID             string        `json:"id" gorm:"primaryKey"`
	OrganizationID string        `json:"organizationId" gorm:"not null;index;uniqueIndex:idx_video_project_version,priority:1"`
	ProjectID      string        `json:"projectId" gorm:"index;uniqueIndex:idx_video_project_version,priority:2"`
	Version        int           `json:"version" gorm:"uniqueIndex:idx_video_project_version,priority:3"`
	TimelineJSON   string        `json:"-" gorm:"type:text"`
	Timeline       VideoTimeline `json:"timeline" gorm:"-"`
	OutputSpecJSON string        `json:"-" gorm:"type:text"`
	OutputSpec     VideoOutputSpec `json:"outputSpec" gorm:"-"`
	CreatedBy      string        `json:"createdBy" gorm:"index"`
	CreatedAt      string        `json:"createdAt"`
}

type VideoProjectList struct { Items []VideoProject `json:"items"`; Total int `json:"total"` }
type SaveVideoProjectInput struct { Name string `json:"name"`; Description string `json:"description"`; ProductID string `json:"productId"`; SKUID string `json:"skuId"`; ExpectedVersion int64 `json:"expectedVersion"`; Timeline VideoTimeline `json:"timeline"` }
type VideoPreflight struct { CanFreeze bool `json:"canFreeze"`; DurationMs int `json:"durationMs"`; Issues []ProductionPreflightIssue `json:"issues"`; Output VideoOutputSpec `json:"output"` }
type CreateVideoProjectVersionInput struct { ExpectedVersion int64 `json:"expectedVersion"` }
