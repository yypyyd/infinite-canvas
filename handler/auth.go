package handler

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

type sendEmailCodeRequest struct {
	Email string `json:"email"`
}

type saveUserRequest struct {
	ID          string           `json:"id"`
	Username    string           `json:"username"`
	Password    string           `json:"password"`
	Email       string           `json:"email"`
	DisplayName string           `json:"displayName"`
	Role        model.UserRole   `json:"role"`
	Group       string           `json:"group"`
	Status      model.UserStatus `json:"status"`
}

type adjustUserCreditsRequest struct {
	Credits int `json:"credits"`
}

type updateProfileRequest struct {
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

func Register(w http.ResponseWriter, r *http.Request) {
	var request registerRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	session, err := service.Register(request.Username, request.Email, request.Code, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	setUserSessionCookie(w, r, session.Token)
	OK(w, session)
}

func SendRegistrationEmailCode(w http.ResponseWriter, r *http.Request) {
	var request sendEmailCodeRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	if err := service.SendRegistrationEmailCode(request.Email, requestRemoteIP(r)); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func requestRemoteIP(r *http.Request) string {
	value := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	remoteIP := net.ParseIP(value)
	if remoteIP == nil {
		return ""
	}
	if remoteIP.IsLoopback() || remoteIP.IsPrivate() {
		parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
		for index := len(parts) - 1; index >= 0; index-- {
			forwardedIP := net.ParseIP(strings.TrimSpace(parts[index]))
			if forwardedIP != nil && !forwardedIP.IsLoopback() && !forwardedIP.IsPrivate() {
				return forwardedIP.String()
			}
		}
	}
	return remoteIP.String()
}

func Login(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	session, err := service.Login(request.Username, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	setUserSessionCookie(w, r, session.Token)
	OK(w, session)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "infinite_canvas_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode})
	OK(w, true)
}

func AdminLogin(w http.ResponseWriter, r *http.Request) {
	var request loginRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	session, err := service.Login(request.Username, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	if session.User.Role != model.UserRoleAdmin {
		Fail(w, "需要管理员权限")
		return
	}
	setUserSessionCookie(w, r, session.Token)
	OK(w, session)
}

func setUserSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	maxAge := config.Cfg.JWTExpireHours * 3600
	if maxAge <= 0 {
		maxAge = 7 * 24 * 3600
	}
	http.SetCookie(w, &http.Cookie{Name: "infinite_canvas_session", Value: token, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode})
}

func requestIsSecure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}

func CurrentUser(w http.ResponseWriter, r *http.Request) {
	if user, ok := service.UserFromContext(r.Context()); ok {
		OK(w, user)
		return
	}
	OK(w, service.GuestUser())
}

func UpdateProfile(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var request updateProfileRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	updated, err := service.UpdateProfile(user.ID, request.DisplayName, request.AvatarURL)
	if err != nil {
		FailError(w, err)
		return
	}
	updated.OrganizationID = user.OrganizationID
	updated, err = service.ApplyEffectiveCredits(updated)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, updated)
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	var request changePasswordRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	if err := service.ChangePassword(user.ID, request.CurrentPassword, request.NewPassword); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func UserCreditLogs(w http.ResponseWriter, r *http.Request) {
	user, ok := service.UserFromContext(r.Context())
	if !ok {
		Fail(w, "请先登录")
		return
	}
	logs, err := service.ListUserCreditLogs(user.ID, parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, logs)
}

func AdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := service.ListUsers(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, users)
}

func AdminSaveUser(w http.ResponseWriter, r *http.Request) {
	var request saveUserRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	user, err := service.SaveUser(model.User{
		ID:          request.ID,
		Username:    request.Username,
		Email:       request.Email,
		DisplayName: request.DisplayName,
		Role:        request.Role,
		Group:       request.Group,
		Status:      request.Status,
	}, request.Password)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}

func AdminAdjustUserCredits(w http.ResponseWriter, r *http.Request, id string) {
	var request adjustUserCreditsRequest
	_ = json.NewDecoder(r.Body).Decode(&request)
	user, err := service.AdjustUserCredits(id, request.Credits)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, user)
}

func AdminCreditLogs(w http.ResponseWriter, r *http.Request) {
	logs, err := service.ListCreditLogs(parseQuery(r))
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, logs)
}

func AdminSaveCreditLog(w http.ResponseWriter, r *http.Request) {
	var log model.CreditLog
	_ = json.NewDecoder(r.Body).Decode(&log)
	result, err := service.SaveCreditLog(log)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}

func AdminDeleteCreditLog(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteCreditLog(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}

func AdminDeleteUser(w http.ResponseWriter, r *http.Request, id string) {
	if err := service.DeleteUser(id); err != nil {
		FailError(w, err)
		return
	}
	OK(w, true)
}
