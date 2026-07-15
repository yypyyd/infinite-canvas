package service

import (
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/basketikun/infinite-canvas/config"
	"github.com/basketikun/infinite-canvas/model"
	"github.com/basketikun/infinite-canvas/repository"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type TokenClaims struct {
	UserID   string         `json:"userId"`
	Username string         `json:"username"`
	Role     model.UserRole `json:"role"`
	jwt.RegisteredClaims
}

func EnsureDefaultAdmin() error {
	if strings.TrimSpace(config.Cfg.AdminUsername) == "" || strings.TrimSpace(config.Cfg.AdminPassword) == "" {
		return nil
	}
	WarnDefaultSecurityConfig()
	hasAdmin, err := repository.HasAdmin()
	if err != nil || hasAdmin {
		return err
	}
	hash, err := hashPassword(config.Cfg.AdminPassword)
	if err != nil {
		return err
	}
	_, err = repository.SaveUser(model.User{
		ID:        newID("user"),
		Username:  strings.TrimSpace(config.Cfg.AdminUsername),
		Password:  hash,
		Role:      model.UserRoleAdmin,
		Group:     "default",
		AffCode:   newAffCode(),
		Status:    model.UserStatusActive,
		CreatedAt: now(),
		UpdatedAt: now(),
	})
	return err
}

func Register(username string, email string, code string, password string) (model.AuthSession, error) {
	settings, err := repository.GetSettings()
	if err != nil {
		return model.AuthSession{}, err
	}
	normalizedSettings := normalizeSettings(settings)
	if normalizedSettings.Public.Auth.AllowRegister != nil && !*normalizedSettings.Public.Auth.AllowRegister {
		return model.AuthSession{}, safeMessageError{message: "当前未开放注册"}
	}
	username = strings.TrimSpace(username)
	if strings.ContainsAny(username, " \t\r\n") {
		return model.AuthSession{}, safeMessageError{message: "用户名不能包含空格"}
	}
	if username == "" || password == "" {
		return model.AuthSession{}, safeMessageError{message: "用户名和密码不能为空"}
	}
	email, err = normalizeEmailAddress(email)
	if err != nil {
		return model.AuthSession{}, safeMessageError{message: "请输入有效的电子邮箱"}
	}
	if err := validateRegistrationEmail(email, normalizedSettings.Public.Auth); err != nil {
		return model.AuthSession{}, err
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return model.AuthSession{}, safeMessageError{message: "请输入 6 位邮箱验证码"}
	}
	if _, ok, err := repository.GetUserByUsername(username); err != nil || ok {
		if err != nil {
			return model.AuthSession{}, err
		}
		return model.AuthSession{}, safeMessageError{message: "用户名已存在"}
	}
	if _, ok, err := repository.GetUserByEmail(email); err != nil || ok {
		if err != nil {
			return model.AuthSession{}, err
		}
		return model.AuthSession{}, safeMessageError{message: "该邮箱已注册"}
	}
	verificationID := emailVerificationID(email)
	verification, ok, err := repository.GetEmailVerification(verificationID)
	if err != nil {
		return model.AuthSession{}, err
	}
	codeHash := emailCodeHash(email, code)
	if !ok || verification.UsedAt != "" || verification.Attempts >= emailMaxAttempts {
		return model.AuthSession{}, safeMessageError{message: "邮箱验证码无效或已过期"}
	}
	expiresAt, parseErr := time.Parse(time.RFC3339, verification.ExpiresAt)
	if parseErr != nil || !expiresAt.After(time.Now()) || !verifyEmailCodeHash(verification.CodeHash, codeHash) {
		_ = repository.IncrementEmailVerificationAttempts(verificationID, verification.CodeHash)
		return model.AuthSession{}, safeMessageError{message: "邮箱验证码无效或已过期"}
	}
	hash, err := hashPassword(password)
	if err != nil {
		return model.AuthSession{}, err
	}
	createdAt := now()
	emailKey := email
	user, err := createRegisteredUser(model.User{
		ID:        newID("user"),
		Username:  username,
		Password:  hash,
		Email:     email,
		EmailKey:  &emailKey,
		Role:      model.UserRoleUser,
		Group:     "default",
		AffCode:   newAffCode(),
		Status:    model.UserStatusActive,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}, normalizedSettings, verificationID, codeHash)
	if err != nil {
		if errors.Is(err, repository.ErrEmailVerificationUnavailable) {
			return model.AuthSession{}, safeMessageError{message: "邮箱验证码无效或已过期"}
		}
		return model.AuthSession{}, err
	}
	return newSession(user)
}

func createRegisteredUser(user model.User, settings model.Settings, verificationID string, codeHash string) (model.User, error) {
	reward := 0
	if settings.Public.Auth.NewUserReward {
		reward = settings.Public.Auth.NewUserRewardCredits
	}
	user.Credits = reward
	if reward <= 0 {
		return repository.CreateVerifiedUserWithCreditLog(user, nil, verificationID, codeHash, now(), emailMaxAttempts)
	}
	log := &model.CreditLog{
		ID:        newID("credit"),
		UserID:    user.ID,
		Type:      model.CreditLogTypeNewUser,
		Amount:    reward,
		Remark:    "新用户注册奖励",
		CreatedAt: user.CreatedAt,
	}
	return repository.CreateVerifiedUserWithCreditLog(user, log, verificationID, codeHash, now(), emailMaxAttempts)
}

func Login(identifier string, password string) (model.AuthSession, error) {
	identifier = strings.TrimSpace(identifier)
	user, ok, err := repository.GetUserByUsername(identifier)
	if err == nil && !ok {
		if email, normalizeErr := normalizeEmailAddress(identifier); normalizeErr == nil {
			user, ok, err = repository.GetUserByEmail(email)
		}
	}
	if err != nil {
		return model.AuthSession{}, err
	}
	if !ok || bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return model.AuthSession{}, safeMessageError{message: "用户名、邮箱或密码错误"}
	}
	if user.Status == model.UserStatusBan {
		return model.AuthSession{}, safeMessageError{message: "账号已被禁用"}
	}
	normalizeUserDefaults(&user)
	user.LastLoginAt = now()
	user.Group = normalizeUserGroup(user.Group)
	user.UpdatedAt = now()
	user, err = repository.SaveUser(user)
	if err != nil {
		return model.AuthSession{}, err
	}
	return newSession(user)
}

func ParseToken(tokenText string) (TokenClaims, error) {
	claims := TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenText, &claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("登录状态无效")
		}
		return []byte(config.Cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return TokenClaims{}, errors.New("登录状态无效")
	}
	return claims, nil
}

func CurrentAuthUser(tokenText string) (model.AuthUser, bool) {
	claims, err := ParseToken(tokenText)
	if err != nil {
		return model.AuthUser{}, false
	}
	user, ok, err := repository.GetUserByID(claims.UserID)
	if err != nil || !ok {
		return model.AuthUser{}, false
	}
	if user.Status == model.UserStatusBan {
		return model.AuthUser{}, false
	}
	normalizeUserDefaults(&user)
	return model.PublicUser(user), true
}

func ListUsers(q model.Query) (model.UserList, error) {
	users, total, err := repository.ListUsers(q)
	if err != nil {
		return model.UserList{}, err
	}
	for i := range users {
		users[i].Password = ""
		normalizeUserDefaults(&users[i])
	}
	return model.UserList{Items: users, Total: int(total)}, nil
}

func SaveUser(user model.User, password string) (model.User, error) {
	requestGroup := strings.TrimSpace(user.Group)
	user.Username = strings.TrimSpace(user.Username)
	if strings.ContainsAny(user.Username, " \t\r\n") {
		return user, safeMessageError{message: "用户名不能包含空格"}
	}
	if user.Username == "" {
		return user, safeMessageError{message: "用户名不能为空"}
	}
	if strings.TrimSpace(user.Email) != "" {
		email, err := normalizeEmailAddress(user.Email)
		if err != nil {
			return user, safeMessageError{message: "请输入有效的电子邮箱"}
		}
		user.Email = email
		user.EmailKey = &user.Email
		if saved, ok, err := repository.GetUserByEmail(email); err != nil {
			return user, err
		} else if ok && saved.ID != user.ID {
			return user, safeMessageError{message: "该邮箱已被使用"}
		}
	} else {
		user.Email = ""
		user.EmailKey = nil
	}
	if user.Role == "" || user.Role == model.UserRoleGuest {
		user.Role = model.UserRoleUser
	}
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}
	if saved, ok, err := repository.GetUserByUsername(user.Username); err != nil {
		return user, err
	} else if ok && saved.ID != user.ID {
		return user, safeMessageError{message: "用户名已存在"}
	}
	isCreate := user.ID == ""
	if isCreate {
		user.ID = newID("user")
		user.AffCode = newAffCode()
		user.CreatedAt = now()
	} else if saved, ok, err := repository.GetUserByID(user.ID); err != nil {
		return user, err
	} else if ok {
		user.CreatedAt = saved.CreatedAt
		user.Password = saved.Password
		if strings.TrimSpace(user.Email) == "" {
			user.Email = saved.Email
			user.EmailKey = saved.EmailKey
		}
		user.AvatarURL = saved.AvatarURL
		user.Credits = saved.Credits
		user.Extra = saved.Extra
		if requestGroup == "" {
			user.Group = saved.Group
		}
		if user.AffCode == "" {
			user.AffCode = saved.AffCode
		}
		if user.AffCode == "" {
			user.AffCode = newAffCode()
		}
		if user.LinuxDoID == "" {
			user.LinuxDoID = saved.LinuxDoID
		}
		user.LastLoginAt = saved.LastLoginAt
	}
	if password != "" {
		hash, err := hashPassword(password)
		if err != nil {
			return user, err
		}
		user.Password = hash
	}
	if isCreate && user.Password == "" {
		return user, safeMessageError{message: "密码不能为空"}
	}
	user.UpdatedAt = now()
	user.Group = normalizeUserGroup(user.Group)
	user, err := repository.SaveUser(user)
	user.Password = ""
	return user, err
}

func AdjustUserCredits(id string, credits int) (model.User, error) {
	user, ok, err := repository.GetUserByID(id)
	if err != nil || !ok {
		if err != nil {
			return user, err
		}
		return user, safeMessageError{message: "用户不存在"}
	}
	oldCredits := user.Credits
	user.Credits = credits
	user.UpdatedAt = now()
	user, err = repository.SaveUser(user)
	if err == nil && oldCredits != credits {
		_, err = repository.SaveCreditLog(model.CreditLog{
			ID:        newID("credit"),
			UserID:    user.ID,
			Type:      model.CreditLogTypeAdminAdjust,
			Amount:    credits - oldCredits,
			Balance:   credits,
			Remark:    "后台手动调整",
			CreatedAt: now(),
		})
	}
	user.Password = ""
	return user, err
}

func ConsumeUserCredits(userID string, modelName string, credits int, path string) error {
	if credits <= 0 {
		return nil
	}
	user, ok, err := repository.ConsumeUserCredits(userID, credits, now())
	if err != nil {
		return err
	}
	if !ok {
		return safeMessageError{message: "算力点不足"}
	}
	extra, _ := json.Marshal(map[string]string{"model": modelName, "path": path})
	_, err = repository.SaveCreditLog(model.CreditLog{
		ID:        newID("credit"),
		UserID:    userID,
		Type:      model.CreditLogTypeAIConsume,
		Amount:    -credits,
		Balance:   user.Credits,
		Remark:    "调用模型 " + modelName,
		Extra:     string(extra),
		CreatedAt: now(),
	})
	return err
}

func RefundUserCredits(userID string, modelName string, credits int, path string) error {
	if credits <= 0 {
		return nil
	}
	user, ok, err := repository.RefundUserCredits(userID, credits, now())
	if err != nil {
		return err
	}
	if !ok {
		return safeMessageError{message: "用户不存在"}
	}
	extra, _ := json.Marshal(map[string]string{"model": modelName, "path": path})
	_, err = repository.SaveCreditLog(model.CreditLog{
		ID:        newID("credit"),
		UserID:    userID,
		Type:      model.CreditLogTypeAIRefund,
		Amount:    credits,
		Balance:   user.Credits,
		Remark:    "模型调用失败返还 " + modelName,
		Extra:     string(extra),
		CreatedAt: now(),
	})
	return err
}

func ListCreditLogs(q model.Query) (model.CreditLogList, error) {
	logs, total, err := repository.ListCreditLogs(q)
	if err != nil {
		return model.CreditLogList{}, err
	}
	return model.CreditLogList{Items: logs, Total: int(total)}, nil
}

func SaveCreditLog(log model.CreditLog) (model.CreditLog, error) {
	if log.ID == "" {
		log.ID = newID("credit")
		log.CreatedAt = now()
	}
	return repository.SaveCreditLog(log)
}

func DeleteCreditLog(id string) error {
	return repository.DeleteCreditLog(id)
}

func DeleteUser(id string) error {
	return repository.DeleteUser(id)
}

func GuestUser() model.AuthUser {
	return model.AuthUser{ID: "", Username: "guest", Role: model.UserRoleGuest, Group: "default"}
}

func newSession(user model.User) (model.AuthSession, error) {
	token, err := newToken(user)
	if err != nil {
		return model.AuthSession{}, err
	}
	return model.AuthSession{Token: token, User: model.PublicUser(user)}, nil
}

func newToken(user model.User) (string, error) {
	expireHours := config.Cfg.JWTExpireHours
	if expireHours <= 0 {
		expireHours = 168
	}
	claims := TokenClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   user.ID,
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Cfg.JWTSecret))
}

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

func now() string {
	return time.Now().Format(time.RFC3339)
}

func newID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

func newAffCode() string {
	return strings.ToUpper(strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
}

func normalizeUserDefaults(user *model.User) {
	if user.Status == "" {
		user.Status = model.UserStatusActive
	}
	user.Group = normalizeUserGroup(user.Group)
	if user.AffCode == "" {
		user.AffCode = newAffCode()
	}
}

func normalizeUserGroup(group string) string {
	group = strings.ToLower(strings.TrimSpace(group))
	if group == "" {
		return "default"
	}
	return group
}

func WarnDefaultSecurityConfig() {
	if config.Cfg.AdminUsername == "admin" && config.Cfg.AdminPassword == "infinite-canvas" {
		log.Println("WARNING: using default admin credentials, please set ADMIN_USERNAME and ADMIN_PASSWORD to safer values before deployment")
	}
}
