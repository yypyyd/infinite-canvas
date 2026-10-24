package service

import (
	"testing"

	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

func TestPreferredProviderDisabledWhenPoolSwitchedOff(t *testing.T) {
	capability := []model.ModelRoute{
		{ID: "image.gpt-image-2.adobe", Provider: "adobe"},
		{ID: "image.gpt-image-2.chatgpt", Provider: "chatgpt"},
	}
	enabled := []model.ModelRoute{
		{ID: "image.gpt-image-2.chatgpt", Provider: "chatgpt"},
	}
	if !preferredProviderDisabled(capability, enabled, "adobe") {
		t.Fatal("adobe preferred + disabled should report ErrProviderDisabled")
	}
	if preferredProviderDisabled(capability, enabled, "chatgpt") {
		t.Fatal("chatgpt still enabled")
	}
	if preferredProviderDisabled(capability, enabled, "grok") {
		t.Fatal("unknown preferred is not disabled")
	}
	if preferredProviderDisabled(capability, capability, "adobe") {
		t.Fatal("adobe enabled should not report disabled")
	}
	if preferredProviderDisabled(capability, enabled, "") {
		t.Fatal("empty preferred is not disabled")
	}
}
