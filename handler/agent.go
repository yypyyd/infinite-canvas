package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/service"
)

func CreateAgentSession(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.CreateAgentSessionRequest
	if !decodeAgentJSON(w, r, &request) { return }
	session, err := service.CreateAgentSession(user, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, session)
}

func SubmitAgentMessage(w http.ResponseWriter, r *http.Request, sessionID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.SubmitAgentMessageRequest
	if !decodeAgentJSON(w, r, &request) { return }
	submission, err := service.SubmitAgentMessage(user, sessionID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, submission)
}

func SubmitAgentToolResult(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.SubmitAgentToolResultRequest
	if !decodeAgentJSON(w, r, &request) { return }
	run, err := service.SubmitAgentToolResult(user, runID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, run)
}

func ClaimAgentToolExecution(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.ClaimAgentToolRequest
	if !decodeAgentJSON(w, r, &request) { return }
	if err := service.ClaimAgentToolExecution(user, runID, request); err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]string{"status": "claimed"})
}

func GetAgentToolResultReceipt(w http.ResponseWriter, r *http.Request, runID, callID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	receipt, err := service.GetAgentToolResultReceipt(user, runID, callID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, receipt)
}

func ConfirmAgentTool(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.ConfirmAgentToolRequest
	if !decodeAgentJSON(w, r, &request) { return }
	run, err := service.ConfirmAgentTool(user, runID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, run)
}

func GetAgentRun(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	run, err := service.GetAgentRun(user, runID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, run)
}

func CancelAgentRun(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	run, err := service.CancelAgentRun(user, runID)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, run)
}

func AgentRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	after, ok := agentEventAfter(w, r)
	if !ok { return }
	if _, err := service.GetAgentRun(user, runID); err != nil {
		FailError(w, err)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		Fail(w, "当前服务不支持事件流")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	pollTicker := time.NewTicker(200 * time.Millisecond)
	heartbeatTicker := time.NewTicker(15 * time.Second)
	defer pollTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		events, err := service.ListAgentEvents(user, runID, after)
		if err != nil { return }
		for _, event := range events {
			if err := writeAgentEvent(w, event); err != nil { return }
			after = event.Sequence
		}
		if len(events) > 0 { flusher.Flush() }
		if len(events) < 100 {
			run, err := service.GetAgentRun(user, runID)
			if err != nil { return }
			if run.Terminal() { return }
		}
		if len(events) == 100 { continue }

		select {
		case <-r.Context().Done():
			return
		case <-pollTicker.C:
		case <-heartbeatTicker.C:
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil { return }
			flusher.Flush()
		}
	}
}

func decodeAgentJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		Fail(w, "请求参数无效")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		Fail(w, "请求参数无效")
		return false
	}
	return true
}

func agentEventAfter(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.URL.Query().Get("after"))
	if value == "" { value = strings.TrimSpace(r.Header.Get("Last-Event-ID")) }
	if value == "" { return 0, true }
	after, err := strconv.ParseInt(value, 10, 64)
	if err != nil || after < 0 {
		Fail(w, "事件游标无效")
		return 0, false
	}
	return after, true
}

func writeAgentEvent(w io.Writer, event model.AgentEvent) error {
	payload := any(map[string]any{})
	if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
		payload = map[string]any{}
	}
	data, err := json.Marshal(struct {
		ID        string               `json:"id"`
		RunID     string               `json:"runId"`
		Sequence  int64                `json:"sequence"`
		Type      model.AgentEventType `json:"type"`
		Data      any                  `json:"data"`
		CreatedAt string               `json:"createdAt"`
	}{
		ID: event.ID, RunID: event.RunID, Sequence: event.Sequence, Type: event.Type,
		Data: payload, CreatedAt: event.CreatedAt,
	})
	if err != nil { return err }
	_, err = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, data)
	return err
}
