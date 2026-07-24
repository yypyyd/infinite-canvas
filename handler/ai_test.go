package handler

import (
	"encoding/json"
	"mime/multipart"
	"strings"
	"testing"
)

func TestAIUpstreamErrorDetail(t *testing.T) {
	got := aiUpstreamErrorDetail([]byte(`{"error":{"code":"InvalidParameter","message":"reference video fps is invalid"}}`))
	if got != "InvalidParameter reference video fps is invalid" {
		t.Fatalf("detail = %q", got)
	}
}

func TestResolveAIProxyPathVividAI(t *testing.T) {
	if got := resolveAIProxyPath("https://vividai.run/v1", "firefly-video", "/videos"); got != "/videos" {
		t.Fatalf("create path = %q", got)
	}
	if got := resolveAIProxyPath("https://vividai.run/v1", "firefly-video", "/videos/task-1/content"); got != "/videos/task-1/content" {
		t.Fatalf("content path = %q", got)
	}
	if got := resolveAIProxyPath("https://api.x.ai/v1", "grok-imagine-video", "/videos"); got != "/videos/generations" {
		t.Fatalf("xAI create path = %q", got)
	}
}

func TestAdaptVideoGenerationsResponse(t *testing.T) {
	body, contentType, ok := adaptVideoGenerationsResponse([]byte(`{"request_id":"task-1","done":true}`))
	if !ok || contentType != "application/json" {
		t.Fatalf("adapter ok=%v contentType=%q", ok, contentType)
	}
	if !strings.Contains(string(body), `"id":"task-1"`) || !strings.Contains(string(body), `"status":"completed"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestAdaptVideoGenerationsResponseCachesB64JSON(t *testing.T) {
	body, contentType, ok := adaptVideoGenerationsResponse([]byte(`{"data":[{"b64_json":"dmlkZW8="}]}`))
	if !ok || contentType != "application/json" {
		t.Fatalf("adapter ok=%v contentType=%q", ok, contentType)
	}
	if !strings.Contains(string(body), `"id":"local-`) || !strings.Contains(string(body), `"status":"completed"`) {
		t.Fatalf("body = %s", body)
	}
	var payload map[string]string
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	videoGenerationCache.Lock()
	item, ok := videoGenerationCache.items[payload["id"]]
	videoGenerationCache.Unlock()
	if !ok || string(item.Body) != "video" {
		t.Fatalf("cache item ok=%v body=%q", ok, string(item.Body))
	}
}

func TestAdaptVividAIVideoRequestBody(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://vividai.run/v1", "firefly-video", "/videos", []byte(`{"model":"firefly-video","prompt":"test","seconds":"5","resolution_name":"720p","size":"16:9"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("contentType = %q", contentType)
	}
	text := string(body)
	for _, want := range []string{`"seconds":"5"`, `"size":"1280x720"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("body %s missing %s", text, want)
		}
	}
	for _, unwanted := range []string{`resolution_name`, `resolution`, `aspect_ratio`, `duration`} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("body %s contains unsupported field %s", text, unwanted)
		}
	}
}

func TestAdaptVividAIVideoMultipartBody(t *testing.T) {
	var source strings.Builder
	writer := multipart.NewWriter(&source)
	_ = writer.WriteField("model", "firefly-video")
	_ = writer.WriteField("prompt", "test")
	_ = writer.WriteField("seconds", "8")
	_ = writer.WriteField("size", "9:16")
	_ = writer.WriteField("resolution_name", "1080p")
	_ = writer.WriteField("preset", "normal")
	file, _ := writer.CreateFormFile("input_reference[]", "image.png")
	_, _ = file.Write([]byte("png"))
	_ = writer.Close()

	body, contentType := adaptAIRequestBody("https://vividai.run/v1", "firefly-video", "/videos", []byte(source.String()), writer.FormDataContentType())
	form, err := readMultipartForm(body, contentType)
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()
	if got := firstFormValue(form, "size"); got != "1080x1920" {
		t.Fatalf("size = %q", got)
	}
	if len(form.File["input_reference"]) != 1 || len(form.File["input_reference[]"]) != 0 {
		t.Fatalf("reference fields = %#v", form.File)
	}
	if firstFormValue(form, "resolution_name", "preset") != "" {
		t.Fatalf("unsupported fields were preserved")
	}
}

func TestAdaptVividAIImageRequestBody(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://vividai.run/v1", "firefly-gpt-image-2", "/images/generations", []byte(`{"model":"firefly-gpt-image-2","prompt":"test","size":"3:2","quality":"high","response_format":"b64_json"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("contentType = %q", contentType)
	}
	text := string(body)
	if !strings.Contains(text, `"size":"3600x2400"`) {
		t.Fatalf("body %s has wrong size", text)
	}
	for _, unwanted := range []string{`quality`, `response_format`, `resolution`} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("body %s contains unsupported field %s", text, unwanted)
		}
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

func TestVideoGenerationsContentURL(t *testing.T) {
	got := videoGenerationsContentURL([]byte(`{"data":[{"url":"https://example.com/video.mp4"}]}`))
	if got != "https://example.com/video.mp4" {
		t.Fatalf("url = %q", got)
	}
}

func TestRetryableAIUpstreamError(t *testing.T) {
	cases := []struct {
		status int
		body   string
	}{
		{408, `{"error_code":"timeout_error","message":"system under load"}`},
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
