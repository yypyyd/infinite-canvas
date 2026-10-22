package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
	"github.com/yypyyd/infinite-canvas/gateway/internal/repo"
)

func matchedProviders(t *testing.T, logicalID string, req model.RouteRequirements) []string {
	t.Helper()
	routes, err := repo.NewModelRepository(nil).Routes().MatchRoutes(context.Background(), logicalID, req)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(routes))
	for _, route := range routes {
		out = append(out, route.Provider)
	}
	sort.Strings(out)
	return out
}

func TestCapabilityRoutingFourKSkipsChatGPT(t *testing.T) {
	got := matchedProviders(t, "gpt-image-2", model.RouteRequirements{Operation: "generation", Ratio: "1:1", Resolution: "4K"})
	if strings.Join(got, ",") != "adobe,byteplus" {
		t.Fatalf("4K providers = %v, want [adobe byteplus]", got)
	}
	got = matchedProviders(t, "gpt-image-2", model.RouteRequirements{Operation: "generation", Ratio: "1:1", Resolution: "1K"})
	if strings.Join(got, ",") != "adobe,byteplus,chatgpt" {
		t.Fatalf("1K providers = %v, want all three", got)
	}
}

func TestCapabilityRoutingWideRatioOnlyAdobe(t *testing.T) {
	for _, logicalID := range []string{"gpt-image-2", "nano-banana-2", "nano-banana-pro"} {
		got := matchedProviders(t, logicalID, model.RouteRequirements{Operation: "generation", Ratio: "21:9", Resolution: "2K"})
		if strings.Join(got, ",") != "adobe" {
			t.Fatalf("%s 21:9 providers = %v, want [adobe]", logicalID, got)
		}
	}
}

// An unsupported combination must fail with ErrUnsupportedParams during route
// matching. The service has no pools or accounts wired in, so any attempt to
// reach an upstream would panic rather than return this error.
func TestCapabilityRoutingUnsupportedComboFailsBeforeUpstream(t *testing.T) {
	s := &V1Service{models: repo.NewModelRepository(nil)}
	logical := &model.ModelConfig{ID: "gpt-image-2"}
	for _, test := range []struct{ ratio, resolution string }{{"2:1", "1K"}, {"3:1", "4K"}} {
		_, _, err := s.dispatchImageRoutes(context.Background(), "evt-test", logical, V1ImageRequest{Resolution: test.resolution}, test.ratio, test.resolution, true)
		if !errors.Is(err, ErrUnsupportedParams) {
			t.Fatalf("%s/%s err = %v, want ErrUnsupportedParams", test.ratio, test.resolution, err)
		}
	}
	if _, err := s.firstAvailableRoute(context.Background(), "gpt-image-2", "image", model.RouteRequirements{Operation: "generation", Ratio: "2:1", Resolution: "1K"}); !errors.Is(err, ErrUnsupportedParams) {
		t.Fatalf("preflight err = %v, want ErrUnsupportedParams", err)
	}
}

// gpt-image-2 used to force 1K for every request with no size. The default is
// now chosen per route: ChatGPT only offers 1K, BytePlus/Adobe keep 2K.
func TestDefaultImageResolutionChosenPerRoute(t *testing.T) {
	routes, err := repo.NewModelRepository(nil).Routes().ListRoutes(context.Background(), "gpt-image-2", true)
	if err != nil {
		t.Fatal(err)
	}
	req := model.RouteRequirements{Operation: "generation", Ratio: "1:1"}
	want := map[string]string{"chatgpt": "1K", "byteplus": "2K", "adobe": "2K"}
	if len(routes) != len(want) {
		t.Fatalf("routes=%d, want %d", len(routes), len(want))
	}
	for _, route := range routes {
		if got := routeImageResolution(route, req, "2K", true); got != want[route.Provider] {
			t.Fatalf("%s defaulted resolution = %s, want %s", route.Provider, got, want[route.Provider])
		}
		if got := routeImageResolution(route, req, "4K", false); got != "4K" {
			t.Fatalf("%s explicit resolution changed to %s", route.Provider, got)
		}
	}
	for _, in := range []V1ImageRequest{{}, {Size: "auto"}, {AspectRatio: "16:9"}} {
		if !imageResolutionDefaulted(in) {
			t.Fatalf("%+v should use the route default", in)
		}
	}
	for _, in := range []V1ImageRequest{{Size: "1024x1024"}, {Resolution: "1K"}} {
		if imageResolutionDefaulted(in) {
			t.Fatalf("%+v is explicit", in)
		}
	}
}

func TestPoolImageRoutesCapImagesPerRequest(t *testing.T) {
	for _, def := range model.CanonicalRoutingCatalog() {
		for _, route := range def.Routes {
			if !strings.HasPrefix(route.ID, "image.") {
				continue
			}
			for _, profile := range model.DecodeCapabilityProfiles(route.Capabilities) {
				if profile.MaxImagesPerRequest != 1 {
					t.Fatalf("%s max_images_per_request = %d, want 1", route.ID, profile.MaxImagesPerRequest)
				}
			}
		}
	}
}

func TestModelEntryReportsCapabilityUnion(t *testing.T) {
	ctx := context.Background()
	routes, err := repo.NewModelRepository(nil).Routes().ListRoutes(ctx, "gpt-image-2", true)
	if err != nil {
		t.Fatal(err)
	}
	entry := v1LogicalModelEntry(model.LogicalModel{ID: "gpt-image-2", Kind: "image"}, routes, 0, true)
	res := strings.Join(entry["supported_resolutions"].([]string), ",")
	if res != "1K,2K,4K" {
		t.Fatalf("resolutions=%s, want union 1K,2K,4K", res)
	}
	if !strings.Contains(","+strings.Join(entry["supported_ratios"].([]string), ",")+",", ",21:9,") {
		t.Fatal("union must include the Adobe-only 21:9 ratio")
	}
	if entry["max_images_per_request"] != 1 {
		t.Fatalf("max_images_per_request=%v, want 1", entry["max_images_per_request"])
	}
	uncapped := model.ModelRoute{Capabilities: datatypesJSON(t, []model.CapabilityProfile{{Operations: []string{"generation"}}})}
	entry = v1LogicalModelEntry(model.LogicalModel{ID: "x", Kind: "image"}, append(routes, uncapped), 0, true)
	if entry["max_images_per_request"] != 0 {
		t.Fatalf("an uncapped route must lift the cap, got %v", entry["max_images_per_request"])
	}
}

func datatypesJSON(t *testing.T, profiles []model.CapabilityProfile) []byte {
	t.Helper()
	raw, err := json.Marshal(profiles)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
