package service

import (
	"math/rand"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// Channel selection is shared by interactive generation, batch execution and
// task maintenance. Every path sees the same channel set (saved channels plus
// the built-in account pool) and the same pick order: the pool first when it
// can serve the request, otherwise a weighted pick among compatible channels.

// loadRuntimeChannels returns the saved channels plus the built-in pool.
func loadRuntimeChannels() ([]model.ModelChannel, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return nil, err
	}
	private := normalizePrivateSetting(settings.Private)
	return runtimeChannels(private.Channels, private.PoolModelAliases), nil
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

// pickChannelSelection prefers the built-in pool, then picks by channel
// weight. hit maps the total weight to a draw in [0,total); nil uses rand.
func pickChannelSelection(selections []ModelChannelSelection, hit func(total int) int) (ModelChannelSelection, bool) {
	if pool, ok := preferBuiltinPool(selections); ok {
		return pool, true
	}
	if len(selections) == 0 {
		return ModelChannelSelection{}, false
	}
	total := 0
	for _, selection := range selections {
		total += max(selection.Channel.Weight, 0)
	}
	if total <= 0 {
		return selections[0], true
	}
	if hit == nil {
		hit = rand.Intn
	}
	draw := hit(total)
	for _, selection := range selections {
		draw -= max(selection.Channel.Weight, 0)
		if draw < 0 {
			return selection, true
		}
	}
	return selections[0], true
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
