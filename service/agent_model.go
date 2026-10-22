package service

import (
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// agentFallbackModel 是用户默认文本模型不支持工具调用时画布 Agent 优先回退的模型。
const agentFallbackModel = "grok-4.6"

// agentModelSupportsTools 判断模型能否用于画布 Agent；未标注（nil）时视为支持以兼容旧配置。
func agentModelSupportsTools(item model.ModelDefinition) bool {
	return item.SupportsTools == nil || *item.SupportsTools
}

// resolveAgentModel 把请求的模型解析为可调用工具的文本模型。
func resolveAgentModel(requested string) (string, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return requested, err
	}
	return resolveAgentModelFrom(settings.Public.ModelChannel.Models, requested)
}

func resolveAgentModelFrom(models []model.ModelDefinition, requested string) (string, error) {
	found := false
	for _, item := range models {
		if item.ID == requested {
			found = true
			if agentModelSupportsTools(item) {
				return requested, nil
			}
			break
		}
	}
	if !found {
		// 目录外模型交给渠道选择按原逻辑处理。
		return requested, nil
	}
	usable := func(item model.ModelDefinition) bool {
		return item.Enabled && item.Modality == "text" && agentModelSupportsTools(item)
	}
	for _, item := range models {
		if item.ID == agentFallbackModel && usable(item) {
			return item.ID, nil
		}
	}
	candidates := make([]model.ModelDefinition, 0, len(models))
	for _, item := range models {
		if usable(item) {
			candidates = append(candidates, item)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		explicitI := candidates[i].SupportsTools != nil
		explicitJ := candidates[j].SupportsTools != nil
		if explicitI != explicitJ {
			return explicitI
		}
		return candidates[i].Sort < candidates[j].Sort
	})
	if len(candidates) > 0 {
		return candidates[0].ID, nil
	}
	return "", safeMessageError{message: "当前没有支持工具调用的文本模型，请联系管理员"}
}

// keepModelToolSupport 在后台保存未携带 supportsTools 时保留已保存的标记，避免旧版界面覆盖。
func keepModelToolSupport(next *model.Settings, saved model.Settings) {
	previous := map[string]bool{}
	for _, item := range saved.Public.ModelChannel.Models {
		if item.SupportsTools != nil {
			previous[item.ID] = *item.SupportsTools
		}
	}
	for i := range next.Public.ModelChannel.Models {
		item := &next.Public.ModelChannel.Models[i]
		if item.SupportsTools != nil {
			continue
		}
		if value, ok := previous[item.ID]; ok {
			copied := value
			item.SupportsTools = &copied
		}
	}
}

var agentFakeToolCallPattern = regexp.MustCompile(`(?i)<\s*function[\s_-]*call|<\s*tool_call|<\s*tool_name\b|<\s*invoke\b|<\s*(box|icon|row|title|entity)\b|_parameters\b|"tool_calls"\s*:|\b(?:tool\s+call|call(?:ing)?|invoke|invoking)\s+[a-z][a-z0-9]*[._][a-z0-9_.]*\s+with\b`)

// agentFakeToolCallFence 匹配伪工具调用常见的 ```xml / ```tool 代码块围栏。
var agentFakeToolCallFence = regexp.MustCompile("(?i)^```\\s*(xml|tool|tool_call|function)?\\s*$")

// stripAgentFakeToolCallText 在已有真实工具调用时去掉正文里伪造的调用文本行和空代码块围栏。
func stripAgentFakeToolCallText(content string) string {
	if !agentFakeToolCallPattern.MatchString(content) {
		return content
	}
	kept := make([]string, 0)
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if agentFakeToolCallPattern.MatchString(trimmed) || agentFakeToolCallFence.MatchString(trimmed) || strings.HasPrefix(trimmed, "</") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// agentStreamNeedsPlainRetry 判断回复是否把工具调用伪装成了文本或界面标记（且没有真实调用）。
func agentStreamNeedsPlainRetry(bodyValue map[string]any, message agentUpstreamMessage) bool {
	if _, hasTools := bodyValue["tools"]; !hasTools || len(message.ToolCalls) > 0 {
		return false
	}
	return agentFakeToolCallPattern.MatchString(message.Content)
}

const (
	// agentCompletionDeadline 是一次模型规划（含全部重试）的总时限。
	agentCompletionDeadline = 300 * time.Second
	// agentUpstreamAttemptTimeout 是单次上游请求的时限。
	agentUpstreamAttemptTimeout = 150 * time.Second
	// agentUpstreamMaxRetries 是连接类错误的最多重试次数。
	agentUpstreamMaxRetries = 2
	// agentUpstreamSlowDropThreshold / agentUpstreamLateDropLimit：在 [25s, 40s) 内断开的视为代理空闲掐断（实测约 30.5s），
	// 允许额外重试 agentUpstreamMaxLateRetries 次；40s 以后才断开的不再重试。
	agentUpstreamSlowDropThreshold = 25 * time.Second
	agentUpstreamLateDropLimit     = 40 * time.Second
	agentUpstreamMaxLateRetries    = 1
	// agentHistoryMaxMessages / agentHistoryMaxRunes 控制随请求发送的历史对话条数与每条长度。
	agentHistoryMaxMessages = 6
	agentHistoryMaxRunes    = 1500
)

// agentUpstreamRetryDelay 返回第 retry 次重试前的等待：0.5s、1s、2s……
func agentUpstreamRetryDelay(retry int) time.Duration {
	if retry < 0 {
		retry = 0
	}
	if retry > 4 {
		retry = 4
	}
	return time.Duration(500<<retry) * time.Millisecond
}

// agentUpstreamShouldRetry 判断连接类错误能否重试：25s 内的快速断连最多重试 agentUpstreamMaxRetries 次；
// 25–40s 的代理空闲掐断另计，最多重试 agentUpstreamMaxLateRetries 次（即最多两次晚断尝试）；其余不重试。
func agentUpstreamShouldRetry(err error, elapsed time.Duration, quickRetries, lateRetries int) (retry bool, late bool) {
	if !isAgentTransportError(err) {
		return false, false
	}
	if elapsed < agentUpstreamSlowDropThreshold {
		return quickRetries < agentUpstreamMaxRetries, false
	}
	if elapsed < agentUpstreamLateDropLimit {
		return lateRetries < agentUpstreamMaxLateRetries, true
	}
	return false, true
}

// agentNonStreamingHosts 是会把流式响应整段缓冲后才返回的代理：流式没有收益，且长时间无字节时易被掐断。
var agentNonStreamingHosts = []string{"cliproxy."}

// agentChannelStreams 判断渠道是否使用流式请求。
func agentChannelStreams(channel model.ModelChannel) bool {
	base := strings.ToLower(channel.BaseURL)
	for _, host := range agentNonStreamingHosts {
		if strings.Contains(base, host) {
			return false
		}
	}
	return true
}

func logAgentUpstreamAttempt(requestID, channel, modelName string, attempt int, streaming bool, bodyBytes, status int, elapsed time.Duration, err error) {
	fields := []any{"request_id", requestID, "channel", channel, "model", modelName, "attempt", attempt, "stream", streaming, "body_bytes", bodyBytes, "status", status, "duration_ms", elapsed.Milliseconds()}
	if err != nil {
		workerLogger.Warn("agent_upstream_attempt", append(fields, "error", truncateAgentRunes(err.Error(), 300))...)
		return
	}
	workerLogger.Info("agent_upstream_attempt", fields...)
}

// isAgentTransportError 判断是否为连接被对端关闭、重置等可安全重试的传输层错误。
func isAgentTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	text := strings.ToLower(err.Error())
	for _, marker := range []string{"connection reset", "unexpected eof", "eof", "broken pipe", "server closed", "connection refused", "http2: stream closed"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
