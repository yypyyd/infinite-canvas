package gateway

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PoolModel is one enabled pool model with the capabilities 幻图 needs to
// offer it as a built-in channel model.
type PoolModel struct {
	ID                  string
	Name                string
	Modality            string // image, video or text
	Operations          []string
	AspectRatios        []string
	ResolutionTiers     []string
	Durations           []int
	MaxReferenceImages  int
	MaxReferenceVideos  int
	MaxReferenceAudios  int
	MaxReferenceMedia   int
	SupportsAudioOutput bool
}

const modelCacheTTL = 30 * time.Second

type modelCache struct {
	mu      sync.Mutex
	items   []PoolModel
	fetched time.Time
}

// Models returns the enabled pool models that have at least one usable route.
// Results are cached briefly because 幻图 reads them on every settings load.
func (m *Module) Models(ctx context.Context) ([]PoolModel, error) {
	m.models.mu.Lock()
	defer m.models.mu.Unlock()
	if m.models.items != nil && time.Since(m.models.fetched) < modelCacheTTL {
		return m.models.items, nil
	}
	entries, err := m.app.V1.ListModels(ctx, true)
	if err != nil {
		if m.models.items != nil {
			return m.models.items, nil // serve stale rather than drop models on a blip
		}
		return nil, err
	}
	items := make([]PoolModel, 0, len(entries))
	for _, entry := range entries {
		item := PoolModel{
			ID:                  stringValue(entry["id"]),
			Name:                stringValue(entry["name"]),
			Modality:            stringValue(entry["kind"]),
			Operations:          stringsValue(entry["operations"]),
			AspectRatios:        stringsValue(entry["supported_ratios"]),
			ResolutionTiers:     stringsValue(entry["supported_resolutions"]),
			Durations:           durationsValue(entry["supported_durations"]),
			MaxReferenceImages:  intValue(entry["max_reference_images"]),
			MaxReferenceVideos:  intValue(entry["max_reference_videos"]),
			MaxReferenceAudios:  intValue(entry["max_reference_audios"]),
			MaxReferenceMedia:   intValue(entry["max_reference_media"]),
			SupportsAudioOutput: entry["supports_audio_output"] == true,
		}
		if item.ID == "" {
			continue
		}
		if item.Name == "" {
			item.Name = item.ID
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	m.models.items, m.models.fetched = items, time.Now()
	return items, nil
}

// InvalidateModels drops the cached model list (after admin edits).
func (m *Module) InvalidateModels() {
	m.models.mu.Lock()
	m.models.items = nil
	m.models.mu.Unlock()
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

func stringsValue(v any) []string {
	items, _ := v.([]string)
	return append([]string(nil), items...)
}

func intValue(v any) int {
	n, _ := v.(int)
	return n
}

// durationsValue converts duration tiers such as "5", "5s" or "10秒" to
// seconds; tiers that are not whole seconds are dropped.
func durationsValue(v any) []int {
	out := []int{}
	for _, raw := range stringsValue(v) {
		value := strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(strings.ToLower(raw), "s"), "秒"))
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}
