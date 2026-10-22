package handler

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/yypyyd/infinite-canvas/gateway"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

func init() { service.PoolImageArchiver = archivePoolImageResult }

// archivePoolImageResult archives a pool image result found by maintenance
// (after a restart or once the request stopped waiting) the same way a live
// request does.
func archivePoolImageResult(ctx context.Context, task model.GenerationTask, created any, data []any) ([]byte, []string, error) {
	items := make([]map[string]any, 0, len(data))
	for _, raw := range data {
		if item, ok := raw.(map[string]any); ok {
			items = append(items, item)
		}
	}
	items = normalizeChatGPT2APIImageTaskData("http://"+gateway.InternalHost, "in-process", items)
	if len(items) == 0 {
		return nil, nil, errors.New("pool task has no image")
	}
	body, err := json.Marshal(map[string]any{"created": created, "data": items})
	if err != nil {
		return nil, nil, err
	}
	user := model.AuthUser{ID: task.UserID, OrganizationID: task.OrganizationID}
	archived, keys := archiveImageGenerationResponseWithOptions(ctx, user, task, body, "", "")
	return archived, keys, nil
}
