package handler

import (
	"encoding/json"
	"errors"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"testing"
)

func TestAIUpstreamErrorDetail(t *testing.T) {
	got := aiUpstreamErrorDetail([]byte(`{"error":{"code":"InvalidParameter","message":"reference video fps is invalid"}}`))
	if got != "InvalidParameter reference video fps is invalid" {
		t.Fatalf("detail = %q", got)
	}
}

func TestReplaceAIRequestModel(t *testing.T) {
	body, _, err := replaceAIRequestModel([]byte(`{"model":"public-image","prompt":"test"}`), "application/json", "upstream-image")
	if err != nil || !strings.Contains(string(body), `"model":"upstream-image"`) {
		t.Fatalf("body=%s err=%v", body, err)
	}
}

func TestReplaceMultipartAIRequestModel(t *testing.T) {
	var source strings.Builder
	writer := multipart.NewWriter(&source)
	_ = writer.WriteField("model", "public-image")
	_ = writer.WriteField("prompt", "test")
	file, _ := writer.CreateFormFile("image", "image.png")
	_, _ = file.Write([]byte("png"))
	_ = writer.Close()

	body, contentType, err := replaceAIRequestModel([]byte(source.String()), writer.FormDataContentType(), "upstream-image")
	if err != nil {
		t.Fatal(err)
	}
	request := readMultipartAIRequest(body, contentType)
	if request.ModelName != "upstream-image" {
		t.Fatalf("model = %q, want upstream-image", request.ModelName)
	}
}

func TestAdaptVividAIImageRequestDoesNotTreatQualityAsResolution(t *testing.T) {
	body, _ := adaptVividAIImageRequestBody([]byte(`{"model":"image","prompt":"test","size":"1024x1024","quality":"high"}`), "application/json")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["size"] != "1024x1024" {
		t.Fatalf("size = %q, want 1024x1024", payload["size"])
	}
}

func TestAIImageResponseErrorUsesNestedUpstreamMessage(t *testing.T) {
	got := aiImageResponseError([]byte(`{"data":[{"error":{"code":"adobe_failed","message":"Adobe upstream rejected the image"}}]}`))
	if got != "AI 接口请求失败：adobe_failed Adobe upstream rejected the image" {
		t.Fatalf("message = %q", got)
	}
}

func TestAIImageResponseErrorAcceptsImage(t *testing.T) {
	if got := aiImageResponseError([]byte(`{"data":[{"url":"https://example.com/image.png"}]}`)); got != "" {
		t.Fatalf("message = %q, want empty", got)
	}
}

func TestAIImageResponseErrorExplainsEmptyData(t *testing.T) {
	got := aiImageResponseError([]byte(`{"data":[]}`))
	if got != "AI 接口请求失败：上游没有返回图片" {
		t.Fatalf("message = %q", got)
	}
}

func TestAdaptVividAIImageRequestUsesPixelTierForPortraitSize(t *testing.T) {
	body, _ := adaptVividAIImageRequestBody([]byte(`{"model":"image","prompt":"test","size":"1024x1824"}`), "application/json")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["size"] != "720x1280" {
		t.Fatalf("size = %q, want 720x1280", payload["size"])
	}
}

func TestAdaptVividAIImageMultipartPreservesReferenceContentType(t *testing.T) {
	var source strings.Builder
	writer := multipart.NewWriter(&source)
	_ = writer.WriteField("model", "image")
	_ = writer.WriteField("prompt", "test")
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="image"; filename="reference.png"`)
	header.Set("Content-Type", "image/png")
	file, _ := writer.CreatePart(header)
	_, _ = file.Write([]byte("png"))
	_ = writer.Close()

	body, contentType, err := adaptVividAIImageMultipartBody([]byte(source.String()), writer.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	form, err := readMultipartForm(body, contentType)
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()
	if files := form.File["image"]; len(files) != 1 || files[0].Header.Get("Content-Type") != "image/png" {
		t.Fatalf("files = %#v, want one image/png reference", files)
	}
}

func TestRetryableAIUpstreamError(t *testing.T) {
	cases := []struct {
		status int
		body   string
	}{
		{408, `{"error_code":"timeout_error","message":"system under load"}`},
		{500, `{"message":"image generation service unavailable"}`},
		{503, `{"detail":"provider temporary unavailable"}`},
		{502, `{}`},
		{504, `{}`},
	}
	for _, item := range cases {
		if !isRetryableAIUpstreamError(item.status, []byte(item.body)) {
			t.Fatalf("status=%d body=%s should be retryable", item.status, item.body)
		}
	}
	if isRetryableAIUpstreamError(400, []byte(`{"detail":"invalid request body"}`)) {
		t.Fatal("invalid request body should not be retryable")
	}
	if isRetryableAIUpstreamError(502, []byte(`{"detail":"chatgpt content policy rejection"}`)) {
		t.Fatal("content policy rejection should not be retryable")
	}
	if isRetryableAIUpstreamError(502, []byte(`{"code":"content_policy_violation"}`)) {
		t.Fatal("content policy code should not be retryable")
	}
}

func TestShouldFailoverAIImageRequest(t *testing.T) {
	if !shouldFailoverAIImageRequest("/images/edits", &http.Response{StatusCode: 500}, []byte(`{"message":"unavailable"}`), nil, false) {
		t.Fatal("image 500 should fail over")
	}
	if shouldFailoverAIImageRequest("/images/generations", &http.Response{StatusCode: 502}, []byte(`{"message":"content policy rejection"}`), nil, false) {
		t.Fatal("content policy rejection should not fail over")
	}
	if shouldFailoverAIImageRequest("/images/generations", &http.Response{StatusCode: 400}, []byte(`{"code":"timeout_error"}`), nil, false) {
		t.Fatal("image client error should not fail over")
	}
	if !shouldFailoverAIImageRequest("/images/generations", nil, nil, &net.OpError{Op: "dial", Err: errors.New("connection refused")}, false) {
		t.Fatal("image connection establishment error should fail over")
	}
	if shouldFailoverAIImageRequest("/images/generations", nil, nil, &net.OpError{Op: "read", Err: errors.New("connection reset")}, false) {
		t.Fatal("ambiguous image read error should not fail over")
	}
	if shouldFailoverAIImageRequest("/images/generations", &http.Response{StatusCode: 504}, nil, nil, false) {
		t.Fatal("image 504 should recover the original task before failover")
	}
	if !shouldFailoverAIImageRequest("/images/generations", &http.Response{StatusCode: 504}, nil, nil, true) {
		t.Fatal("image 504 should fail over when task recovery is explicitly unsupported")
	}
	if shouldFailoverAIImageRequest("/videos", &http.Response{StatusCode: 503}, nil, nil, false) {
		t.Fatal("video request should not use image failover")
	}
}

func TestAIUpstreamErrorDetailExplainsSensitiveVideo(t *testing.T) {
	got := aiUpstreamErrorDetail([]byte(`{"error":{"code":"InputVideoSensitiveContentDetected.PrivacyInformation","message":"The request failed because the input video may contain real person."}}`))
	if !strings.Contains(got, "参考视频疑似包含真人") || !strings.Contains(got, "asset://") {
		t.Fatalf("detail = %q", got)
	}
}

func TestSafeUpstreamTextTruncates(t *testing.T) {
	got := safeUpstreamText(strings.Repeat("错", 320))
	if len([]rune(got)) != 303 {
		t.Fatalf("truncated rune length = %d", len([]rune(got)))
	}
}
