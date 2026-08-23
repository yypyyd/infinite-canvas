package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/service"
)

func TestReadJSONAIRequestStorageReferences(t *testing.T) {
	meta := readJSONAIRequest([]byte(`{"model":"video-model","prompt":"make it move","seconds":"10","input_reference_storage_keys":[" image:one ",null],"reference_videos_storage_keys":["video-reference:a","video-reference:b"],"reference_audios_storage_keys":"audio-reference:c"}`))
	if meta.ModelName != "video-model" || meta.Duration != 10 || meta.ReferenceImages != 1 || meta.ReferenceVideos != 2 || meta.ReferenceAudios != 1 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if len(meta.InputReferenceStorageKeys) != 1 || meta.InputReferenceStorageKeys[0] != "image:one" {
		t.Fatalf("input storage keys = %#v", meta.InputReferenceStorageKeys)
	}
	if got := readStringField(meta.Payload, "prompt"); got != "make it move" {
		t.Fatalf("prompt = %q", got)
	}
	if !hasStorageReferences(meta) {
		t.Fatal("expected storage references")
	}
	if !canBuildStorageReferenceRequest(meta) {
		t.Fatal("expected storage-only request")
	}
	meta.Payload["reference_videos"] = []any{"legacy"}
	if canBuildStorageReferenceRequest(meta) {
		t.Fatal("expected mixed legacy request to use multipart path")
	}
}

func TestValidateStorageMediaSize(t *testing.T) {
	var total int64
	media := service.UserWorkspaceMedia{Size: 45 << 20}
	if err := validateStorageMediaSize("reference_videos", media, &total); err != nil {
		t.Fatal(err)
	}
	if err := validateStorageMediaSize("reference_videos", media, &total); err != nil {
		t.Fatal(err)
	}
	if total != 90<<20 {
		t.Fatalf("total = %d, want %d", total, 90<<20)
	}
	if err := validateStorageMediaSize("reference_videos", service.UserWorkspaceMedia{Size: 201 << 20}, &total); err == nil {
		t.Fatal("expected per-video size error")
	}
	total = maxVideoReferenceRequestBytes - (1 << 20)
	if err := validateStorageMediaSize("reference_videos", service.UserWorkspaceMedia{Size: 2 << 20}, &total); err == nil {
		t.Fatal("expected total size error")
	}
}

func TestAppendStorageMultipartFileStreamsAndChecksSize(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = io.WriteString(w, "hello")
	}))
	t.Cleanup(server.Close)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	media := service.UserWorkspaceMedia{MimeType: "video/mp4", Size: 5, URL: server.URL}
	if err := appendStorageMultipartFile(context.Background(), writer, "reference_videos", "video-reference:test", media); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader := multipart.NewReader(&body, writer.Boundary())
	part, err := reader.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(part)
	if err != nil || string(content) != "hello" {
		t.Fatalf("content = %q, err=%v", content, err)
	}
	if part.FormName() != "reference_videos" || part.FileName() != "video-reference:test" {
		t.Fatalf("part = name %q filename %q", part.FormName(), part.FileName())
	}
}

func TestStorageRequestCanReopenBodyForRetry(t *testing.T) {
	temporary, err := os.CreateTemp(t.TempDir(), "video-request-*.multipart")
	if err != nil {
		t.Fatal(err)
	}
	path := temporary.Name()
	if _, err := temporary.WriteString("payload"); err != nil {
		t.Fatal(err)
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequest(http.MethodPost, "http://example.test/videos", temporary)
	if err != nil {
		t.Fatal(err)
	}
	request.GetBody = func() (io.ReadCloser, error) { return os.Open(path) }
	first, err := io.ReadAll(request.Body)
	if err != nil || string(first) != "payload" {
		t.Fatalf("first body = %q, err=%v", first, err)
	}
	_ = request.Body.Close()
	request.Body, err = request.GetBody()
	if err != nil {
		t.Fatal(err)
	}
	second, err := io.ReadAll(request.Body)
	if err != nil || string(second) != "payload" {
		t.Fatalf("retry body = %q, err=%v", second, err)
	}
	_ = request.Body.Close()
}

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

func TestAdaptAIRequestBodyUsesHuantu4KImageAdapter(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://api.huantu.xyz/v1", "image", "/images/generations", []byte(`{"model":"image","prompt":"test","size":"2480x3312","quality":"high"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["size"] != "3072x4096" {
		t.Fatalf("size = %q, want 3072x4096", payload["size"])
	}
}

func TestAdaptVividAIVideoRequestPreserves480p(t *testing.T) {
	body, _ := adaptVividAIVideoRequestBody([]byte(`{"model":"oreate-seedance-2.0-mini","prompt":"test","seconds":"10","resolution":"480p","size":"640x480"}`), "application/json")
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["size"] != "640x480" {
		t.Fatalf("size = %q, want 640x480", payload["size"])
	}
	if payload["resolution"] != "480p" {
		t.Fatalf("resolution = %q, want 480p", payload["resolution"])
	}
}

func TestAdaptAIRequestBodyUsesVividAIVideoAdapter(t *testing.T) {
	body, contentType := adaptAIRequestBody("https://vividai.run", "oreate-seedance-2.0-mini", "/videos", []byte(`{"model":"oreate-seedance-2.0-mini","prompt":"test","seconds":"10","resolution":"480p","size":"640x480"}`), "application/json")
	if contentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", contentType)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["size"] != "640x480" {
		t.Fatalf("size = %q, want 640x480", payload["size"])
	}
	if payload["resolution"] != "480p" {
		t.Fatalf("resolution = %q, want 480p", payload["resolution"])
	}
}

func TestAdaptVividAIVideoMultipartPreserves480p(t *testing.T) {
	var source strings.Builder
	writer := multipart.NewWriter(&source)
	_ = writer.WriteField("model", "oreate-seedance-2.0-mini")
	_ = writer.WriteField("prompt", "test")
	_ = writer.WriteField("seconds", "10")
	_ = writer.WriteField("resolution", "480p")
	_ = writer.WriteField("size", "640x480")
	file, _ := writer.CreateFormFile("input_reference", "reference.png")
	_, _ = file.Write([]byte("png"))
	_ = writer.Close()

	body, contentType, err := adaptVividAIVideoMultipartBody([]byte(source.String()), writer.FormDataContentType())
	if err != nil {
		t.Fatal(err)
	}
	form, err := readMultipartForm(body, contentType)
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()
	if got := firstFormValue(form, "size"); got != "640x480" {
		t.Fatalf("size = %q, want 640x480", got)
	}
	if got := firstFormValue(form, "resolution"); got != "480p" {
		t.Fatalf("resolution = %q, want 480p", got)
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
