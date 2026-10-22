package service

import (
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// Channel selection is shared by interactive generation, batch execution and
// task maintenance. Every path sees the same channel set (saved channels plus
// the built-in account pool) and the same pick order: the highest-priority
// compatible sources first, weighted inside that group (see source_priority.go).

// loadRuntimeChannels returns the saved channels plus the built-in pool.
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

// runtimeChannelByName finds a usable runtime channel (including the pool) by name.
func runtimeChannelByName(name string) (model.ModelChannel, bool, error) {
	channels, err := loadRuntimeChannels()
	if err != nil {
		return model.ModelChannel{}, false, err
	}
	for _, channel := range channels {
		if channel.Name == name && channel.BaseURL != "" && channel.APIKey != "" {
			return channel, true, nil
		}
	}
	return model.ModelChannel{}, false, nil
}
