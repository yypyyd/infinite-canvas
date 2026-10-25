package service

import (
	"strings"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// Channel selection is shared by interactive generation, batch execution and
// task maintenance. Every path sees the same channel set (saved channels plus
// flat built-in providers) and the same pick order: the highest-priority
// compatible sources first, weighted inside that group (see source_priority.go).

// loadRuntimeChannels returns saved channels plus flat built-in providers.
func loadRuntimeChannels() ([]model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	private := normalizePrivateSetting(settings.Private)
	return runtimeChannelsFor(private), nil
}

// channelSelectionsFor lists every compatible channel/model pair for request.
func channelSelectionsFor(request PricingRequest) ([]ModelChannelSelection, PricingRequest, error) {
	channels, err := loadRuntimeChannels()
	if err != nil {
		return nil, request, err
	}
	request = normalizePricingRequest(request)
	return modelChannelSelectionsForRequest(channels, request), request, nil
}

// runtimeChannelByName finds a usable runtime channel by name or id
// (including flat builtin providers and the legacy pool name).
func runtimeChannelByName(name string) (model.ModelChannel, bool, error) {
	name = strings.TrimSpace(name)
	channels, err := loadRuntimeChannels()
	if err != nil {
		return model.ModelChannel{}, false, err
	}
	for _, channel := range channels {
		if (channel.Name == name || channel.ID == name) && channel.BaseURL != "" && channel.APIKey != "" {
			return channel, true, nil
		}
	}
	if name == BuiltinPoolChannelName || name == BuiltinPoolChannelID {
		for _, channel := range channels {
			if IsBuiltinProviderChannel(channel) && channel.BaseURL != "" && channel.APIKey != "" {
				return channel, true, nil
			}
		}
	}
	return model.ModelChannel{}, false, nil
}
