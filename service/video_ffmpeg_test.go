package service

import (
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/model"
)

func TestBuildVideoRenderFFmpegArgsBasic(t *testing.T) {
	timeline := model.VideoTimeline{
		Shots: []model.VideoTimelineShot{
			{ID: "s1", Source: model.VideoTimelineSource{Kind: "image", StorageKey: "image:a", SourceType: "upload"}, StartMs: 0, DurationMs: 5000, CropMode: "cover", TransitionToNext: model.VideoTimelineTransition{Type: "none"}},
			{ID: "s2", Source: model.VideoTimelineSource{Kind: "image", StorageKey: "image:b", SourceType: "upload"}, StartMs: 5000, DurationMs: 5000, CropMode: "contain", TransitionToNext: model.VideoTimelineTransition{Type: "fade", DurationMs: 500}},
			{ID: "s3", Source: model.VideoTimelineSource{Kind: "video", StorageKey: "video:c", SourceType: "upload"}, StartMs: 10000, DurationMs: 5000, TrimStartMs: 1000, CropMode: "cover", TransitionToNext: model.VideoTimelineTransition{Type: "none"}},
		},
		Subtitles: []model.VideoTimelineSubtitle{{ID: "sub1", Text: "Hello:World", StartMs: 0, EndMs: 2000, Style: "default", PositionY: 80}},
		Output:    model.VideoOutputSpec{Ratio: "16:9", Width: 1920, Height: 1080, FPS: 30, Format: "mp4", VideoCodec: "h264", AudioCodec: "aac"},
	}
	shotFiles := map[string]string{"s1": "/tmp/a.jpg", "s2": "/tmp/b.jpg", "s3": "/tmp/c.mp4"}
	args, err := buildVideoRenderFFmpegArgs("/tmp/work", timeline, shotFiles, "", "/tmp/out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "concat=n=3") {
		t.Fatalf("expected concat of 3 shots, got %s", joined)
	}
	if !strings.Contains(joined, "drawtext=") {
		t.Fatalf("expected drawtext subtitle, got %s", joined)
	}
	if !strings.Contains(joined, "libx264") || !strings.HasSuffix(joined, "/tmp/out.mp4") {
		t.Fatalf("unexpected codec/output: %s", joined)
	}
}

func TestEstimateVideoRenderCreditsFloor(t *testing.T) {
	credits := EstimateVideoRenderCredits(1500, model.VideoOutputSpec{Width: 1920, Height: 1080})
	if credits < 1 {
		t.Fatalf("credits=%d", credits)
	}
}
