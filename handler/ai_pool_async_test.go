package handler

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuiltinPoolUsesAsyncImageTasks(t *testing.T) {
	for _, path := range []string{"/images/generations", "/images/edits"} {
		if !supportsAsyncImageTasks("http://nexa.internal/v1", path) {
			t.Fatalf("pool must use async image tasks for %s", path)
		}
	}
	if supportsAsyncImageTasks("http://nexa.internal/v1", "/videos") {
		t.Fatal("videos keep their own task flow")
	}
	if supportsAsyncImageTasks("https://example.com/v1", "/images/generations") {
		t.Fatal("third-party channels unchanged")
	}
	if got := imageTaskRecoveryWindow("http://nexa.internal/v1/images/generations"); got < 20*time.Minute {
		t.Fatalf("pool recovery window too short: %v", got)
	}
	if got := imageTaskRecoveryWindow("https://api.huantu.xyz/v1/images/generations"); got != 7*time.Minute {
		t.Fatalf("other channels unchanged, got %v", got)
	}
}

func TestRecoverImageTaskReroutesOnlyPreAcceptanceFailures(t *testing.T) {
	for _, tc := range []struct {
		body string
		want imageTaskRecoveryState
	}{
		{`{"status":"failed","error":"no account","reroutable":true}`, imageTaskRecoveryReroute},
		{`{"status":"failed","error":"provider request failed"}`, imageTaskRecoveryHandled},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
		request := httptest.NewRequest(http.MethodPost, server.URL+"/v1/images/generations", nil)
		request.Header.Set("Idempotency-Key", "req-1")
		_, _, state := recoverImageTask(httptest.NewRecorder(), request, true)
		server.Close()
		if state != tc.want {
			t.Fatalf("%s: state=%v want %v", tc.body, state, tc.want)
		}
	}
}

func TestStripPoolDefaultImageFields(t *testing.T) {
	body, _ := stripPoolDefaultImageFields([]byte(`{"prompt":"p","output_format":"png","background":"auto","n":1}`), "application/json")
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	if _, ok := payload["output_format"]; ok || payload["background"] != nil || payload["prompt"] != "p" {
		t.Fatalf("json defaults not stripped: %s", body)
	}
	kept, _ := stripPoolDefaultImageFields([]byte(`{"output_format":"webp"}`), "application/json")
	if !strings.Contains(string(kept), "webp") {
		t.Fatal("explicit non-default formats are left for the pool to reject")
	}
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	_ = writer.WriteField("prompt", "p")
	_ = writer.WriteField("output_format", "png")
	file, _ := writer.CreateFormFile("image", "a.png")
	_, _ = file.Write([]byte("img"))
	_ = writer.Close()
	stripped, contentType := stripPoolDefaultImageFields(buffer.Bytes(), writer.FormDataContentType())
	_, params, _ := mime.ParseMediaType(contentType)
	form, err := multipart.NewReader(bytes.NewReader(stripped), params["boundary"]).ReadForm(1 << 20)
	if err != nil || form.Value["output_format"] != nil || form.Value["prompt"][0] != "p" || len(form.File["image"]) != 1 {
		t.Fatalf("multipart defaults not stripped: %v", err)
	}
}
