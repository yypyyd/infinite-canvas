package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const (
	agentInspectionFrameCount   = 6
	agentInspectionFrameTimeout = time.Minute
)

var agentVideoDurationPattern = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)

func agentUnavailableInspection(summary string) AgentToolInspection {
	return AgentToolInspection{Status: "unavailable", Summary: summary, Issues: []string{}}
}

// inspectAgentImages 用视觉模型对照目标验收图片；读不到图片或模型不可用时返回“验收暂不可用”，不让整轮失败。
func inspectAgentImages(ctx context.Context, user model.AuthUser, run model.AgentRun, steps []model.AgentStep, callID string, input imageInspectArguments) AgentToolInspection {
	_, nodes, err := agentRunCanvas(run, steps)
	if err != nil {
		return agentUnavailableInspection("待验收图片暂时无法读取")
	}
	images := make([]string, 0, len(input.NodeIDs))
	for _, id := range input.NodeIDs {
		node := nodes[id]
		if node.Type != "image" {
			return agentUnavailableInspection("待验收图片已不在当前画布中")
		}
		image, err := agentInspectionImage(ctx, user, node.StorageKey)
		if err != nil {
			return agentUnavailableInspection("待验收图片暂时无法读取")
		}
		images = append(images, image)
	}
	return requestAgentVisualInspection(ctx, user, run, callID, input.Criteria, fmt.Sprintf("附带的 %d 张图片", len(images)), images)
}

// inspectAgentVideo 按时间顺序抽取关键帧后交给视觉模型验收。
func inspectAgentVideo(ctx context.Context, user model.AuthUser, run model.AgentRun, steps []model.AgentStep, callID string, input videoInspectArguments) AgentToolInspection {
	_, nodes, err := agentRunCanvas(run, steps)
	if err != nil || nodes[input.NodeID].Type != "video" {
		return agentUnavailableInspection("待验收视频已不在当前画布中")
	}
	frames, err := agentVideoKeyFrames(ctx, run.OrganizationID, nodes[input.NodeID].StorageKey)
	if err != nil {
		logWorkerError("agent_tool", "video_frames_failed", err, "run_id", run.ID, "call_id", callID)
		return agentUnavailableInspection("视频关键帧提取失败")
	}
	return requestAgentVisualInspection(ctx, user, run, callID, input.Criteria, fmt.Sprintf("按时间顺序提取的 %d 张视频关键帧", len(frames)), frames)
}

func requestAgentVisualInspection(ctx context.Context, user model.AuthUser, run model.AgentRun, callID, criteria, source string, images []string) AgentToolInspection {
	content := []any{map[string]any{"type": "text", "text": "你是严格的媒体质量验收员。请查看" + source + "，并只依据可见内容按以下标准验收：\n" + criteria + "\n\n只输出一个合法 JSON 对象，不要 Markdown、代码块或额外文字。通过时输出 {\"status\":\"passed\",\"summary\":\"简洁中文结论\",\"issues\":[]}；存在影响目标达成的问题时输出 {\"status\":\"needs_revision\",\"summary\":\"简洁中文结论\",\"issues\":[\"具体可见问题\"],\"revisedPrompt\":\"一段可直接重新生成的完整提示词\"}。issues 最多 6 条；revisedPrompt 必须保留原目标并明确修正全部问题，不得臆造不可见事实。"}}
	for _, image := range images {
		content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": image}})
	}
	body, err := json.Marshal(map[string]any{"model": run.Model, "messages": []any{map[string]any{"role": "user", "content": content}}})
	if err != nil {
		return agentUnavailableInspection("视觉模型调用或结果解析失败")
	}
	select {
	case <-ctx.Done():
		return agentUnavailableInspection("视觉模型响应超时")
	case response := <-requestAgentAI(ctx, user, "/chat/completions", "application/json", "agent-inspect:"+run.ID+":"+callID, body):
		message, err := parseAgentCompletionJSON(response)
		if err != nil {
			return agentUnavailableInspection("视觉模型调用或结果解析失败")
		}
		inspection, ok := parseAgentMediaInspection(message.Content)
		if !ok {
			return agentUnavailableInspection("视觉模型调用或结果解析失败")
		}
		return inspection
	}
}

// parseAgentMediaInspection 解析视觉模型给出的验收结论，并整理成工具结果允许的形状；结论缺失或自相矛盾时返回 false。
func parseAgentMediaInspection(answer string) (AgentToolInspection, bool) {
	// 模型常把 JSON 包在代码块或说明文字里，只取最外层的对象。
	start, end := strings.Index(answer, "{"), strings.LastIndex(answer, "}")
	var inspection AgentToolInspection
	if start < 0 || end < start || json.Unmarshal([]byte(answer[start:end+1]), &inspection) != nil {
		return inspection, false
	}
	inspection.Status, inspection.Summary = strings.TrimSpace(inspection.Status), truncateAgentRunes(strings.TrimSpace(inspection.Summary), 1000)
	inspection.RevisedPrompt = truncateAgentRunes(strings.TrimSpace(inspection.RevisedPrompt), 8000)
	issues := []string{}
	for _, issue := range inspection.Issues {
		if issue = truncateAgentRunes(strings.TrimSpace(issue), 300); issue != "" && len(issues) < 6 {
			issues = append(issues, issue)
		}
	}
	inspection.Issues = issues
	switch inspection.Status {
	case "passed", "unavailable":
		inspection.Issues, inspection.RevisedPrompt = []string{}, ""
	case "needs_revision":
		if len(issues) == 0 || inspection.RevisedPrompt == "" {
			return inspection, false
		}
	default:
		return inspection, false
	}
	return inspection, inspection.Summary != ""
}

// agentInspectionImage 读取一张待验收图片的预览图（与页面查看大图用的 preview 规格一致），返回 data URL。
func agentInspectionImage(ctx context.Context, user model.AuthUser, storageKey string) (string, error) {
	if storageKey == "" {
		return "", errors.New("inspection image is not stored")
	}
	if file, mimeType, _, ok := OpenLocalUserWorkspaceImageVariant(user, storageKey, "preview"); ok {
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, agentReferenceImageBytes))
		if err != nil || len(data) == 0 {
			return "", errors.New("inspection image is unreadable")
		}
		return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
	}
	data, mimeType, err := readOrganizationFile(ctx, user.OrganizationID, storageKey, "preview", agentReferenceImageBytes)
	if err != nil {
		return "", err
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

// agentVideoKeyFrames 用 ffmpeg 按时间顺序抽取关键帧并返回 data URL；本机没有 ffmpeg 或视频读不出来时返回错误。
func agentVideoKeyFrames(ctx context.Context, organizationID, storageKey string) ([]string, error) {
	item, ok, err := repository.GetUserFile(organizationID, storageKey)
	if err != nil || !ok {
		return nil, errors.New("inspection video is not stored")
	}
	source := ""
	if effectiveStorageDriver(item.StorageDriver) == "local" {
		setting, err := currentUserStorageSetting()
		if err != nil {
			return nil, err
		}
		if source, err = localStorageObjectPath(setting, item.ObjectKey); err != nil {
			return nil, err
		}
	} else if source, ok = signedOrganizationFileURL(item, "", 30*time.Minute); !ok {
		return nil, errors.New("inspection video URL is unavailable")
	}
	// 不带输出地运行 ffmpeg 只会打印媒体信息并以非零状态退出，这里只读其中的时长。
	probeCtx, cancelProbe := context.WithTimeout(ctx, agentInspectionFrameTimeout)
	info, _ := exec.CommandContext(probeCtx, ffmpegBinary(), "-hide_banner", "-i", source).CombinedOutput()
	cancelProbe()
	match := agentVideoDurationPattern.FindSubmatch(info)
	if match == nil {
		return nil, fmt.Errorf("video duration is unavailable: %s", truncateAgentRunes(strings.TrimSpace(string(info)), 200))
	}
	hours, _ := strconv.ParseFloat(string(match[1]), 64)
	minutes, _ := strconv.ParseFloat(string(match[2]), 64)
	seconds, _ := strconv.ParseFloat(string(match[3]), 64)
	duration := hours*3600 + minutes*60 + seconds
	if duration <= 0 {
		return nil, errors.New("video duration is invalid")
	}
	frames := make([]string, 0, agentInspectionFrameCount)
	for index := range agentInspectionFrameCount {
		position := min(max(0, duration-0.05), duration*(0.05+float64(index)/float64(agentInspectionFrameCount-1)*0.9))
		frameCtx, cancelFrame := context.WithTimeout(ctx, agentInspectionFrameTimeout)
		frame, err := exec.CommandContext(frameCtx, ffmpegBinary(), "-hide_banner", "-loglevel", "error", "-ss", strconv.FormatFloat(position, 'f', 3, 64), "-i", source,
			"-an", "-frames:v", "1", "-vf", "scale='min(768,iw)':-2", "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "4", "pipe:1").Output()
		cancelFrame()
		if err != nil || len(frame) == 0 {
			return nil, fmt.Errorf("video frame %d extraction failed: %v", index+1, err)
		}
		frames = append(frames, "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(frame))
	}
	return frames, nil
}
