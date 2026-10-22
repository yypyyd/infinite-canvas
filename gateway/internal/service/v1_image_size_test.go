package service

import (
	"context"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

func TestResolveImageSizeGPTImage2DefaultsTo1K(t *testing.T) {
	item := &model.ModelConfig{ID: "gpt-image-2"}
	tests := []struct {
		name string
		in   V1ImageRequest
		ar   string
		res  string
	}{
		{name: "omitted size", in: V1ImageRequest{}, ar: "1:1", res: "1K"},
		{name: "auto size", in: V1ImageRequest{Size: "auto"}, ar: "1:1", res: "1K"},
		{name: "explicit aspect ratio", in: V1ImageRequest{AspectRatio: "16:9"}, ar: "16:9", res: "1K"},
		{name: "explicit pixel size", in: V1ImageRequest{Size: "2048x2048"}, ar: "1:1", res: "2K"},
		{name: "explicit resolution", in: V1ImageRequest{Resolution: "2K"}, ar: "1:1", res: "2K"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ar, res := resolveImageSize(item, test.in)
			if ar != test.ar || res != test.res {
				t.Fatalf("resolveImageSize() = (%q, %q), want (%q, %q)", ar, res, test.ar, test.res)
			}
		})
	}
}

func TestResolveImageSizeKeepsGenericDefault(t *testing.T) {
	for _, modelID := range []string{"seedream-5.0-lite", "nano-banana-2"} {
		t.Run(modelID, func(t *testing.T) {
			item := &model.ModelConfig{ID: modelID}
			for _, size := range []string{"", "auto"} {
				ar, res := resolveImageSize(item, V1ImageRequest{Size: size})
				if ar != "1:1" || res != "2K" {
					t.Fatalf("size=%q: resolveImageSize() = (%q, %q), want (%q, %q)", size, ar, res, "1:1", "2K")
				}
			}
		})
	}
}

func TestGPTImageSizeRouteMatching(t *testing.T) {
	ctx := context.Background()
	models := repo.NewModelRepository(nil)
	item, err := models.Get(ctx, "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		size       string
		ratio      string
		resolution string
		supported  bool
	}{
		{"", "1:1", "1K", true}, {"auto", "1:1", "1K", true}, {"1024x1024", "1:1", "1K", true},
		{"1024x576", "16:9", "1K", true}, {"576x1024", "9:16", "1K", true},
		{"1024x768", "4:3", "1K", true}, {"768x1024", "3:4", "1K", true},
		{"1200x800", "3:2", "1K", true}, {"1536x1024", "3:2", "1K", true},
		// Explicit choices must not be silently downgraded or stretched.
		// The unified model is served by ChatGPT (1K), BytePlus and Adobe (1K-4K,
		// Adobe adds the wide ratios), so these resolve to some route now.
		{"1024x1536", "2:3", "1K", true},
		{"1280x1024", "5:4", "1K", true}, {"1024x1280", "4:5", "1K", true},
		{"1680x720", "21:9", "1K", true},
		{"2048x2048", "1:1", "2K", true}, {"4096x4096", "1:1", "4K", true},
		// No route accepts these ratios.
		{"1536x768", "2:1", "1K", false},
		{"1728x576", "3:1", "1K", false}, {"576x1728", "1:3", "1K", false},
	} {
		t.Run(test.size, func(t *testing.T) {
			ar, res := resolveImageSize(item, V1ImageRequest{Size: test.size})
			if ar != test.ratio || res != test.resolution {
				t.Fatalf("size=%q resolved=%s/%s, want %s/%s", test.size, ar, res, test.ratio, test.resolution)
			}
			for _, operation := range []string{"generation", "edit"} {
				references := 0
				if operation == "edit" {
					references = 1
				}
				routes, err := models.Routes().MatchRoutes(ctx, item.ID, model.RouteRequirements{
					Operation: operation, Ratio: ar, Resolution: res, ReferenceImages: references,
				})
				if err != nil {
					t.Fatal(err)
				}
				if (len(routes) > 0) != test.supported {
					t.Fatalf("operation=%s size=%q resolved=%s/%s routes=%d, want supported=%t", operation, test.size, ar, res, len(routes), test.supported)
				}
			}
		})
	}
}

func TestGPTImageSizePreservesExplicitOverrides(t *testing.T) {
	item, err := repo.NewModelRepository(nil).Get(context.Background(), "gpt-image-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range []V1ImageRequest{
		{Size: "720x1680", AspectRatio: "4:3", Resolution: "4K"},
		{Size: "auto", AspectRatio: "4:3", Resolution: "4K"},
	} {
		ar, res := resolveImageSize(item, in)
		if ar != "4:3" || res != "4K" {
			t.Fatalf("explicit overrides changed: resolved=%s/%s", ar, res)
		}
	}
}

func TestChatGPTRatioDoesNotExpandOtherRoutes(t *testing.T) {
	ctx := context.Background()
	models := repo.NewModelRepository(nil)
	// 3:2 is a ChatGPT-only ratio at 1K among the non-Adobe routes; at 4K only
	// routes that support both may match, never ChatGPT.
	routes, err := models.Routes().MatchRoutes(ctx, "gpt-image-2", model.RouteRequirements{
		Operation: "generation", Ratio: "3:2", Resolution: "4K",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range routes {
		if route.Provider != "adobe" {
			t.Fatalf("3:2 at 4K matched %s; capabilities must be evaluated per route", route.ID)
		}
	}
	if len(routes) != 1 {
		t.Fatalf("routes=%d, want only the Adobe route", len(routes))
	}
}
