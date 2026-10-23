package service

import (
	"testing"
	"time"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

func TestDeriveModelStatusFromOutcomes(t *testing.T) {
	latencyOK := int64(12_000)
	latencyHigh := int64(120_000)
	cases := []struct {
		name       string
		capacityOK bool
		known      bool
		rows       []repository.GenerationOutcomeRow
		latency    *int64
		want       string
	}{
		{name: "no capacity", capacityOK: false, known: true, want: model.ModelStatusUnavailable},
		{name: "unknown capacity", capacityOK: false, known: false, want: model.ModelStatusUnknown},
		{name: "capacity no traffic", capacityOK: true, known: true, want: model.ModelStatusOperational},
		{
			name: "healthy", capacityOK: true, known: true, latency: &latencyOK,
			rows: []repository.GenerationOutcomeRow{
				{Status: model.GenerationTaskStatusSuccess},
				{Status: model.GenerationTaskStatusSuccess},
				{Status: model.GenerationTaskStatusSuccess},
			},
			want: model.ModelStatusOperational,
		},
		{
			name: "busy", capacityOK: true, known: true,
			rows: []repository.GenerationOutcomeRow{
				{Status: model.GenerationTaskStatusFailed, ErrorMessage: "Adobe 上游服务暂时繁忙，请稍后重试"},
				{Status: model.GenerationTaskStatusSuccess},
				{Status: model.GenerationTaskStatusSuccess},
			},
			want: model.ModelStatusDegraded,
		},
		{
			name: "all failing", capacityOK: true, known: true,
			rows: []repository.GenerationOutcomeRow{
				{Status: model.GenerationTaskStatusFailed, ErrorMessage: "上游生成失败"},
				{Status: model.GenerationTaskStatusFailed, ErrorMessage: "provider execution failed"},
			},
			want: model.ModelStatusUnavailable,
		},
		{
			name: "high latency", capacityOK: true, known: true, latency: &latencyHigh,
			rows: []repository.GenerationOutcomeRow{
				{Status: model.GenerationTaskStatusSuccess},
				{Status: model.GenerationTaskStatusSuccess},
			},
			want: model.ModelStatusDegraded,
		},
		{
			name: "user fault ignored", capacityOK: true, known: true,
			rows: []repository.GenerationOutcomeRow{
				{Status: model.GenerationTaskStatusFailed, ErrorMessage: "内容审核未通过"},
				{Status: model.GenerationTaskStatusSuccess},
			},
			want: model.ModelStatusOperational,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := deriveModelStatus(tc.capacityOK, tc.known, tc.rows, tc.latency)
			if got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestBuildModelStatusItemTimelineAndLabel(t *testing.T) {
	nowStamp := time.Now().UTC().Format(timestampLayout)
	item := buildModelStatusItem("gpt-image-2", true, true, []repository.GenerationOutcomeRow{
		{Model: "gpt-image-2", Status: model.GenerationTaskStatusSuccess, DurationMs: 8_000, CreatedAt: nowStamp},
		{Model: "gpt-image-2", Status: model.GenerationTaskStatusFailed, ErrorMessage: "all accounts are at their concurrency limit, please try again shortly", DurationMs: 1_000, CreatedAt: nowStamp},
	}, time.Now().UTC())
	if item.Status != model.ModelStatusDegraded || item.Label != "繁忙" {
		t.Fatalf("item = %+v", item)
	}
	if len(item.Timeline) != modelStatusBucketCount {
		t.Fatalf("timeline len = %d", len(item.Timeline))
	}
	if item.LatencyMs == nil || *item.LatencyMs != 8_000 {
		t.Fatalf("latency = %#v", item.LatencyMs)
	}
	if item.Timeline[0].Status != model.ModelStatusDegraded {
		t.Fatalf("newest bucket = %+v", item.Timeline[0])
	}
}

func TestPlazaModelIDsSkipsTextAndDisabled(t *testing.T) {
	ids := plazaModelIDs(model.PublicModelChannelSetting{
		AvailableModels: []string{"gpt-image-2", "text-1", "off-1"},
		Models: []model.ModelDefinition{
			{ID: "gpt-image-2", Modality: "image", Enabled: true},
			{ID: "text-1", Modality: "text", Enabled: true},
			{ID: "off-1", Modality: "video", Enabled: false},
			{ID: "hidden", Modality: "audio", Enabled: true},
		},
	})
	if len(ids) != 1 || ids[0] != "gpt-image-2" {
		t.Fatalf("ids = %#v", ids)
	}
}