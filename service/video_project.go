package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func defaultVideoTimeline() model.VideoTimeline {
	return model.VideoTimeline{Shots: []model.VideoTimelineShot{}, Subtitles: []model.VideoTimelineSubtitle{}, Output: model.VideoOutputSpec{Ratio: "16:9", Width: 1920, Height: 1080, FPS: 30, Format: "mp4", VideoCodec: "h264", AudioCodec: "aac"}}
}

func ListVideoProjects(user model.AuthUser, q model.Query) (model.VideoProjectList, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoProjectList{}, err
	}
	items, total, err := repository.ListVideoProjects(organization.ID, q)
	if err != nil {
		return model.VideoProjectList{}, err
	}
	for index := range items {
		items[index], err = hydrateVideoProjectTimeline(items[index])
		if err != nil {
			return model.VideoProjectList{}, err
		}
	}
	return model.VideoProjectList{Items: items, Total: int(total)}, nil
}

func hydrateVideoProjectTimeline(item model.VideoProject) (model.VideoProject, error) {
	if err := json.Unmarshal([]byte(item.DraftTimelineJSON), &item.DraftTimeline); err != nil {
		return item, errors.New("video project timeline is invalid")
	}
	return item, nil
}

func GetVideoProject(user model.AuthUser, id string) (model.VideoProject, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoProject{}, err
	}
	item, ok, err := repository.GetVideoProject(organization.ID, strings.TrimSpace(id))
	if err != nil {
		return item, err
	}
	if !ok {
		return item, safeMessageError{message: "视频工程不存在"}
	}
	return hydrateVideoProjectTimeline(item)
}

func SaveVideoProject(user model.AuthUser, id string, input model.SaveVideoProjectInput) (model.VideoProject, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoProject{}, err
	}
	if !canWriteCommerce(membership.Role) {
		return model.VideoProject{}, safeMessageError{message: "没有视频工程编辑权限"}
	}
	id = strings.TrimSpace(id)
	if (id == "" && input.ExpectedVersion != 0) || (id != "" && input.ExpectedVersion <= 0) {
		return model.VideoProject{}, safeMessageError{message: "数据版本无效，请刷新后重试"}
	}
	input.Name, input.Description, input.ProductID, input.SKUID = strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), strings.TrimSpace(input.ProductID), strings.TrimSpace(input.SKUID)
	if input.Name == "" || len([]rune(input.Name)) > 200 {
		return model.VideoProject{}, safeMessageError{message: "工程名称不能为空或过长"}
	}
	if len([]rune(input.Description)) > 2000 {
		return model.VideoProject{}, safeMessageError{message: "工程描述不能超过 2000 字"}
	}
	if input.ProductID != "" {
		if _, ok, err := repository.GetProduct(organization.ID, input.ProductID); err != nil || !ok {
			if err != nil {
				return model.VideoProject{}, err
			}
			return model.VideoProject{}, safeMessageError{message: "关联商品不存在"}
		}
	}
	if input.SKUID != "" {
		sku, ok, err := repository.GetProductSKU(organization.ID, input.SKUID)
		if err != nil || !ok || sku.ProductID != input.ProductID {
			if err != nil {
				return model.VideoProject{}, err
			}
			return model.VideoProject{}, safeMessageError{message: "关联 SKU 不属于所选商品"}
		}
	}
	if id == "" && len(input.Timeline.Shots) == 0 {
		input.Timeline = defaultVideoTimeline()
	}
	preflight, storageKeys, err := validateVideoTimeline(organization.ID, input.Timeline, false)
	if err != nil {
		return model.VideoProject{}, err
	}
	_ = preflight
	timelineJSON, err := json.Marshal(input.Timeline)
	if err != nil || len(timelineJSON) > 1<<20 {
		return model.VideoProject{}, safeMessageError{message: "视频工程时间线无效或过大"}
	}
	timestamp := now()
	item := model.VideoProject{ID: id, OrganizationID: organization.ID, ProductID: input.ProductID, SKUID: input.SKUID, Name: input.Name, Description: input.Description, DraftTimelineJSON: string(timelineJSON), DraftTimeline: input.Timeline, Status: model.VideoProjectStatusDraft, Version: input.ExpectedVersion + 1, CreatedBy: user.ID, CreatedAt: timestamp, UpdatedAt: timestamp}
	if item.ID == "" {
		item.ID, item.Version, input.ExpectedVersion = newID("video-project"), 1, 0
	}
	action := "video_project.update"
	if input.ExpectedVersion == 0 {
		action = "video_project.create"
	}
	item, err = repository.SaveVideoProject(item, input.ExpectedVersion, storageKeys, newAuditLog(user.ID, organization.ID, action, "video_project", item.ID, item.Name, timestamp))
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return item, safeMessageError{message: "视频工程已被其他成员修改，请刷新后重试"}
	}
	if errors.Is(err, repository.ErrWorkspaceFileMissing) {
		return item, safeMessageError{message: "工程引用的素材不存在或无权访问"}
	}
	return item, err
}

func PreflightVideoProject(user model.AuthUser, id string) (model.VideoPreflight, error) {
	item, err := GetVideoProject(user, id)
	if err != nil {
		return model.VideoPreflight{}, err
	}
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoPreflight{}, err
	}
	result, _, err := validateVideoTimeline(organization.ID, item.DraftTimeline, true)
	return result, err
}

func ListVideoProjectVersions(user model.AuthUser, id string) ([]model.VideoProjectVersion, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return nil, err
	}
	id = strings.TrimSpace(id)
	if _, ok, err := repository.GetVideoProject(organization.ID, id); err != nil || !ok {
		if err != nil {
			return nil, err
		}
		return nil, safeMessageError{message: "视频工程不存在"}
	}
	return repository.ListVideoProjectVersions(organization.ID, id)
}

func GetVideoProjectVersion(user model.AuthUser, id string, versionNumber int) (model.VideoProjectVersion, error) {
	organization, _, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoProjectVersion{}, err
	}
	id = strings.TrimSpace(id)
	if _, ok, err := repository.GetVideoProject(organization.ID, id); err != nil || !ok {
		if err != nil {
			return model.VideoProjectVersion{}, err
		}
		return model.VideoProjectVersion{}, safeMessageError{message: "视频工程不存在"}
	}
	item, ok, err := repository.GetVideoProjectVersion(organization.ID, id, versionNumber)
	if err != nil {
		return item, err
	}
	if !ok {
		return item, safeMessageError{message: "视频工程版本不存在"}
	}
	return item, nil
}

func CreateVideoProjectVersion(user model.AuthUser, id string, expectedVersion int64) (model.VideoProjectVersion, error) {
	organization, membership, err := EnsureOrganization(user)
	if err != nil {
		return model.VideoProjectVersion{}, err
	}
	if !canWriteCommerce(membership.Role) {
		return model.VideoProjectVersion{}, safeMessageError{message: "没有冻结工程版本权限"}
	}
	if expectedVersion <= 0 {
		return model.VideoProjectVersion{}, safeMessageError{message: "数据版本无效，请刷新后重试"}
	}
	project, ok, err := repository.GetVideoProject(organization.ID, strings.TrimSpace(id))
	if err != nil || !ok {
		if err != nil {
			return model.VideoProjectVersion{}, err
		}
		return model.VideoProjectVersion{}, safeMessageError{message: "视频工程不存在"}
	}
	if project.Version != expectedVersion {
		return model.VideoProjectVersion{}, safeMessageError{message: "视频工程已变化，请刷新后重试"}
	}
	if err := json.Unmarshal([]byte(project.DraftTimelineJSON), &project.DraftTimeline); err != nil {
		return model.VideoProjectVersion{}, errors.New("video project timeline is invalid")
	}
	preflight, storageKeys, err := validateVideoTimeline(organization.ID, project.DraftTimeline, true)
	if err != nil {
		return model.VideoProjectVersion{}, err
	}
	if !preflight.CanFreeze {
		return model.VideoProjectVersion{}, safeMessageError{message: "工程预检未通过，请先修复素材或时间线问题"}
	}
	timestamp := now()
	version := model.VideoProjectVersion{ID: newID("video-project-version"), OrganizationID: organization.ID, ProjectID: project.ID, CreatedBy: user.ID, CreatedAt: timestamp}
	version, err = repository.CreateVideoProjectVersion(project, project.DraftTimelineJSON, version, storageKeys, newAuditLog(user.ID, organization.ID, "video_project.version", "video_project_version", version.ID, map[string]any{"projectId": project.ID}, timestamp))
	if errors.Is(err, repository.ErrCommerceVersionConflict) {
		return version, safeMessageError{message: "视频工程已变化，请刷新后重试"}
	}
	if errors.Is(err, repository.ErrWorkspaceFileMissing) {
		return version, safeMessageError{message: "工程引用的素材不存在或无权访问"}
	}
	return version, err
}

func validateVideoTimeline(organizationID string, timeline model.VideoTimeline, strict bool) (model.VideoPreflight, []string, error) {
	issues := []model.ProductionPreflightIssue{}
	storageKeys := []string{}
	storageKeySet := map[string]bool{}
	shotIDs := map[string]bool{}
	duration := 0
	if strict && (len(timeline.Shots) < 3 || len(timeline.Shots) > 100) {
		issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_COUNT", Message: "冻结版本需要 3 到 100 个镜头"})
	}
	for _, shot := range timeline.Shots {
		if shot.ID == "" || shotIDs[shot.ID] {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_ID", Message: "镜头编号不能为空且不能重复"})
		}
		shotIDs[shot.ID] = true
		if shot.DurationMs <= 0 || shot.DurationMs > 60000 || (shot.CropMode != "cover" && shot.CropMode != "contain") {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_DURATION", Message: "镜头时长或裁切方式无效"})
		}
		if shot.TransitionToNext.Type != "none" && shot.TransitionToNext.Type != "fade" && shot.TransitionToNext.Type != "cross_dissolve" {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "TRANSITION", Message: "镜头转场无效"})
		}
		if shot.TransitionToNext.DurationMs < 0 || shot.TransitionToNext.DurationMs > shot.DurationMs {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "TRANSITION_DURATION", Message: "转场时长不能超过镜头时长"})
		}
		validSourceType := shot.Source.SourceType == "sku" || shot.Source.SourceType == "asset" || shot.Source.SourceType == "upload" || shot.Source.SourceType == "generated"
		if shot.Source.StorageKey == "" || (shot.Source.Kind != "image" && shot.Source.Kind != "video") || !validSourceType {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_SOURCE", Message: "镜头素材无效"})
		} else if !storageKeySet[shot.Source.StorageKey] {
			storageKeys = append(storageKeys, shot.Source.StorageKey)
			storageKeySet[shot.Source.StorageKey] = true
		}
		if shot.StartMs < 0 || shot.TrimStartMs < 0 {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_RANGE", Message: "镜头开始时间和裁切起点不能为负数"})
		}
		if end := shot.StartMs + shot.DurationMs; end > duration {
			duration = end
		}
	}
	for _, subtitle := range timeline.Subtitles {
		if subtitle.Text == "" || subtitle.StartMs < 0 || subtitle.EndMs <= subtitle.StartMs || subtitle.EndMs > duration || subtitle.PositionY < 10 || subtitle.PositionY > 90 {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SUBTITLE_RANGE", Message: "字幕文本、时间或安全区位置无效"})
		}
	}
	if timeline.BGM != nil {
		if timeline.BGM.StorageKey == "" || !timeline.BGM.RightsConfirmed {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "BGM_RIGHTS", Message: "背景音乐必须引用企业音频并确认使用权"})
		} else if !storageKeySet[timeline.BGM.StorageKey] {
			storageKeys = append(storageKeys, timeline.BGM.StorageKey)
			storageKeySet[timeline.BGM.StorageKey] = true
		}
		if timeline.BGM.Volume < 0 || timeline.BGM.Volume > 100 {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "BGM_VOLUME", Message: "背景音乐音量必须在 0 到 100 之间"})
		}
	}
	validOutput := (timeline.Output.Ratio == "16:9" && timeline.Output.Width == 1920 && timeline.Output.Height == 1080) || (timeline.Output.Ratio == "9:16" && timeline.Output.Width == 1080 && timeline.Output.Height == 1920) || (timeline.Output.Ratio == "1:1" && timeline.Output.Width == 1080 && timeline.Output.Height == 1080)
	if !validOutput || timeline.Output.FPS != 30 || timeline.Output.Format != "mp4" || timeline.Output.VideoCodec != "h264" || timeline.Output.AudioCodec != "aac" {
		issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "OUTPUT_SPEC", Message: "输出规格必须为支持的 1080p、30fps 工程契约"})
	}
	if strict && (duration < 15000 || duration > 60000) {
		issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "TOTAL_DURATION", Message: "冻结版本总时长必须在 15 到 60 秒之间"})
	}
	for _, key := range storageKeys {
		file, ok, err := repository.GetUserFile(organizationID, key)
		if err != nil {
			return model.VideoPreflight{}, nil, err
		}
		if !ok {
			issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "FILE_MISSING", Field: key, Message: "工程素材不存在或不属于当前企业"})
			continue
		}
		if timeline.BGM != nil && key == timeline.BGM.StorageKey {
			if !strings.HasPrefix(file.MimeType, "audio/") {
				issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "BGM_MIME", Message: "背景音乐必须是音频文件"})
			}
			continue
		}
		for _, shot := range timeline.Shots {
			if shot.Source.StorageKey == key && !strings.HasPrefix(file.MimeType, shot.Source.Kind+"/") {
				issues = append(issues, model.ProductionPreflightIssue{Severity: "error", Code: "SHOT_MIME", Field: key, Message: "镜头素材类型与声明不一致"})
				break
			}
		}
	}
	return model.VideoPreflight{CanFreeze: len(issues) == 0, DurationMs: duration, Issues: issues, Output: timeline.Output}, storageKeys, nil
}
