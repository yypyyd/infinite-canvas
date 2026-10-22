package service

import (
	"context"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

const preferredProviderKey dispatchContextKey = "preferred-provider"

// WithPreferredProvider pins dispatch to one provider pool (chatgpt/grok/…).
// Empty provider is a no-op. Used when the main site selected a flat built-in
// provider channel and forwarded X-Preferred-Provider.
func WithPreferredProvider(ctx context.Context, provider string) context.Context {
	provider = normalizePool(provider)
	if provider == "" {
		return ctx
	}
	return context.WithValue(ctx, preferredProviderKey, provider)
}

// PreferredProviderFromContext returns the pinned provider pool, if any.
func PreferredProviderFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(preferredProviderKey).(string)
	return normalizePool(value)
}

// filterPreferredProviderRoutes keeps only routes for the preferred provider.
func filterPreferredProviderRoutes(routes []model.ModelRoute, preferred string) []model.ModelRoute {
	preferred = normalizePool(preferred)
	if preferred == "" || len(routes) == 0 {
		return routes
	}
	out := make([]model.ModelRoute, 0, len(routes))
	for _, route := range routes {
		if normalizePool(route.Provider) == preferred {
			out = append(out, route)
		}
	}
	return out
}
