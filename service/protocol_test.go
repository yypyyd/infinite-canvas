package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestProtocolNDJSONPersistsReceiptBeforeMalformedTail(t *testing.T) {
	mapping := vividAIProtocolTemplate().Operations["image.generation"].Create.Response
	for _, tail := range []string{`{"status":`, `{"jobId":"job-1","status":"unknown"}`} {
		frames := []protocolTaskResult{}
		err := decodeProtocolResponse(strings.NewReader("\n{\"jobId\":\"job-1\",\"status\":\"running\"}\n\n"+tail), mapping, func(frame protocolTaskResult) error { frames = append(frames, frame); return nil })
		if err == nil || len(frames) == 0 || frames[0].TaskID != "job-1" || frames[0].Status != "running" {
			t.Fatalf("receipt lost: %#v, %v", frames, err)
		}
	}
	frames := []protocolTaskResult{}
	err := decodeProtocolResponse(strings.NewReader("\n{\"jobId\":\"j\",\"status\":\"running\"}\n\n{\"jobId\":\"j\",\"status\":\"succeeded\",\"result\":\"https://example.com/out.png\"}"), mapping, func(frame protocolTaskResult) error { frames = append(frames, frame); return nil })
	if err != nil || len(frames) != 2 || frames[1].Status != "success" {
		t.Fatalf("stream failed: %#v, %v", frames, err)
	}
}

func TestProtocolPreviewMapsTypedValuesAndRedactsCredentials(t *testing.T) {
	result, err := PreviewProtocol(ProtocolPreviewInput{Template: vividAIProtocolTemplate(), Operation: "video.generation", Variables: map[string]any{
		"model": "video", "prompt": "example", "resolution": "1k", "seconds": 5, "reference_keys": []string{"ref"}, "api_key": "do-not-echo",
	}})
	if err != nil {
		t.Fatal(err)
	}
	body := result["body"].(map[string]any)
	if body["quality"] != "1K" || body["duration"] != 5 || !reflect.DeepEqual(body["refs"], []string{"ref"}) || body["ratio"] != nil {
		t.Fatalf("wrong mapping: %#v", body)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "do-not-echo") || result["headers"].(map[string]string)["Authorization"] != "Bearer ***" {
		t.Fatal("preview exposed credentials")
	}
}

func TestProtocolValidationRejectsUnrecoverableTemplates(t *testing.T) {
	if err := ValidateProtocolTemplate(vividAIProtocolTemplate()); err != nil {
		t.Fatal(err)
	}
	withIdempotency := vividAIProtocolTemplate()
	withIdempotency.Operations["image.generation"].Create.Headers["Idempotency-Key"] = "${request_id}"
	if err := ValidateProtocolTemplate(withIdempotency); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []func(*model.ProtocolTemplate){
		func(item *model.ProtocolTemplate) { item.Headers["Authorization"] = "Bearer secret" },
		func(item *model.ProtocolTemplate) {
			item.Operations["image.generation"].Resume.Body["prompt"] = "${prompt}"
		},
		func(item *model.ProtocolTemplate) {
			item.Operations["image.generation"].Resume.Path = "//untrusted.example.com/${task_id}"
		},
		func(item *model.ProtocolTemplate) {
			item.Operations["image.generation"].Resume.Response.Success = []string{"running"}
		},
	} {
		item := vividAIProtocolTemplate()
		edit(&item)
		if err := ValidateProtocolTemplate(item); err == nil {
			t.Fatal("invalid template accepted")
		}
	}
	value, err := protocolText("/jobs?id=${task_id}", map[string]any{"task_id": "a+b&c"}, true)
	if err != nil || value != "/jobs?id=a%2Bb%26c" {
		t.Fatalf("query substitution: %s, %v", value, err)
	}
}

func TestProtocolPresignedUploadKeepsMIMEAndOmitsAuthorization(t *testing.T) {
	original := protocolHTTPClient
	t.Cleanup(func() { protocolHTTPClient = original })
	requests := 0
	data := []byte("reference-content")
	protocolHTTPClient = &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Host == "vividai.run" {
			body, _ := io.ReadAll(request.Body)
			if request.Method != "POST" || request.URL.Path != "/v1/uploads" || request.Header.Get("Authorization") != "Bearer test-key" || !strings.Contains(string(body), `"mime":"image/png"`) {
				t.Fatalf("bad prepare request: %s", body)
			}
			return protocolTestResponse(`{"key":"ref-key","url":"https://upload.example.com/signed?token=example"}`), nil
		}
		body, _ := io.ReadAll(request.Body)
		if request.URL.Host != "upload.example.com" || request.Method != "PUT" || request.Header.Get("Authorization") != "" || request.Header.Get("Content-Type") != "image/png" || !bytes.Equal(body, data) {
			t.Fatal("signed upload did not preserve its trust boundary")
		}
		return protocolTestResponse(`{}`), nil
	})}
	keys, err := uploadProtocolFiles(context.Background(), model.ModelChannel{BaseURL: "https://vividai.run", APIKey: "test-key"}, vividAIProtocolTemplate(), []protocolFile{{Kind: "image", Mime: "image/png", Data: data}})
	if err != nil || requests != 2 || !reflect.DeepEqual(keys, []string{"ref-key"}) {
		t.Fatalf("upload failed: %v, %v", keys, err)
	}
}

func protocolTestResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func protocolTestTask(t *testing.T, suffix string) (model.GenerationTask, model.AuthUser, ModelChannelSelection) {
	t.Helper()
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, now()) })
	settings := saved
	channel := model.ModelChannel{ID: "protocol-channel", Name: "VividAI", Protocol: modelChannelProtocolCustomHTTP, TemplateID: "vividai", BaseURL: "https://vividai.run", APIKey: "test-key"}
	template := vividAIProtocolTemplate()
	for key, operation := range template.Operations {
		operation.Create.Response.ResultURL, operation.Create.Response.ResultBase64 = "", "image"
		operation.Resume.Response.ResultURL, operation.Resume.Response.ResultBase64 = "", "image"
		template.Operations[key] = operation
	}
	settings.Private.Channels = []model.ModelChannel{channel}
	settings.Private.ProtocolTemplates = []model.ProtocolTemplate{template}
	settings.Private.Storage.Driver, settings.Private.Storage.LocalPath = "local", t.TempDir()
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		t.Fatal(err)
	}
	user, auth, organization := seedTenant(t, suffix)
	db, err := repository.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&user).Update("credits", 100).Error; err != nil {
		t.Fatal(err)
	}
	task, err := BeginGenerationTask(GenerationTaskInput{UserID: user.ID, OrganizationID: organization.ID, RequestID: suffix, Model: "image-test", UpstreamModel: "gpt-image-2", ChannelName: channel.Name, Path: "/images/generations", Modality: "image", Operation: "generation", ResolutionTier: "1k", Quantity: 1, Credits: 3})
	if err != nil {
		t.Fatal(err)
	}
	return task, auth, ModelChannelSelection{Channel: channel, Model: model.ChannelModel{Model: task.Model, UpstreamModel: task.UpstreamModel}}
}

func TestProtocolRecoveryUsesSnapshotAndArchivesBeforeSettlement(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "protocol-ordinary"
		if batch {
			name = "protocol-batch"
		}
		t.Run(name, func(t *testing.T) {
			task, auth, selection := protocolTestTask(t, name)
			db, _ := repository.DB()
			if batch {
				task.BatchJobID, task.BatchItemID = "job-test", "item-test"
				if err := db.Model(&task).Updates(map[string]any{"batch_job_id": task.BatchJobID, "batch_item_id": task.BatchItemID}).Error; err != nil {
					t.Fatal(err)
				}
			}
			original := protocolHTTPClient
			t.Cleanup(func() { protocolHTTPClient = original })
			calls := 0
			protocolHTTPClient = &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.URL.String() != "https://vividai.run/v1/generate" {
					t.Fatalf("snapshot endpoint changed: %s", request.URL)
				}
				var body map[string]any
				if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if calls == 1 {
					return protocolTestResponse("{\"jobId\":\"job-1\",\"status\":\"running\"}\n\n"), nil
				}
				if len(body) != 1 || body["jobId"] != "job-1" || request.Header.Get("Authorization") != "Bearer rotated-key" {
					t.Fatalf("recovery replayed create or wrong credentials: %#v", body)
				}
				image := base64.StdEncoding.EncodeToString(append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...))
				return protocolTestResponse(`{"jobId":"job-1","status":"succeeded","image":"` + image + `"}`), nil
			})}
			if err := SubmitManagedMediaTask(context.Background(), auth, &task, selection, MediaGenerationInput{Prompt: "test", Count: 1}); err != nil {
				t.Fatal(err)
			}
			if task.UpstreamTaskID != "job-1" || task.ProtocolSnapshot == nil || task.Status != model.GenerationTaskStatusRunning || len(task.StorageKeys) != 0 {
				t.Fatalf("bad receipt: %#v", task)
			}
			if err := SubmitManagedMediaTask(context.Background(), auth, &task, selection, MediaGenerationInput{}); !errors.Is(err, ErrProtocolSubmissionUncertain) || calls != 1 {
				t.Fatal("submission was replayed")
			}
			settings, _ := repository.GetSettings()
			settings.Private.Channels[0].Name, settings.Private.Channels[0].BaseURL, settings.Private.Channels[0].APIKey = "renamed", "https://changed.example.com", "rotated-key"
			settings.Private.ProtocolTemplates = nil
			if _, err := repository.SaveSettings(settings, now()); err != nil {
				t.Fatal(err)
			}
			settled, err := ReconcileManagedMediaTask(context.Background(), &task, model.ModelChannel{})
			if err != nil || !settled || len(task.StorageKeys) != 1 {
				t.Fatalf("recovery failed: settled=%v, err=%v", settled, err)
			}
			stored, err := UserGenerationTaskByRequest(auth.OrganizationID, auth.ID, task.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			expected := model.GenerationTaskStatusSuccess
			if batch {
				expected = model.GenerationTaskStatusRunning
			}
			if stored.Status != expected || len(stored.StorageKeys) != 1 {
				t.Fatalf("wrong settlement owner: %#v", stored)
			}
			if _, err := readProtocolStoredFile(context.Background(), auth, stored.StorageKeys[0], "image", maxUserFileSize); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(stored.ResultJSON, "https://") || strings.Contains(stored.ResultJSON, "test-key") {
				t.Fatal("upstream secrets escaped")
			}
		})
	}
}

func TestProtocolSynchronousVideoGetsQueryableLocalID(t *testing.T) {
	task, auth, _ := protocolTestTask(t, "protocol-sync-video")
	task.Modality = "video"
	db, _ := repository.DB()
	if err := db.Model(&task).Update("modality", "video").Error; err != nil {
		t.Fatal(err)
	}
	// A minimal MP4 signature is sufficient for the storage MIME check.
	data := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0, 0, 0, 0, 'm', 'p', '4', '2', 'i', 's', 'o', 'm'}
	err := archiveProtocolResult(context.Background(), &task, protocolTaskResult{Status: "success", Base64: []string{base64.StdEncoding.EncodeToString(data)}})
	if err != nil {
		t.Fatal(err)
	}
	if task.UpstreamTaskID != task.ID || len(task.StorageKeys) != 1 || task.Status != model.GenerationTaskStatusSuccess {
		t.Fatalf("missing synchronous video receipt: %#v", task)
	}
	found, exists, err := UserGenerationTaskByUpstreamID(auth.OrganizationID, auth.ID, task.ID)
	if err != nil || !exists || found.ID != task.ID {
		t.Fatalf("video cannot be queried: %v", err)
	}
}

func TestProtocolModelDiscoveryDoesNotInventCapabilities(t *testing.T) {
	_, _, selection := protocolTestTask(t, "protocol-discovery")
	original := protocolHTTPClient
	t.Cleanup(func() { protocolHTTPClient = original })
	protocolHTTPClient = &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != "GET" || request.URL.Path != "/v1/models" {
			t.Fatal("unexpected discovery endpoint")
		}
		return protocolTestResponse(`{"data":[{"id":"custom-video-id"}]}`), nil
	})}
	models, err := fetchProtocolModels(selection.Channel)
	if err != nil || len(models) != 1 || models[0].ID != "custom-video-id" || models[0].Modality != "" || len(models[0].SupportedResolutions) != 0 {
		t.Fatalf("discovery invented metadata: %#v, %v", models, err)
	}
}

func TestProtocolUncertainSubmissionExpiresWithoutReplayAndRefundsOnce(t *testing.T) {
	task, auth, selection := protocolTestTask(t, "protocol-uncertain")
	original := protocolHTTPClient
	t.Cleanup(func() { protocolHTTPClient = original })
	calls := 0
	protocolHTTPClient = &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("connection dropped")
	})}
	if err := SubmitManagedMediaTask(context.Background(), auth, &task, selection, MediaGenerationInput{}); !errors.Is(err, ErrProtocolSubmissionUncertain) {
		t.Fatal(err)
	}
	if task.ProtocolSnapshot == nil {
		t.Fatal("submission marker was not persisted")
	}
	_, _ = ReconcileManagedMediaTask(context.Background(), &task, model.ModelChannel{})
	if calls != 1 {
		t.Fatal("unknown receipt replayed")
	}
	task.ProtocolSnapshot.DeadlineAt = time.Now().UTC().Add(-time.Minute).Format(timestampLayout)
	reconcileStaleGenerationTask(context.Background(), task)
	reconcileStaleGenerationTask(context.Background(), task)
	stored, err := UserGenerationTaskByRequest(auth.OrganizationID, auth.ID, task.RequestID)
	if err != nil || stored.Status != model.GenerationTaskStatusFailed {
		t.Fatalf("timeout was not settled: %v", err)
	}
	user, _, _ := repository.GetUserByID(auth.ID)
	if user.Credits != 100 {
		t.Fatalf("refund was duplicated or lost: %d", user.Credits)
	}
	var refunds int64
	db, _ := repository.DB()
	if err := db.Model(&model.CreditLog{}).Where("related_id = ? AND type = ?", task.ID, model.CreditLogTypeAIRefund).Count(&refunds).Error; err != nil {
		t.Fatal(err)
	}
	if refunds != 1 || calls != 1 {
		t.Fatalf("refunds=%d, submissions=%d", refunds, calls)
	}
}

func TestProtocolSwitchPreservesSavedKeyAndModels(t *testing.T) {
	saved := model.Settings{Private: model.PrivateSetting{Channels: []model.ModelChannel{{Name: "VividAI", BaseURL: "https://vividai.run/v1", APIKey: "saved-key", Models: channelModels("gpt-image-2")}}}}
	updated := normalizeSettings(saved)
	updated.Private.Channels[0].Protocol, updated.Private.Channels[0].TemplateID = modelChannelProtocolCustomHTTP, "vividai"
	updated.Private.Channels[0].BaseURL, updated.Private.Channels[0].APIKey = "https://vividai.run", ""
	// Deep copy the saved configuration because slices intentionally share storage.
	saved.Private.Channels = []model.ModelChannel{{Name: "VividAI", BaseURL: "https://vividai.run/v1", APIKey: "saved-key"}}
	keepPrivateAPIKeys(&updated, saved)
	if updated.Private.Channels[0].APIKey != "saved-key" || len(updated.Private.Channels[0].Models) != 1 {
		t.Fatal("switch discarded the existing configuration")
	}
	if _, ok := findSavedChannel(model.ModelChannel{Name: "new"}, nil, -1); ok {
		t.Fatal("new channel unexpectedly resolved")
	}
}
