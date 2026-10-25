package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	videoRenderModelID          = "commerce-video-render"
	videoRenderDefaultCreditsPS = 1 // credits per output second when no pricing rule
	videoRenderLeaseTTL         = 10 * time.Minute
)

func EstimateVideoRenderCredits(durationMs int, output model.VideoOutputSpec) int {
	seconds := int(math.Ceil(float64(durationMs) / 1000.0))
	if seconds < 1 {
		seconds = 1
	}
	tier := "1080p"
	unit, quantity := VideoBillingUnitAndQuantity(videoRenderModelID, tier, tier, seconds)
	settings, err := repository.GetSettings()
	if err != nil {
		return seconds * videoRenderDefaultCreditsPS
	}
	rules := normalizePublicSetting(settings.Public).ModelChannel.PricingRules
	rule, ok := selectPricingRule(rules, normalizePricingRequest(PricingRequest{
		Model: videoRenderModelID, Modality: "video", Operation: "generation", Unit: unit, ResolutionTier: tier, Quantity: quantity,
	}))
	if !ok {
		return seconds * videoRenderDefaultCreditsPS
	}
	credits := calculateRuleCredits(rule, quantity, 1)
	if credits < 1 {
		credits = seconds * videoRenderDefaultCreditsPS
	}
	_ = output
	return credits
}

func ListVideoRenderJobs(user model.AuthUser, q model.Query) (model.VideoRenderJobList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoRenderJobList{}, err
	}
	q.Normalize()
	items, total, err := repository.ListVideoRenderJobs(organization.ID, q)
	if err != nil {
		return model.VideoRenderJobList{}, err
	}
	return model.VideoRenderJobList{Items: items, Total: int(total)}, nil
}

func GetVideoRenderJob(user model.AuthUser, id string) (model.VideoRenderJob, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	job, ok, err := repository.GetVideoRenderJob(organization.ID, strings.TrimSpace(id))
	if err != nil {
		return job, err
	}
	if !ok {
		return job, safeMessageError{message: "渲染任务不存在"}
	}
	return job, nil
}

func CreateVideoRenderJob(user model.AuthUser, projectID string, input model.CreateVideoRenderJobInput) (model.VideoRenderJob, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	if !canWriteCommerce(membership.Role) {
		return model.VideoRenderJob{}, safeMessageError{message: "没有视频渲染权限"}
	}
	projectID = strings.TrimSpace(projectID)
	requestID := strings.TrimSpace(input.RequestID)
	if projectID == "" || requestID == "" {
		return model.VideoRenderJob{}, safeMessageError{message: "请求参数无效"}
	}
	project, err := GetVideoProject(user, projectID)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	if input.ExpectedVersion > 0 && project.Version != input.ExpectedVersion {
		return model.VideoRenderJob{}, safeMessageError{message: "工程草稿版本已变化，请刷新后重试"}
	}
	versionNumber := input.VersionNumber
	if versionNumber <= 0 {
		versionNumber = project.CurrentVersion
	}
	if versionNumber <= 0 {
		return model.VideoRenderJob{}, safeMessageError{message: "请先冻结工程版本再提交渲染"}
	}
	version, err := GetVideoProjectVersion(user, projectID, versionNumber)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	preflight, _, err := validateVideoTimeline(organization.ID, version.Timeline, true)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	if !preflight.CanFreeze {
		return model.VideoRenderJob{}, safeMessageError{message: "冻结版本未通过预检，无法渲染"}
	}
	timelineJSON, err := json.Marshal(version.Timeline)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	outputJSON, err := json.Marshal(version.OutputSpec)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	estimated := EstimateVideoRenderCredits(preflight.DurationMs, version.OutputSpec)
	timestamp := now()
	job := model.VideoRenderJob{
		ID:               newID("video-render"),
		OrganizationID:   organization.ID,
		RequestID:        requestID,
		RequestHash:      repository.VideoRenderRequestHash(projectID, versionNumber, string(timelineJSON), string(outputJSON)),
		ProjectID:        projectID,
		ProjectVersionID: version.ID,
		VersionNumber:    versionNumber,
		Name:             project.Name,
		Status:           model.VideoRenderStatusQueued,
		Phase:            "queued",
		TimelineJSON:     string(timelineJSON),
		OutputSpecJSON:   string(outputJSON),
		DurationMs:       preflight.DurationMs,
		EstimatedCredits: estimated,
		ReviewStatus:     model.VideoRenderReviewPending,
		CreatedBy:        user.ID,
		CreatedAt:        timestamp,
		UpdatedAt:        timestamp,
	}
	saved, err := repository.CreateVideoRenderJob(job)
	if err != nil {
		if errors.Is(err, repository.ErrVideoRenderRequestConflict) {
			return model.VideoRenderJob{}, safeMessageError{message: "相同请求已存在且内容不一致"}
		}
		if errors.Is(err, repository.ErrVideoRenderQueueFull) {
			return model.VideoRenderJob{}, safeMessageError{message: "企业渲染队列已满，请稍后重试"}
		}
		if errors.Is(err, repository.ErrInsufficientUserCredits) || errors.Is(err, repository.ErrInsufficientOrganizationCredits) {
			return model.VideoRenderJob{}, safeMessageError{message: "算力不足，无法提交渲染"}
		}
		if errors.Is(err, repository.ErrOrganizationCreditBudgetExceeded) {
			return model.VideoRenderJob{}, safeMessageError{message: "本月企业算力预算不足"}
		}
		return model.VideoRenderJob{}, err
	}
	return saved, nil
}

func CancelVideoRenderJob(user model.AuthUser, id string) error {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return err
	}
	if !canWriteCommerce(membership.Role) {
		return safeMessageError{message: "没有视频渲染权限"}
	}
	if err := repository.CancelVideoRenderJob(organization.ID, strings.TrimSpace(id), now()); err != nil {
		if errors.Is(err, repository.ErrVideoRenderStateConflict) {
			return safeMessageError{message: "当前状态不可取消"}
		}
		return err
	}
	return nil
}

func ReviewVideoRenderJob(user model.AuthUser, id string, input model.ReviewVideoRenderJobInput) (model.VideoRenderJob, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoRenderJob{}, err
	}
	if !canWriteCommerce(membership.Role) {
		return model.VideoRenderJob{}, safeMessageError{message: "没有审核权限"}
	}
	if input.Status != model.VideoRenderReviewApproved && input.Status != model.VideoRenderReviewRejected {
		return model.VideoRenderJob{}, safeMessageError{message: "审核状态无效"}
	}
	if err := repository.ReviewVideoRenderJob(organization.ID, strings.TrimSpace(id), input.Status, now()); err != nil {
		if errors.Is(err, repository.ErrVideoRenderStateConflict) {
			return model.VideoRenderJob{}, safeMessageError{message: "当前状态不可审核"}
		}
		return model.VideoRenderJob{}, err
	}
	return GetVideoRenderJob(user, id)
}

func VideoRenderResultURL(user model.AuthUser, id string) (string, error) {
	job, err := GetVideoRenderJob(user, id)
	if err != nil {
		return "", err
	}
	if job.ResultStorageKey == "" {
		return "", safeMessageError{message: "成片尚未就绪"}
	}
	if job.Status != model.VideoRenderStatusPendingReview && job.Status != model.VideoRenderStatusCompleted {
		return "", safeMessageError{message: "成片尚未通过审核"}
	}
	url, ok := organizationFileURL(job.OrganizationID, job.ResultStorageKey, "", 15*time.Minute)
	if !ok {
		return "", safeMessageError{message: "无法生成下载链接"}
	}
	return url, nil
}

func ClaimVideoRenderJob(maxTenantRunning int) (model.VideoRenderJob, bool, error) {
	claimedAt := time.Now().UTC()
	return repository.ClaimNextVideoRenderJob(
		claimedAt.Format(timestampLayout),
		claimedAt.Add(videoRenderLeaseTTL).Format(timestampLayout),
		newID("lease"),
		maxTenantRunning,
	)
}

func RenewVideoRenderJobLease(job model.VideoRenderJob) error {
	claimedAt := time.Now().UTC()
	return repository.RenewVideoRenderLease(job.OrganizationID, job.ID, job.LeaseToken, claimedAt.Add(videoRenderLeaseTTL).Format(timestampLayout), claimedAt.Format(timestampLayout))
}

func videoRenderNextAttemptAt(attempt int) string {
	delay := time.Duration(math.Min(float64(30*attempt*attempt), 600)) * time.Second
	return time.Now().UTC().Add(delay).Format(timestampLayout)
}

func parseVideoRenderTimeline(job model.VideoRenderJob) (model.VideoTimeline, error) {
	var timeline model.VideoTimeline
	if err := json.Unmarshal([]byte(job.TimelineJSON), &timeline); err != nil {
		return timeline, fmt.Errorf("timeline json: %w", err)
	}
	return timeline, nil
}
