package service

import (
	"context"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/adobe"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/byteplus"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/chatgpt"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/grok"
	"github.com/yypyyd/infinite-canvas/gateway/internal/provider/proxypool"
)

const (
	settingProxyURL        = "proxy.url"
	settingExtractAPI      = "proxy.extract_api"
	settingProviderProxyNS = "proxy."
)

// extractManager is the process-wide residential lease table. Configure is
// idempotent: unchanged extract APIs keep in-memory assignments.
var extractManager = proxypool.NewManager()

// providerProxySnapshot is one read of the administrator residential routes.
type providerProxySnapshot struct {
	Default           string
	ByProvider        map[string]string
	Stored            map[string]string
	DefaultExtract    string
	ExtractByProvider map[string]string
	ExtractStored     map[string]string
}

func providerProxySettingKey(pool string) string {
	return settingProviderProxyNS + strings.ToLower(strings.TrimSpace(pool)) + ".url"
}

func providerExtractSettingKey(pool string) string {
	return settingProviderProxyNS + strings.ToLower(strings.TrimSpace(pool)) + ".extract_api"
}

func proxyAwareProviders() []string {
	return []string{"chatgpt", "grok", "adobe", "byteplus"}
}

// inheritDefaultProxy reports whether an empty per-channel URL should fall back
// to proxy.url. Adobe and BytePlus stay direct unless their own URL is set.
func inheritDefaultProxy(pool string) bool {
	switch strings.ToLower(strings.TrimSpace(pool)) {
	case "chatgpt", "grok":
		return true
	default:
		return false
	}
}

func inheritDefaultExtract(pool string) bool {
	return inheritDefaultProxy(pool)
}

func resolveProviderProxy(pool, stored, fallback string) string {
	stored = strings.TrimSpace(stored)
	if stored != "" {
		return stored
	}
	if inheritDefaultProxy(pool) {
		return strings.TrimSpace(fallback)
	}
	return ""
}

func resolveProviderExtract(pool, stored, fallback string) string {
	stored = strings.TrimSpace(stored)
	if stored != "" {
		return stored
	}
	if inheritDefaultExtract(pool) {
		return strings.TrimSpace(fallback)
	}
	return ""
}

func loadProviderProxies(ctx context.Context, settings settingValueReader) (providerProxySnapshot, error) {
	snap := providerProxySnapshot{
		ByProvider:        map[string]string{},
		Stored:            map[string]string{},
		ExtractByProvider: map[string]string{},
		ExtractStored:     map[string]string{},
	}
	if settings == nil {
		return snap, nil
	}
	fallback, err := settings.GetValue(ctx, settingProxyURL)
	if err != nil {
		return snap, err
	}
	snap.Default = strings.TrimSpace(fallback)
	defaultExtract, err := settings.GetValue(ctx, settingExtractAPI)
	if err != nil {
		return snap, err
	}
	snap.DefaultExtract = strings.TrimSpace(defaultExtract)
	for _, pool := range proxyAwareProviders() {
		raw, getErr := settings.GetValue(ctx, providerProxySettingKey(pool))
		if getErr != nil {
			return snap, getErr
		}
		raw = strings.TrimSpace(raw)
		snap.Stored[pool] = raw
		snap.ByProvider[pool] = resolveProviderProxy(pool, raw, snap.Default)
		extract, getErr := settings.GetValue(ctx, providerExtractSettingKey(pool))
		if getErr != nil {
			return snap, getErr
		}
		extract = strings.TrimSpace(extract)
		snap.ExtractStored[pool] = extract
		snap.ExtractByProvider[pool] = resolveProviderExtract(pool, extract, snap.DefaultExtract)
	}
	return snap, nil
}

func assignProviderProxies(snap providerProxySnapshot, chatgptClient *chatgpt.Client, grokClient *grok.Client, adobeClient *adobe.Client, byteplusClient *byteplus.Client) {
	for _, pool := range proxyAwareProviders() {
		extractManager.Configure(pool, snap.ExtractByProvider[pool])
	}
	if chatgptClient != nil {
		chatgptClient.SetProxy(snap.ByProvider["chatgpt"])
		chatgptClient.SetAssigner(extractManager.Bind("chatgpt"))
	}
	if grokClient != nil {
		grokClient.SetProxy(snap.ByProvider["grok"])
		grokClient.SetAssigner(extractManager.Bind("grok"))
	}

	if adobeClient != nil {
		adobeClient.SetProxy(snap.ByProvider["adobe"])
		adobeClient.SetAssigner(extractManager.Bind("adobe"))
	}
	if byteplusClient != nil {
		byteplusClient.SetProxy(snap.ByProvider["byteplus"])
		byteplusClient.SetAssigner(extractManager.Bind("byteplus"))
	}
}
