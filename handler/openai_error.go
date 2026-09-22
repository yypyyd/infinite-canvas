package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

type openAIErrorBody struct {
	Error openAIErrorDetail `json:"error"`
}

type openAIErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

func isOpenAIResponse(w http.ResponseWriter) bool {
	type marker interface{ OpenAICompatible() bool }
	seen := map[http.ResponseWriter]struct{}{}
	for w != nil {
		if _, ok := seen[w]; ok {
			return false
		}
		seen[w] = struct{}{}
		if marked, ok := w.(marker); ok && marked.OpenAICompatible() {
			return true
		}
		unwrapper, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		next := unwrapper.Unwrap()
		if next == nil || next == w {
			return false
		}
		w = next
	}
	return false
}

func writeOpenAIError(w http.ResponseWriter, status int, message, errType, code string, param *string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(openAIErrorBody{Error: openAIErrorDetail{
		Message: message, Type: errType, Param: param, Code: code,
	}})
}

func classifyOpenAIError(msg string) (int, string, string, *string) {
	modelParam := "model"
	switch {
	case strings.Contains(msg, "API Key 无效"), strings.Contains(msg, "未登录"):
		return http.StatusUnauthorized, "authentication_error", "invalid_api_key", nil
	case strings.Contains(msg, "AI 接口鉴权失败"):
		return http.StatusBadGateway, "api_error", "upstream_auth_error", nil
	case strings.Contains(msg, "限流"), strings.Contains(msg, "过于频繁"):
		return http.StatusTooManyRequests, "rate_limit_error", "rate_limit_exceeded", nil
	case strings.Contains(msg, "余额不足"), strings.Contains(msg, "预算不足"), strings.Contains(msg, "额度不足"):
		return http.StatusTooManyRequests, "insufficient_quota", "insufficient_quota", nil
	case strings.Contains(msg, "不能访问其他企业"):
		return http.StatusForbidden, "permission_error", "organization_mismatch", nil
	case strings.Contains(msg, "企业不可用"), strings.Contains(msg, "企业不存在"), strings.Contains(msg, "权限不足"):
		return http.StatusForbidden, "permission_error", "insufficient_permissions", nil
	case strings.Contains(msg, "缺少模型"):
		return http.StatusBadRequest, "invalid_request_error", "missing_model", &modelParam
	case strings.Contains(msg, "未设置价格"):
		return http.StatusBadRequest, "invalid_request_error", "model_not_priced", &modelParam
	case strings.Contains(msg, "没有支持"):
		return http.StatusNotFound, "invalid_request_error", "model_not_found", &modelParam
	case strings.Contains(msg, "不存在"):
		return http.StatusNotFound, "invalid_request_error", "not_found", nil
	case strings.Contains(msg, "请勿重复"):
		return http.StatusConflict, "invalid_request_error", "idempotency_conflict", nil
	case strings.HasPrefix(msg, "AI 接口"):
		return http.StatusBadGateway, "api_error", "upstream_error", nil
	default:
		return http.StatusBadRequest, "invalid_request_error", "invalid_request", nil
	}
}
