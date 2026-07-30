package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
)

const (
	UserAPIKeyTokenPrefix = "ic_live_"
	userAPIKeyActiveLimit = 10
)

func ListUserAPIKeys(user model.AuthUser) ([]model.UserAPIKey, error) {
	organization, _, err := ResolveOrganizationAccess(user, user.OrganizationID)
	if err != nil {
		return nil, err
	}
	return repository.ListUserAPIKeys(organization.ID, user.ID)
}

func CreateUserAPIKey(user model.AuthUser, name string) (model.UserAPIKeyCredential, error) {
	organization, _, err := ResolveOrganizationAccess(user, user.OrganizationID)
	if err != nil {
		return model.UserAPIKeyCredential{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "默认密钥"
	}
	if utf8.RuneCountInString(name) > 50 {
		return model.UserAPIKeyCredential{}, safeMessageError{message: "密钥名称不能超过 50 个字符"}
	}
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return model.UserAPIKeyCredential{}, err
	}
	secret := UserAPIKeyTokenPrefix + base64.RawURLEncoding.EncodeToString(random)
	timestamp := now()
	item := model.UserAPIKey{
		ID:             newID("apikey"),
		OrganizationID: organization.ID,
		UserID:         user.ID,
		Name:           name,
		Prefix:         secret[:len(UserAPIKeyTokenPrefix)+8],
		KeyHash:        userAPIKeyHash(secret),
		Status:         model.UserAPIKeyStatusActive,
		CreatedAt:      timestamp,
		UpdatedAt:      timestamp,
	}
	item, err = repository.CreateUserAPIKey(item, userAPIKeyActiveLimit, newAuditLog(user.ID, organization.ID, "api_key.create", "api_key", item.ID, map[string]string{"name": item.Name, "prefix": item.Prefix}, timestamp))
	if errors.Is(err, repository.ErrUserAPIKeyLimit) {
		return model.UserAPIKeyCredential{}, safeMessageError{message: "每个企业最多保留 10 个有效 API Key"}
	}
	return model.UserAPIKeyCredential{UserAPIKey: item, Secret: secret}, err
}

func RevokeUserAPIKey(user model.AuthUser, id string) error {
	organization, _, err := ResolveOrganizationAccess(user, user.OrganizationID)
	if err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return safeMessageError{message: "API Key 不存在"}
	}
	timestamp := now()
	err = repository.RevokeUserAPIKey(organization.ID, user.ID, id, timestamp, newAuditLog(user.ID, organization.ID, "api_key.revoke", "api_key", id, map[string]string{"id": id}, timestamp))
	if errors.Is(err, repository.ErrUserAPIKeyNotFound) {
		return safeMessageError{message: "API Key 不存在或已撤销"}
	}
	return err
}

func AuthenticateUserAPIKey(secret string) (model.AuthUser, model.UserAPIKey, error) {
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, UserAPIKeyTokenPrefix) || len(secret) > 128 {
		return model.AuthUser{}, model.UserAPIKey{}, safeMessageError{message: "API Key 无效或已撤销"}
	}
	item, ok, err := repository.GetActiveUserAPIKeyByHash(userAPIKeyHash(secret))
	if err != nil {
		return model.AuthUser{}, model.UserAPIKey{}, err
	}
	if !ok {
		return model.AuthUser{}, model.UserAPIKey{}, safeMessageError{message: "API Key 无效或已撤销"}
	}
	user, ok, err := repository.GetUserByID(item.UserID)
	if err != nil {
		return model.AuthUser{}, model.UserAPIKey{}, err
	}
	if !ok || user.Status == model.UserStatusBan || user.Role == model.UserRoleGuest {
		return model.AuthUser{}, model.UserAPIKey{}, safeMessageError{message: "API Key 无效或已撤销"}
	}
	authUser := model.PublicUser(user)
	organization, _, err := ResolveOrganizationAccess(authUser, item.OrganizationID)
	if err != nil {
		return model.AuthUser{}, model.UserAPIKey{}, safeMessageError{message: "API Key 所属企业不可用"}
	}
	authUser.OrganizationID = organization.ID
	authUser = applyOrganizationCredits(authUser, organization)
	timestamp := now()
	cutoff := time.Now().UTC().Add(-5 * time.Minute).Format(timestampLayout)
	if item.LastUsedAt == "" || item.LastUsedAt < cutoff {
		_ = repository.TouchUserAPIKey(item.ID, cutoff, timestamp)
	}
	return authUser, item, nil
}

func userAPIKeyHash(secret string) string {
	hash := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(hash[:])
}
