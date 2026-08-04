package service

import (
	"encoding/json"

	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/repository"
)

const maxUserPreferencesBytes = 64 << 10

var userPreferenceKeys = map[string]bool{"theme": true, "aiConfig": true, "imageQuickTools": true}
var aiPreferenceKeys = map[string]bool{
	"model": true, "imageModel": true, "videoModel": true, "textModel": true, "audioModel": true,
	"audioVoice": true, "audioFormat": true, "audioSpeed": true, "audioInstructions": true,
	"videoSeconds": true, "vquality": true, "videoGenerateAudio": true, "videoWatermark": true,
	"quality": true, "size": true, "count": true, "canvasImageCount": true,
}

func UserPreferences(userID string) (json.RawMessage, error) {
	item, found, err := repository.GetUserPreference(userID)
	if err != nil || !found || !json.Valid([]byte(item.Data)) {
		return json.RawMessage(`{}`), err
	}
	return json.RawMessage(item.Data), nil
}

func SaveUserPreferences(userID string, data json.RawMessage) (json.RawMessage, error) {
	if len(data) == 0 || len(data) > maxUserPreferencesBytes {
		return nil, safeMessageError{message: "用户偏好为空或过大"}
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil || value == nil {
		return nil, safeMessageError{message: "用户偏好格式不正确"}
	}
	for key, raw := range value {
		if !userPreferenceKeys[key] || !validUserPreferenceValue(key, raw) {
			return nil, safeMessageError{message: "用户偏好字段不正确"}
		}
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	item := model.UserPreference{UserID: userID, Data: string(normalized), UpdatedAt: now()}
	if err := repository.SaveUserPreference(item); err != nil {
		return nil, err
	}
	return normalized, nil
}

func validUserPreferenceValue(key string, raw json.RawMessage) bool {
	if key == "theme" {
		var theme string
		return json.Unmarshal(raw, &theme) == nil && (theme == "light" || theme == "dark")
	}
	if key == "aiConfig" {
		return validAIConfigPreferences(raw)
	}
	return validImageQuickToolsPreferences(raw)
}

func validAIConfigPreferences(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return false
	}
	for key, item := range value {
		var text string
		if !aiPreferenceKeys[key] || json.Unmarshal(item, &text) != nil {
			return false
		}
	}
	return true
}

func validImageQuickToolsPreferences(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return false
	}
	for key, item := range value {
		if key == "showLabels" {
			var showLabels bool
			if json.Unmarshal(item, &showLabels) != nil {
				return false
			}
			continue
		}
		if key != "ids" {
			return false
		}
		var ids []string
		if json.Unmarshal(item, &ids) != nil || len(ids) > 64 {
			return false
		}
	}
	return true
}
