package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
	"github.com/yypyyd/infinite-canvas/service"
)

type loginRequest struct {
	Username       string `json:"username"`
	Password       string `json:"password"`
	TurnstileToken string `json:"turnstileToken"`
}

type registerRequest struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Code         string `json:"code"`
	Password     string `json:"password"`
	ReferralCode string `json:"referralCode"`
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
	session, err := service.Register(request.Username, request.Email, request.Code, request.Password, request.ReferralCode)
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

func requestRemoteIP(r *http.Request) string { return service.RequestClientIP(r) }

func Login(w http.ResponseWriter, r *http.Request) { credentialLogin(w, r, false) }

func AdminLogin(w http.ResponseWriter, r *http.Request) { credentialLogin(w, r, true) }

func AdminLoginChallenge(w http.ResponseWriter, r *http.Request) {
	siteKey, err := service.AdminLoginChallenge()
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, map[string]string{"siteKey": siteKey})
}

func credentialLogin(w http.ResponseWriter, r *http.Request, admin bool) {
	var request loginRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&request) != nil {
		FailStatus(w, http.StatusBadRequest, "登录参数无效")
		return
	}
	session, err := service.Login(r.Context(), service.LoginInput{Username: request.Username, Password: request.Password, Challenge: request.TurnstileToken, IP: service.RequestClientIP(r), RequestID: r.Header.Get("X-Request-ID"), Admin: admin})
	if err != nil {
		FailError(w, err)
		return
	}
	if admin {
		http.SetCookie(w, &http.Cookie{Name: "infinite_canvas_admin_session", Value: session.Token, Path: "/api", MaxAge: service.AdminSessionMaxAge(), HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteStrictMode})
	} else {
		setUserSessionCookie(w, r, session.Token)
	}
	OK(w, session)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	if err := service.Logout(r); err != nil {
		FailError(w, err)
		return
	}
	clearSessionCookies(w, r)
	OK(w, true)
}

func clearSessionCookies(w http.ResponseWriter, r *http.Request) {
	for name, path := range map[string]string{"infinite_canvas_session": "/", "infinite_canvas_admin_session": "/api"} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: path, MaxAge: -1, HttpOnly: true, Secure: requestIsSecure(r), SameSite: http.SameSiteLaxMode})
	}
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
	clearSessionCookies(w, r)
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
