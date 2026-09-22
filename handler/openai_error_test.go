package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type openAITestWriter struct {
	*httptest.ResponseRecorder
}

func (w openAITestWriter) OpenAICompatible() bool { return true }

func TestFailUsesOpenAIErrorForAPIKeyWriter(t *testing.T) {
	recorder := openAITestWriter{ResponseRecorder: httptest.NewRecorder()}
	Fail(recorder, "API Key 无效或已撤销")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body openAIErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Type != "authentication_error" || body.Error.Code != "invalid_api_key" || body.Error.Message != "API Key 无效或已撤销" || body.Error.Param != nil {
		t.Fatalf("unexpected error: %#v", body.Error)
	}
}

func TestFailKeepsPlatformEnvelopeWithoutAPIKeyWriter(t *testing.T) {
	recorder := httptest.NewRecorder()
	Fail(recorder, "未登录或权限不足")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var body response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 1 || body.Msg != "未登录或权限不足" {
		t.Fatalf("unexpected envelope: %#v", body)
	}
}

func TestClassifyOpenAIError(t *testing.T) {
	cases := []struct {
		message string
		status  int
		code    string
	}{
		{message: "该模型或当前规格未设置价格", status: http.StatusBadRequest, code: "model_not_priced"},
		{message: "模型 demo 没有支持当前操作或规格的可用渠道", status: http.StatusNotFound, code: "model_not_found"},
		{message: "个人算力余额不足", status: http.StatusTooManyRequests, code: "insufficient_quota"},
		{message: "API Key 不能访问其他企业", status: http.StatusForbidden, code: "organization_mismatch"},
		{message: "AI 接口请求失败：上游连接中断，请稍后重试", status: http.StatusBadGateway, code: "upstream_error"},
		{message: "AI 接口限流或额度不足，请稍后重试或检查额度", status: http.StatusTooManyRequests, code: "rate_limit_exceeded"},
	}
	for _, item := range cases {
		status, _, code, _ := classifyOpenAIError(item.message)
		if status != item.status || code != item.code {
			t.Fatalf("%s => status %d code %s", item.message, status, code)
		}
	}
}
