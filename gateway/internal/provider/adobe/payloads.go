package adobe

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var imageSeedCounter atomic.Uint64

func init() {
	imageSeedCounter.Store(uint64(time.Now().UnixNano()))
}

// nextImageSeed is process-unique for 999,999 consecutive calls. Adobe accepts
// seeds in this range; a monotonic atomic counter avoids the duplicate seeds
// produced by the old second-resolution timestamp during immediate retries.
func nextImageSeed() int {
	return int(imageSeedCounter.Add(1) % 999999)
}

type modelSpec struct {
	UpstreamModelID      string
	UpstreamModelVersion string
}

// Firefly's 1K presets. Resolution scales pixel dimensions; quality selects
// detailLevel independently and must never be represented as a resolution.
var gptImage2Size = map[string][2]int{
	"21:9": {1584, 672}, "16:9": {1376, 768}, "5:4": {1152, 928},
	"4:3": {1200, 896}, "3:2": {1264, 848}, "1:1": {1024, 1024},
	"4:5": {928, 1152}, "3:4": {896, 1200}, "2:3": {848, 1264},
	"9:16": {768, 1376},
}

var gptImage15Size = map[string][2]int{
	"1:1": {1024, 1024},
	"3:2": {1536, 1024},
	"2:3": {1024, 1536},
}

var defaultSize = map[string]map[string][2]int{
	"1K": {"1:1": {1024, 1024}, "1:8": {384, 3072}, "1:4": {512, 2048}, "16:9": {1360, 768}, "9:16": {768, 1360}, "4:1": {2048, 512}, "4:3": {1152, 864}, "3:4": {864, 1152}, "8:1": {3072, 384}},
	"2K": {"1:1": {2048, 2048}, "1:8": {768, 6144}, "1:4": {1024, 4096}, "16:9": {2752, 1536}, "9:16": {1536, 2752}, "4:1": {4096, 1024}, "4:3": {2048, 1536}, "3:4": {1536, 2048}, "8:1": {6144, 768}},
	"4K": {"1:1": {4096, 4096}, "1:8": {1536, 12288}, "1:4": {2048, 8192}, "16:9": {5504, 3072}, "9:16": {3072, 5504}, "4:1": {8192, 2048}, "4:3": {4096, 3072}, "3:4": {3072, 4096}, "8:1": {12288, 1536}},
}

func ResolveModelSpec(modelID string) modelSpec {
	// Adobe closed catalog only ships GPT Image 2 (firefly-gpt-image-2).
	// Legacy 1.5 / bare firefly-gpt-image ids remain for historical rows.
	switch modelID {
	case "firefly-gpt-image-1.5":
		return modelSpec{UpstreamModelID: "gpt-image", UpstreamModelVersion: "1.5"}
	case "firefly-gpt-image", "firefly-gpt-image-2":
		return modelSpec{UpstreamModelID: "gpt-image", UpstreamModelVersion: "2"}
	default:
		return modelSpec{UpstreamModelID: "gpt-image", UpstreamModelVersion: "2"}
	}
}

// buildImage5Payload builds the Adobe Firefly Image 5 request. It uses a distinct
// schema from the firefly-3p models: NO modelId/size, a top-level aspectRatio
// string label and a resolutionLevel (1K→1MP, 2K→4MP). Mirrors a captured
// working image-v5.ff.adobe.io request.
func buildImage5Payload(prompt, aspectRatio, resolution string, blobIDs []string) map[string]any {
	p := map[string]any{
		"n":                    1,
		"seeds":                []int{nextImageSeed()},
		"output":               map[string]any{"storeInputs": true},
		"prompt":               prompt,
		"referenceBlobs":       []any{},
		"modelSpecificPayload": map[string]any{"locale": "en-US", "prompt_reasoner": "quality"},
		"modelVersion":         "image5",
		"resolutionLevel":      image5ResolutionLevel(resolution),
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
	}
	if len(blobIDs) > 0 {
		// Instruct-edit: aspect ratio is derived from the reference image; sending
		// aspectRatio is rejected with a validation_error.
		p["referenceBlobs"] = blobRefs(blobIDs, "general")
	} else {
		p["aspectRatio"] = defaultString(aspectRatio, "1:1")
	}
	return p
}

// image5ResolutionLevel maps the UI resolution tier to Image 5's megapixel level.
func image5ResolutionLevel(resolution string) string {
	switch strings.ToUpper(strings.TrimSpace(resolution)) {
	case "1K":
		return "1MP"
	case "2K":
		return "4MP"
	default:
		return "4MP"
	}
}

func BuildImagePayloadCandidates(modelID, prompt, aspectRatio, outputResolution, quality string, blobIDs []string) []map[string]any {
	spec := ResolveModelSpec(modelID)
	ratio := defaultString(aspectRatio, "1:1")
	resolution := defaultString(outputResolution, "1K")

	switch spec.UpstreamModelID {
	case "gpt-image":
		if spec.UpstreamModelVersion == "1.5" {
			return buildGPTImage15Payloads(spec, prompt, ratio, blobIDs)
		}
		return buildGPTImagePayloads(spec, prompt, ratio, resolution, quality, blobIDs)
	default:
		return buildDefaultPayloads(spec, prompt, ratio, resolution, blobIDs)
	}
}

func buildGPTImage15Payloads(spec modelSpec, prompt, ratio string, blobIDs []string) []map[string]any {
	size, ok := gptImage15Size[ratio]
	if !ok {
		size = gptImage15Size["1:1"]
	}
	base := map[string]any{
		"modelId":              spec.UpstreamModelID,
		"modelVersion":         spec.UpstreamModelVersion,
		"n":                    1,
		"prompt":               prompt,
		"size":                 map[string]any{"width": size[0], "height": size[1]},
		"seeds":                []int{nextImageSeed()},
		"output":               map[string]any{"storeInputs": true},
		"referenceBlobs":       []any{},
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"modelSpecificPayload": map[string]any{},
	}
	if len(blobIDs) == 0 {
		return []map[string]any{base}
	}
	edited := cloneMap(base)
	edited["referenceBlobs"] = blobRefs(blobIDs, "subject")
	return []map[string]any{edited}
}

func buildGPTImagePayloads(spec modelSpec, prompt, ratio, resolution, quality string, blobIDs []string) []map[string]any {
	size := gptImage2Size[ratio]
	if size == [2]int{} {
		size = gptImage2Size["1:1"]
	}
	if strings.EqualFold(strings.TrimSpace(resolution), "2K") {
		// 2K is a 2048-pixel long edge, not twice the 1K preset's edges.
		// Doubling portrait/wide presets exceeds the free tier's accepted size.
		longEdge := max(size[0], size[1])
		size[0] = (size[0]*2048/longEdge + 8) / 16 * 16
		size[1] = (size[1]*2048/longEdge + 8) / 16 * 16
	}
	base := map[string]any{
		"modelId":              spec.UpstreamModelID,
		"modelVersion":         spec.UpstreamModelVersion,
		"n":                    1,
		"prompt":               prompt,
		"size":                 map[string]any{"width": size[0], "height": size[1]},
		"seeds":                []int{nextImageSeed()},
		"output":               map[string]any{"storeInputs": true},
		"referenceBlobs":       []any{},
		"generationMetadata":   map[string]any{"module": "text2image", "submodule": "ff-image-generate"},
		"modelSpecificPayload": map[string]any{},
		"generationSettings":   map[string]any{"detailLevel": gptImage2DetailLevel(quality)},
	}
	if len(blobIDs) == 0 {
		return []map[string]any{base}
	}
	subject := cloneMap(base)
	subject["referenceBlobs"] = blobRefs(blobIDs, "subject")
	return []map[string]any{subject}
}

func gptImage2DetailLevel(quality string) int {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "low":
		return 1
	case "high":
		return 5
	default:
		return 3
	}
}

func buildDefaultPayloads(spec modelSpec, prompt, ratio, resolution string, blobIDs []string) []map[string]any {
	size := getSize(defaultSize, resolution, ratio, "16:9")
	// Shape mirrors a captured working firefly.adobe.com request exactly: top-level
	// size object, modelSpecificPayload only {parameters:{addWatermark:false}},
	// groundSearch:false, module "text2image" (even with a reference blob). NO
	// skipCai and NO modelSpecificPayload.aspectRatio — sending those got 403.
	base := map[string]any{
		"modelId":      spec.UpstreamModelID,
		"modelVersion": spec.UpstreamModelVersion,
		"n":            1,
		"prompt":       prompt,
		"size":         map[string]any{"width": size[0], "height": size[1]},
		"seeds":        []int{nextImageSeed()},
		"groundSearch": false,
		"output":       map[string]any{"storeInputs": true},
		"generationMetadata": map[string]any{
			"module":    "text2image",
			"submodule": "ff-image-generate",
		},
		"modelSpecificPayload": map[string]any{
			"parameters": map[string]any{"addWatermark": false},
		},
	}
	if len(blobIDs) == 0 {
		base["referenceBlobs"] = []any{}
		return []map[string]any{base}
	}
	edited := cloneMap(base)
	edited["referenceBlobs"] = blobRefs(blobIDs, "general")
	return []map[string]any{edited}
}

func getSize(table map[string]map[string][2]int, resolution, ratio, fallbackRatio string) [2]int {
	level := defaultString(resolution, "2K")
	levelTable, ok := table[level]
	if !ok {
		levelTable = table["2K"]
	}
	size, ok := levelTable[ratio]
	if !ok {
		size = levelTable[fallbackRatio]
	}
	return size
}

func blobRefs(ids []string, usage string) []any {
	out := make([]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"id": id, "usage": usage})
	}
	return out
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// VideoInputs carries uploaded blob IDs and the Firefly generateAudio flag for
// partner video models (currently Kling 4.0 only).
type VideoInputs struct {
	ImageBlobIDs  []string
	VideoBlobIDs  []string
	AudioBlobIDs  []string
	GenerateAudio bool
}

// BuildVideoPayload builds the firefly-3p generate-async body for the supported
// Adobe video engines. Only Kling 4.0 (kling-v4) is restored; other partner
// video stacks stay out of the closed catalog.
func BuildVideoPayload(engine, prompt, aspectRatio string, durationSeconds int, resolution, referenceMode, upstreamModel string, in VideoInputs) map[string]any {
	_ = upstreamModel
	seedVal := int(time.Now().Unix()) % 999999
	engine = defaultString(engine, "kling-v4")
	resolution = defaultString(resolution, "720p")
	aspectRatio = defaultString(aspectRatio, "16:9")
	if durationSeconds <= 0 {
		durationSeconds = 8
	}

	switch engine {
	case "kling-v4":
		aspectRatio = klingAspectRatio(aspectRatio)
		isImage := len(in.ImageBlobIDs) > 0
		isVideo := len(in.VideoBlobIDs) > 0
		module := "text2video"
		if isVideo {
			module = "video2video"
		} else if isImage && referenceMode == "frame" {
			module = "image2video"
		}
		// The Firefly model-version keys are hyphenated. The underscored
		// firefly_3p:external:* identifiers are BKS routing IDs, not versions.
		modelVersion := "kling-v4-omni-ref"
		if isVideo {
			modelVersion = "kling-v4-omni-ref-v2v-create"
		}
		payload := map[string]any{
			"modelId":      "kling",
			"modelVersion": modelVersion,
			"resolution":   resolution,
			"duration":     durationSeconds,
			"seeds":        []int{seedVal},
			"prompt":       prompt,
			// Kling 4 requires audio; false is rejected by its request schema.
			"generateAudio":      true,
			"generationSettings": map[string]any{"aspectRatio": aspectRatio},
			"generationMetadata": map[string]any{"module": module, "submodule": "ff-video-generate"},
			"output":             map[string]any{"storeInputs": true},
			"referenceBlobs":     []any{},
		}
		refs := make([]any, 0, len(in.ImageBlobIDs)+len(in.VideoBlobIDs))
		for _, id := range in.VideoBlobIDs {
			refs = append(refs, map[string]any{"id": id, "usage": "source"})
		}
		for index, id := range in.ImageBlobIDs {
			ref := map[string]any{"id": id, "usage": "general"}
			if referenceMode == "frame" {
				ref["usage"], ref["order"] = "frame", index+1
			} else {
				ref["mention"] = map[string]any{"id": fmt.Sprintf("image%d", index+1)}
			}
			refs = append(refs, ref)
		}
		payload["referenceBlobs"] = refs
		return payload
	default:
		// Unknown engines fall through to Kling 4.0 t2v so a misconfigured route
		// still produces a recognizable payload rather than an empty body.
		return BuildVideoPayload("kling-v4", prompt, aspectRatio, durationSeconds, resolution, referenceMode, upstreamModel, in)
	}
}

// KlingVideoModelVersionCandidates returns Firefly 3P modelVersion slugs.
// Firefly's UI modelVersion is distinct from its firefly_3p:external BKS id.
// Keep the route on Kling 4.0 rather than silently falling back to Kling 3.
func KlingVideoModelVersionCandidates(preferred string, hasVideo bool) []string {
	out := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(preferred)
	if hasVideo {
		add("kling-v4-omni-ref-v2v-create")
	} else {
		add("kling-v4-omni-ref")
	}
	return out
}

func klingAspectRatio(aspectRatio string) string {
	switch strings.ToLower(strings.TrimSpace(aspectRatio)) {
	case "", "auto":
		return "16:9"
	case "16:9", "1:1", "9:16":
		return strings.TrimSpace(aspectRatio)
	default:
		return "16:9"
	}
}

func videoSize(aspectRatio, resolution string) map[string]any {
	ratio := strings.TrimSpace(aspectRatio)
	if ratio == "" || strings.EqualFold(ratio, "auto") {
		ratio = "16:9"
	}
	table720 := map[string][2]int{
		"21:9": {1280, 548},
		"16:9": {1280, 720},
		"1:1":  {720, 720},
		"9:16": {720, 1280},
	}
	table1080 := map[string][2]int{
		"21:9": {1920, 822},
		"16:9": {1920, 1080},
		"1:1":  {1080, 1080},
		"9:16": {1080, 1920},
	}
	table := table720
	if strings.EqualFold(resolution, "1080p") {
		table = table1080
	}
	size, ok := table[ratio]
	if !ok {
		size = table["16:9"]
	}
	return map[string]any{"width": size[0], "height": size[1]}
}
