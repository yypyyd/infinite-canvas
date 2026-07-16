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

func TestResolveAIProxyPathVideoGenerations(t *testing.T) {
	if got := resolveAIProxyPath("https://vividai.run/v1", "firefly-video", "/videos"); got != "/videos/generations" {
		t.Fatalf("create path = %q", got)
	}
	if got := resolveAIProxyPath("https://vividai.run/v1", "firefly-video", "/videos/task-1/content"); got != "/videos/task-1" {
		t.Fatalf("content path = %q", got)
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

func TestAdaptAIRequestBodyVideoGenerations(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://vividai.run/v1", "firefly-video", "/videos", []byte(`{"model":"firefly-video","prompt":"test","seconds":"5","resolution_name":"720p","size":"16:9"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("contentType = %q", contentType)
	}
	text := string(body)
	for _, want := range []string{`"duration":"5s"`, `"resolution":"720p"`, `"aspect_ratio":"16:9"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("body %s missing %s", text, want)
		}
	}
}

func TestNormalizeVideoGenerationsPayloadByModel(t *testing.T) {
	tests := []struct {
		model string
		body  string
		want  []string
	}{
		{"firefly-video", `{"model":"firefly-video","prompt":"test","seconds":"6","resolution_name":"480p","size":"1024x1024"}`, []string{`"duration":"5s"`, `"resolution":"720p"`, `"aspect_ratio":"16:9"`}},
		{"firefly-ray", `{"model":"firefly-ray","prompt":"test","seconds":"6","resolution_name":"1080p","size":"3:4"}`, []string{`"duration":"10s"`, `"resolution":"720p"`, `"aspect_ratio":"3:4"`}},
		{"gemini-veo31", `{"model":"gemini-veo31","prompt":"test","seconds":"5","resolution_name":"1080p","size":"9:16"}`, []string{`"duration":"6s"`, `"resolution":"1080p"`, `"aspect_ratio":"9:16"`}},
	}
	for _, tt := range tests {
		body, _ := adaptAIRequestBody("https://vividai.run/v1", tt.model, "/videos", []byte(tt.body), "application/json")
		text := string(body)
		for _, want := range tt.want {
			if !strings.Contains(text, want) {
				t.Fatalf("%s body %s missing %s", tt.model, text, want)
			}
		}
	}
}

func TestNormalizeVideoGenerationsPayloadStripsReferenceDataURL(t *testing.T) {
	body, _ := adaptAIRequestBody("https://vividai.run/v1", "gemini-veo31", "/videos", []byte(`{"model":"gemini-veo31","prompt":"test","duration":"6s","reference_images":["data:image/png;base64,QUJD"]}`), "application/json")
	text := string(body)
	if !strings.Contains(text, `"reference_images":["QUJD"]`) {
		t.Fatalf("body = %s", text)
	}
	if strings.Contains(text, "data:image") {
		t.Fatalf("body still contains data URL prefix: %s", text)
	}
}

func TestAdaptVividAIImageRequestBody(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://vividai.run/v1", "firefly-image-5", "/images/generations", []byte(`{"model":"firefly-image-5","prompt":"test","size":"3:2","resolution":"4k"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("contentType = %q", contentType)
	}
	text := string(body)
	for _, want := range []string{`"size":"1:1"`, `"resolution":"1k"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("body %s missing %s", text, want)
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
