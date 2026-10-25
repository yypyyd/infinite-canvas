package gateway

import (
	"context"
)

// SetProvidersEnabled updates gateway providers_enabled for the given pools.
// Site「供应源」enable toggles call this so account scheduling stays aligned
// with BuiltinProviders.Enabled (the site-side route table).
func (m *Module) SetProvidersEnabled(ctx context.Context, enabled map[string]bool) error {
	if m == nil || m.app == nil || m.app.Admin == nil {
		return errAdminUnavailable
	}
	if len(enabled) == 0 {
		return nil
	}
	flags := make(map[string]any, len(enabled))
	for provider, on := range enabled {
		flags[provider] = on
	}
	_, err := m.app.Admin.UpdateSettings(ctx, map[string]any{"providers_enabled": flags})
	return err
}
