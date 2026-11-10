package service

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const agentStreamPublishInterval = 60 * time.Millisecond

type agentUpstreamToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type agentUpstreamToolCall struct {
	ID       string                    `json:"id"`
	Type     string                    `json:"type"`
	Function agentUpstreamToolFunction `json:"function"`
}

// agentLooseString 只取字符串值；上游把推理字段返回成对象或数组时按空处理，不让整条响应解析失败。
type agentLooseString string

func (value *agentLooseString) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*value = agentLooseString(text)
	}
	return nil
}

type agentUpstreamMessage struct {
	Content          string                  `json:"content"`
	ReasoningContent agentLooseString        `json:"reasoning_content"`
	Reasoning        agentLooseString        `json:"reasoning"`
	ToolCalls        []agentUpstreamToolCall `json:"tool_calls"`
}

// reasoningText 兼容 reasoning_content（DeepSeek、Qwen 等）与 reasoning（OpenRouter）两种字段。
func (message agentUpstreamMessage) reasoningText() string {
	if text := strings.TrimSpace(string(message.ReasoningContent)); text != "" {
		return text
	}
	return strings.TrimSpace(string(message.Reasoning))
}

// parseAgentCompletionJSON 解析非流式 chat/completions 响应。
func parseAgentCompletionJSON(body []byte) (agentUpstreamMessage, error) {
	var result struct {
		Choices []struct {
			Message agentUpstreamMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return agentUpstreamMessage{}, err
	}
	if len(result.Choices) == 0 {
		return agentUpstreamMessage{}, errors.New("agent upstream returned no choices")
	}
	return result.Choices[0].Message, nil
}

// parseAgentCompletionStream 解析 OpenAI 兼容的 SSE 流：正文每次增量回调累计文本；
// 某个工具调用在后续调用开始或流结束时视为参数完整并回调 onToolReady。
func parseAgentCompletionStream(reader io.Reader, onContent func(string), onReasoning func(string), onToolReady func(int, agentUpstreamToolCall)) (agentUpstreamMessage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	var content, reasoning strings.Builder
	calls := map[int]*agentUpstreamToolCall{}
	ready := map[int]bool{}
	lastIndex := -1
	sawChunk := false
	markReady := func(before int) {
		indexes := make([]int, 0, len(calls))
		for index := range calls {
			if index < before && !ready[index] {
				indexes = append(indexes, index)
			}
		}
		sort.Ints(indexes)
		for _, index := range indexes {
			ready[index] = true
			if onToolReady != nil {
				onToolReady(index, *calls[index])
			}
		}
	}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content          string           `json:"content"`
					ReasoningContent agentLooseString `json:"reasoning_content"`
					Reasoning        agentLooseString `json:"reasoning"`
					ToolCalls        []struct {
						Index    *int                      `json:"index"`
						ID       string                    `json:"id"`
						Type     string                    `json:"type"`
						Function agentUpstreamToolFunction `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				Message *agentUpstreamMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return agentUpstreamMessage{}, fmt.Errorf("agent upstream stream chunk invalid: %w", err)
		}
		if chunk.Error != nil {
			return agentUpstreamMessage{}, fmt.Errorf("agent upstream stream error: %s", truncateAgentRunes(chunk.Error.Message, 300))
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		if choice.Message != nil && !sawChunk && content.Len() == 0 && len(calls) == 0 {
			// 少数兼容服务在流里一次性返回完整 message。
			content.WriteString(choice.Message.Content)
			reasoning.WriteString(choice.Message.reasoningText())
			for i, call := range choice.Message.ToolCalls {
				copied := call
				calls[i] = &copied
			}
			if onContent != nil && content.Len() > 0 {
				onContent(content.String())
			}
			continue
		}
		sawChunk = true
		// 两个推理字段同时出现时内容相同，只取其一。
		delta := string(choice.Delta.ReasoningContent)
		if delta == "" {
			delta = string(choice.Delta.Reasoning)
		}
		if delta != "" {
			reasoning.WriteString(delta)
			if onReasoning != nil {
				onReasoning(reasoning.String())
			}
		}
		if choice.Delta.Content != "" {
			content.WriteString(choice.Delta.Content)
			if onContent != nil {
				onContent(content.String())
			}
		}
		for _, delta := range choice.Delta.ToolCalls {
			index := lastIndex
			if delta.Index != nil {
				index = *delta.Index
			} else if delta.ID != "" || index < 0 {
				index = lastIndex + 1
			}
			if index < 0 {
				index = 0
			}
			call, exists := calls[index]
			if !exists {
				call = &agentUpstreamToolCall{}
				calls[index] = call
			}
			if index > lastIndex {
				markReady(index)
				lastIndex = index
			}
			if delta.ID != "" && call.ID == "" {
				call.ID = delta.ID
			}
			if delta.Type != "" {
				call.Type = delta.Type
			}
			if delta.Function.Name != "" && call.Function.Name == "" {
				call.Function.Name = delta.Function.Name
			}
			call.Function.Arguments += delta.Function.Arguments
		}
	}
	if err := scanner.Err(); err != nil {
		return agentUpstreamMessage{}, err
	}
	markReady(int(^uint(0) >> 1))
	indexes := make([]int, 0, len(calls))
	for index := range calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	message := agentUpstreamMessage{Content: content.String(), ReasoningContent: agentLooseString(reasoning.String()), ToolCalls: make([]agentUpstreamToolCall, 0, len(indexes))}
	for _, index := range indexes {
		call := *calls[index]
		if call.Type == "" {
			call.Type = "function"
		}
		message.ToolCalls = append(message.ToolCalls, call)
	}
	return message, nil
}

// agentRunStreamPublisher 把模型流转成不落库的运行事件，正文和推理按固定间隔节流。
type agentRunStreamPublisher struct {
	runID              string
	mu                 sync.Mutex
	lastPublish        time.Time
	pending            string
	published          string
	pendingReasoning   string
	publishedReasoning string
}

func newAgentRunStreamPublisher(runID string) *agentRunStreamPublisher {
	return &agentRunStreamPublisher{runID: runID}
}

func (p *agentRunStreamPublisher) content(text string) {
	p.mu.Lock()
	p.pending = text
	due := time.Since(p.lastPublish) >= agentStreamPublishInterval
	p.mu.Unlock()
	if due {
		p.flush()
	}
}

func (p *agentRunStreamPublisher) reasoning(text string) {
	p.mu.Lock()
	p.pendingReasoning = text
	due := time.Since(p.lastPublish) >= agentStreamPublishInterval
	p.mu.Unlock()
	if due {
		p.flush()
	}
}

func (p *agentRunStreamPublisher) flush() {
	p.mu.Lock()
	text := sanitizeAgentUserFacingText(p.pending)
	reasoning := truncateAgentRunes(strings.TrimSpace(p.pendingReasoning), agentReasoningMaxRunes)
	publishText := strings.TrimSpace(text) != "" && text != p.published
	publishReasoning := reasoning != "" && reasoning != p.publishedReasoning
	if publishText {
		p.published = text
	}
	if publishReasoning {
		p.publishedReasoning = reasoning
	}
	if publishText || publishReasoning {
		p.lastPublish = time.Now()
	}
	p.mu.Unlock()
	if publishReasoning {
		payload, _ := json.Marshal(map[string]any{"content": reasoning, "streaming": true})
		repository.PublishAgentRunEvent(p.runID, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventReasoningStream, Payload: string(payload), CreatedAt: now()})
	}
	if publishText {
		payload, _ := json.Marshal(map[string]any{"content": text, "streaming": true})
		repository.PublishAgentRunEvent(p.runID, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventMessageStream, Payload: string(payload), CreatedAt: now()})
	}
}

func (p *agentRunStreamPublisher) toolReady(index int, call agentUpstreamToolCall) {
	name := canonicalAgentToolName(call.Function.Name)
	if _, ok := agentToolDefinitionFor(name); !ok {
		return
	}
	var arguments any
	if json.Unmarshal([]byte(call.Function.Arguments), &arguments) != nil {
		arguments = nil
	}
	payload, _ := json.Marshal(map[string]any{"index": index, "name": name, "arguments": arguments})
	repository.PublishAgentRunEvent(p.runID, model.AgentEvent{ID: newID("agent-event"), Type: model.AgentEventToolPreview, Payload: string(payload), CreatedAt: now()})
}
