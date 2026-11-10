package handler

import (
	"bytes"
	"context"
	"net/http"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func init() {
	service.AgentAIRequester = requestAIInProcess
}

// inProcessResponse 在内存里接住生成接口写出的响应。
type inProcessResponse struct {
	header http.Header
	body   bytes.Buffer
}

func (response *inProcessResponse) Header() http.Header { return response.header }

func (response *inProcessResponse) Write(data []byte) (int, error) { return response.body.Write(data) }

func (response *inProcessResponse) WriteHeader(int) {}

// requestAIInProcess 让服务端执行的助手工具直接走 /api/v1 的生成处理流程，不经过网络和登录态：
// 选路、扣费、归档和失败返还与页面发起的请求完全一致，失败时响应体是 { code, msg }。
func requestAIInProcess(ctx context.Context, user model.AuthUser, path, contentType, requestID string, body []byte) []byte {
	request, err := http.NewRequestWithContext(service.WithUser(ctx, user), http.MethodPost, "/api/v1"+path, bytes.NewReader(body))
	if err != nil {
		return nil
	}
	request.Header.Set("Content-Type", contentType)
	request.Header.Set("Idempotency-Key", requestID)
	response := &inProcessResponse{header: http.Header{}}
	proxyAIRequest(response, request, path)
	return response.body.Bytes()
}

func ClaimAgentToolPresentation(w http.ResponseWriter, r *http.Request, runID string) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "未登录或权限不足")
		return
	}
	var request service.ClaimAgentToolPresentationRequest
	if !decodeAgentJSON(w, r, &request) {
		return
	}
	status, err := service.ClaimAgentToolPresentation(user, runID, request)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]string{"status": status})
}
