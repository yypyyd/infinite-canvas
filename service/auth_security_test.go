package service

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/model"
)

func TestClientIPRequiresProxyAuthentication(t *testing.T) {
	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })
	config.Cfg.ReverseProxyToken = strings.Repeat("k", 32)
	r := httptest.NewRequest("POST", "/api/auth/login", nil)
	r.RemoteAddr = "127.0.0.1:8080"
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	r.Header.Set("CF-Connecting-IP", "198.51.100.2")
	r.Header.Set("X-Real-IP", "198.51.100.3")
	r.Header.Set("X-Proxy-Token", "forged")
	if ip := RequestClientIP(r); ip != "127.0.0.1" {
		t.Fatalf("untrusted headers accepted: %s", ip)
	}
	r.Header.Set("X-Proxy-Token", config.Cfg.ReverseProxyToken)
	if ip := RequestClientIP(r); ip != "198.51.100.3" {
		t.Fatalf("trusted proxy address lost: %s", ip)
	}
	r.Header.Set("X-Real-IP", "invalid")
	if ip := RequestClientIP(r); ip != "127.0.0.1" {
		t.Fatalf("invalid proxy address accepted: %s", ip)
	}
}

func TestLoginLimitReservesInFlightAttemptsAndBacksOff(t *testing.T) {
	key := loginLimitKey("account", t.Name())
	t.Cleanup(func() { loginLimits.Lock(); delete(loginLimits.items, key); loginLimits.Unlock() })
	var finish []func(bool)
	for range 5 {
		done, err := beginLoginAttempt(key)
		if err != nil {
			t.Fatal(err)
		}
		finish = append(finish, done)
	}
	if _, err := beginLoginAttempt(key); err == nil {
		t.Fatal("sixth in-flight attempt was admitted")
	}
	for _, done := range finish {
		done(false)
	}
	if _, err := beginLoginAttempt(key); err == nil {
		t.Fatal("failed account was not blocked")
	}
	loginLimits.Lock()
	loginLimits.items[key].blockedUntil = time.Now().Add(-time.Second)
	loginLimits.Unlock()
	for range 5 {
		done, err := beginLoginAttempt(key)
		if err != nil {
			t.Fatal(err)
		}
		done(false)
	}
	_, err := beginLoginAttempt(key)
	failure, ok := err.(authError)
	if !ok || failure.status != 429 || failure.retryAfter < 1790 {
		t.Fatalf("second lockout did not back off: %v", err)
	}
}

func TestSessionAlgorithmAndAdminLifetime(t *testing.T) {
	previous := config.Cfg
	t.Cleanup(func() { config.Cfg = previous })
	config.Cfg.JWTSecret = strings.Repeat("s", 32)
	config.Cfg.JWTExpireHours = 168
	user := model.User{ID: "admin-test", Role: model.UserRoleAdmin, TokenVersion: 7}
	token, err := newToken(user)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if !claims.Admin || claims.TokenVersion != 7 || claims.ExpiresAt.Sub(claims.IssuedAt.Time) > 12*time.Hour {
		t.Fatal("admin session scope, version or lifetime is incorrect")
	}
	wrongAlgorithm, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(config.Cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(wrongAlgorithm); err == nil {
		t.Fatal("HS384 token was accepted")
	}
	claims.ExpiresAt = nil
	missingExpiry, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(config.Cfg.JWTSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(missingExpiry); err == nil {
		t.Fatal("token without expiration was accepted")
	}
}
