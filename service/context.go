package service

import (
	"context"

	"github.com/yypyyd/infinite-canvas/model"
)

type userContextKey struct{}

func WithUser(ctx context.Context, user model.AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func UserFromContext(ctx context.Context) (model.AuthUser, bool) {
	user, ok := ctx.Value(userContextKey{}).(model.AuthUser)
	return user, ok
}

type apiKeyContextKey struct{}

func WithAPIKey(ctx context.Context, apiKeyID string) context.Context {
	return context.WithValue(ctx, apiKeyContextKey{}, apiKeyID)
}

func APIKeyIDFromContext(ctx context.Context) string {
	apiKeyID, _ := ctx.Value(apiKeyContextKey{}).(string)
	return apiKeyID
}
