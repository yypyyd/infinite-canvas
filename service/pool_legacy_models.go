package service

import (
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

// legacyPoolModelIDs maps the provider-prefixed pool model IDs that existed
// before the unified catalog (gateway migration 000034) to their unified IDs.
var legacyPoolModelIDs = map[string]string{
	"chatgpt-gpt-image-2":      "gpt-image-2",
	"byteplus-gpt-image-2":     "gpt-image-2",
	"adobe-gpt-image-2":        "gpt-image-2",
	"byteplus-nano-banana-2":   "nano-banana-2",
	"adobe-nano-banana-2":      "nano-banana-2",
	"byteplus-nano-banana-pro": "nano-banana-pro",
	"adobe-nano-banana-pro":    "nano-banana-pro",
	"adobe-seedance-2.0":       "seedance-2.0",
	"adobe-seedance-2.0-fast":  "seedance-2.0-fast",
}

// unifiedPoolModelID returns the unified ID for a legacy prefixed ID.
func unifiedPoolModelID(id string) (string, bool) {
	unified, ok := legacyPoolModelIDs[strings.ToLower(strings.TrimSpace(id))]
	return unified, ok
}

// legacyRuleBetter decides which of two legacy rules for the same unified
// spec survives: a live (enabled, non-draft) rule beats a draft/disabled one,
// then the higher price wins so merging never lowers what users pay.
func legacyRuleBetter(candidate, current model.PricingRule) bool {
	live := func(rule model.PricingRule) bool { return rule.Enabled && !rule.Draft }
	if live(candidate) != live(current) {
		return live(candidate)
	}
	if candidate.Credits != current.Credits {
		return candidate.Credits > current.Credits
	}
	return candidate.ModelRatio > current.ModelRatio
}

// mergeLegacyPoolPricingRules moves pricing rules keyed by legacy prefixed
// IDs onto the unified ID. Pricing is per logical model + spec, so a rule
// already set on the unified ID always wins; otherwise one legacy rule per
// spec is kept (see legacyRuleBetter) and its remark notes the merge. It
// expects normalized rules and returns them in their original order.
func mergeLegacyPoolPricingRules(rules []model.PricingRule) ([]model.PricingRule, int) {
	unifiedKeys := map[string]bool{}
	for _, rule := range rules {
		if _, legacy := unifiedPoolModelID(rule.Model); !legacy {
			unifiedKeys[pricingSpecKey(rule.Model, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)] = true
		}
	}
	result := make([]model.PricingRule, 0, len(rules))
	slot := map[string]int{}
	sources := map[string][]string{}
	changed := 0
	for _, rule := range rules {
		unified, legacy := unifiedPoolModelID(rule.Model)
		if !legacy {
			result = append(result, rule)
			continue
		}
		changed++
		key := pricingSpecKey(unified, rule.Modality, rule.Operation, rule.Unit, rule.ResolutionTier)
		if unifiedKeys[key] {
			continue
		}
		sources[key] = append(sources[key], rule.Model)
		rule.Model = unified
		if index, ok := slot[key]; ok {
			if legacyRuleBetter(rule, result[index]) {
				result[index] = rule
			}
			continue
		}
		slot[key] = len(result)
		result = append(result, rule)
	}
	for key, index := range slot {
		note := fmt.Sprintf("merged from %s", strings.Join(sources[key], ", "))
		if result[index].Remark == "" {
			result[index].Remark = note
		} else if !strings.Contains(result[index].Remark, note) {
			result[index].Remark += " (" + note + ")"
		}
	}
	return result, changed
}

// MigrateLegacyPoolPricingRules persists the legacy-ID merge once at startup.
// Reads already see merged rules (normalizePricingRules applies it), this
// only makes the stored settings match. Safe to run repeatedly.
func MigrateLegacyPoolPricingRules() (int, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return 0, err
	}
	_, changed := mergeLegacyPoolPricingRules(normalizePricingRulesOnly(settings.Public.ModelChannel.PricingRules))
	if changed == 0 {
		return 0, nil
	}
	if _, err := SaveSettings(settings); err != nil {
		return 0, err
	}
	log.Printf("pricing: moved %d rule(s) from legacy pool model IDs onto unified IDs", changed)
	return changed, nil
}

// The pool's ChatGPT image model moved from gpt-image-2 to gpt-image-2.6
// (gateway migration 000048).
const (
	poolChatGPTImageModel         = "gpt-image-2.6"
	poolChatGPTImagePreviousModel = "gpt-image-2"
)

// MovePoolChatGPTImageModelSettings persists the site's settings for the renamed
// ChatGPT image model once at startup. Safe to run repeatedly.
func MovePoolChatGPTImageModelSettings() error {
	if gateway.Current() == nil {
		return nil
	}
	settings, err := repository.GetSettings()
	if err != nil || !movePoolChatGPTImageModel(&settings) {
		return err
	}
	if _, err := repository.SaveSettings(settings, now()); err != nil {
		return err
	}
	log.Printf("settings: moved %s model settings to %s", poolChatGPTImagePreviousModel, poolChatGPTImageModel)
	return nil
}

// movePoolChatGPTImageModel gives gpt-image-2.6 the model definition, prices and
// default image model configured for gpt-image-2, so the renamed model keeps its
// position, name and price. The gpt-image-2 entries stay only while another
// channel still serves that ID. It reports false and changes nothing when the
// new ID is already configured.
func movePoolChatGPTImageModel(settings *model.Settings) bool {
	channel := &settings.Public.ModelChannel
	for _, item := range channel.Models {
		if item.ID == poolChatGPTImageModel {
			return false
		}
	}
	for _, rule := range channel.PricingRules {
		if rule.Model == poolChatGPTImageModel {
			return false
		}
	}
	keep := slices.Contains(enabledChannelModels(runtimeChannelsFor(settings.Private)), poolChatGPTImagePreviousModel)
	moved := channel.DefaultImageModel == poolChatGPTImagePreviousModel
	if moved {
		channel.DefaultImageModel = poolChatGPTImageModel
	}
	models := make([]model.ModelDefinition, 0, len(channel.Models)+1)
	for _, item := range channel.Models {
		if item.ID == poolChatGPTImagePreviousModel {
			if keep {
				models = append(models, item)
			}
			item.ID, moved = poolChatGPTImageModel, true
			if item.Name == poolChatGPTImagePreviousModel {
				item.Name = poolChatGPTImageModel
			}
		}
		models = append(models, item)
	}
	rules := make([]model.PricingRule, 0, len(channel.PricingRules))
	for _, rule := range channel.PricingRules {
		if rule.Model == poolChatGPTImagePreviousModel {
			if keep {
				rules = append(rules, rule)
			}
			rule.Model, moved = poolChatGPTImageModel, true
		}
		rules = append(rules, rule)
	}
	channel.Models, channel.PricingRules = models, rules
	return moved
}
