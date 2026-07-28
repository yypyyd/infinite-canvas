package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

func TestBatchProductionPromptIncludesFrozenCommerceInput(t *testing.T) {
	brand := &model.Brand{Name: "测试品牌", Tone: "克制", Guidelines: "保持蓝色", ProhibitedTerms: []string{"夸大"}}
	sku := &model.ProductSKU{Name: "蓝色款", Attributes: map[string]string{"尺寸": "大", "颜色": "蓝"}}
	preset, ok := findBuiltinProductionTemplate("product-main")
	if !ok { t.Fatal("product-main production template is missing") }
	input := BatchProductionExecution{
		Job: model.BatchProductionJob{PresetID: "product-main", PresetPrompt: preset.Prompt},
		Product: model.Product{Name: "测试商品", Category: "家居", Description: "商品描述", SellingPoints: []string{"耐用", "轻巧"}},
		Brand: brand,
		SKU: sku,
	}
	prompt, err := batchProductionPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"纯白背景电商主图", "测试商品", "测试品牌", "耐用；轻巧", "蓝色款", "夸大"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt does not contain %q: %s", expected, prompt)
		}
	}
	if strings.Index(prompt, "尺寸=大") > strings.Index(prompt, "颜色=蓝") {
		t.Fatal("SKU attributes should be sorted")
	}
}

func TestParseStandardBatchResponseAcceptsBase64Image(t *testing.T) {
	image := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	body := fmt.Sprintf(`{"data":[{"b64_json":%q}]}`, base64.StdEncoding.EncodeToString(image))
	result, err := parseStandardBatchResponse(context.Background(), http.DefaultClient, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if result.MimeType != "image/png" || result.Size != int64(len(image)) || !bytes.Equal(result.Data, image) {
		t.Fatalf("unexpected image result: %#v", result)
	}
}

func TestParseStandardBatchResponseDownloadsURLImage(t *testing.T) {
	image := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	client := &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://cdn.example.com/generated.png" {
			t.Fatalf("unexpected image URL: %s", request.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(image)), Request: request}, nil
	})}
	result, err := parseStandardBatchResponse(context.Background(), client, strings.NewReader(`{"data":[{"url":"https://cdn.example.com/generated.png"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if result.MimeType != "image/png" || !bytes.Equal(result.Data, image) {
		t.Fatalf("unexpected URL image result: %#v", result)
	}
}

func TestParseStandardBatchResponseRejectsNonImageURLContent(t *testing.T) {
	client := &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("not an image")), Request: request}, nil
	})}
	if _, err := parseStandardBatchResponse(context.Background(), client, strings.NewReader(`{"data":[{"url":"https://cdn.example.com/generated.png"}]}`)); err == nil {
		t.Fatal("expected non-image URL content to be rejected")
	}
}

func TestBuildStandardBatchEditUsesDocumentedImageArrayField(t *testing.T) {
	image := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0}, 32)...)
	client := &http.Client{Transport: batchWorkerRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(image)), Request: request}, nil
	})}
	selection := ModelChannelSelection{Channel: model.ModelChannel{BaseURL: "https://api.example.com"}, Model: model.ChannelModel{UpstreamModel: "image-model"}}
	body, contentType, err := buildStandardBatchRequest(context.Background(), client, selection, "生成商品图", []string{"https://cdn.example.com/reference.png"}, standardBatchImageSize)
	if err != nil {
		t.Fatal(err)
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatal(err)
	}
	form, err := multipart.NewReader(bytes.NewReader(body), params["boundary"]).ReadForm(int64(len(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer form.RemoveAll()
	if len(form.File["image[]"]) != 1 || form.Value["model"][0] != "image-model" || form.Value["prompt"][0] != "生成商品图" {
		t.Fatalf("unexpected multipart form: values=%#v files=%#v", form.Value, form.File)
	}
}

func TestStandardBatchUpstreamClientDoesNotFollowBearerRedirect(t *testing.T) {
	targetReached := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/target" {
			targetReached <- struct{}{}
			response.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(response, request, "/target", http.StatusFound)
	}))
	defer server.Close()
	client := (StandardBatchProductionExecutor{Client: server.Client()}).standardUpstreamClient()
	request, err := http.NewRequest(http.MethodPost, server.URL+"/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer secret")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("redirect status = %d, want %d", response.StatusCode, http.StatusFound)
	}
	select {
	case <-targetReached:
		t.Fatal("Bearer request should not follow redirects")
	default:
	}
}

func TestStandardBatchReferenceURLsAreStableAndLimited(t *testing.T) {
	items := map[string]string{}
	for index := 9; index >= 0; index-- {
		items[fmt.Sprintf("image:%02d", index)] = fmt.Sprintf("https://cdn.example.com/%02d.png", index)
	}
	result, err := standardBatchReferenceURLs(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != maxStandardBatchReferences || result[0] != "https://cdn.example.com/00.png" {
		t.Fatalf("unexpected references: %#v", result)
	}
}

func TestStandardBatchRequestIDIsStableWithinRun(t *testing.T) {
	item := model.BatchProductionItem{ID: "item-a", RunNumber: 2}
	requestID := standardBatchRequestID(item)
	if requestID != standardBatchRequestID(item) {
		t.Fatal("request ID should be stable within the same run")
	}
	item.RunNumber++
	if requestID == standardBatchRequestID(item) {
		t.Fatal("request ID should change for a new run")
	}
}

func TestSelectStandardBatchModelChannelKeepsOriginalChannel(t *testing.T) {
	saved, err := repository.GetSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings := saved
	settings.Private.Channels = []model.ModelChannel{
		{Name: "channel-a", BaseURL: "https://a.example.com", APIKey: "key-a", Weight: 100, Enabled: true, Models: []model.ChannelModel{{Model: "image-model", UpstreamModel: "upstream-a", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}}}},
		{Name: "channel-b", BaseURL: "https://b.example.com", APIKey: "key-b", Weight: 1, Enabled: true, Models: []model.ChannelModel{{Model: "image-model", UpstreamModel: "upstream-b", Operations: []string{"generation"}, ResolutionTiers: []string{"1k"}}}},
	}
	if _, err := repository.SaveSettings(settings, "1"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = repository.SaveSettings(saved, "2") })
	task := model.GenerationTask{Model: "image-model", ChannelName: "channel-b", UpstreamModel: "upstream-b"}
	selection, err := selectStandardBatchModelChannel(PricingRequest{Model: "image-model", Modality: "image", Operation: "generation", ResolutionTier: "1k"}, &task, "request-a")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Channel.Name != task.ChannelName || selection.Model.UpstreamModel != task.UpstreamModel {
		t.Fatalf("unexpected resumed channel selection: %#v", selection)
	}
}

func TestStandardBatchExecutorReturnsResumedTaskOnPreflightFailure(t *testing.T) {
	user, _, organization := seedTenant(t, "batch-resume-preflight")
	setTestUserCredits(t, user.ID, 10)
	item := model.BatchProductionItem{ID: "item-batch-resume-preflight", RunNumber: 1}
	task, err := BeginGenerationTask(GenerationTaskInput{
		UserID: user.ID, OrganizationID: organization.ID, RequestID: standardBatchRequestID(item),
		Model: "image-model", UpstreamModel: "upstream-image", ChannelName: "channel-a",
		Path: "/images/generations", Modality: "image", Operation: "generation", ResolutionTier: "1k", Quantity: 1, Credits: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer FinishGenerationTask(task, model.GenerationTaskStatusFailed, "test cleanup")
	result, err := (StandardBatchProductionExecutor{}).Execute(context.Background(), BatchProductionExecution{
		Job: model.BatchProductionJob{OrganizationID: organization.ID, CreatedBy: user.ID},
		Item: item, Product: model.Product{Name: "测试商品"},
	})
	if err == nil {
		t.Fatal("expected invalid frozen preset to fail")
	}
	if result.GenerationTask == nil || result.GenerationTask.ID != task.ID {
		t.Fatalf("expected resumed generation task %q, got %#v", task.ID, result.GenerationTask)
	}
}
