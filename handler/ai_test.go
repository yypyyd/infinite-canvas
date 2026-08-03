package handler

import (
	"encoding/json"
	"mime/multipart"
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
