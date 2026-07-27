package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/basketikun/infinite-canvas/config"
)

func TestSetUserSessionCookieUsesSecureDefaults(t *testing.T) {
	previous := config.Cfg.JWTExpireHours
	config.Cfg.JWTExpireHours = 2
	t.Cleanup(func() { config.Cfg.JWTExpireHours = previous })
	request := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/login", nil)
	request.Header.Set("X-Forwarded-Proto", "https")
	recorder := httptest.NewRecorder()

	setUserSessionCookie(recorder, request, "session-token")
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "infinite_canvas_session" || cookie.Value != "session-token" || cookie.Path != "/" {
		t.Fatalf("unexpected session cookie: %#v", cookie)
	}
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 7200 {
		t.Fatalf("expected secure HttpOnly SameSite=Lax cookie, got %#v", cookie)
	}
}

func TestLogoutClearsSessionCookie(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "http://example.com/api/auth/logout", nil)
	recorder := httptest.NewRecorder()

	Logout(recorder, request)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one cleared cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "infinite_canvas_session" || cookie.Value != "" || cookie.MaxAge != -1 || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected cleared session cookie: %#v", cookie)
	}
}
