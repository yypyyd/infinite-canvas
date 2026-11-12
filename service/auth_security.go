package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yypyyd/infinite-canvas/config"
	"github.com/yypyyd/infinite-canvas/repository"
)

type authError struct {
	status     int
	message    string
	retryAfter int
}

func (e authError) Error() string       { return e.message }
func (e authError) SafeMessage() string { return e.message }
func (e authError) HTTPStatus() int     { return e.status }
func (e authError) RetryAfter() int     { return e.retryAfter }

func AdminSessionMaxAge() int {
	hours := config.Cfg.JWTExpireHours
	if hours <= 0 || hours > 12 {
		hours = 12
	}
	return hours * 3600
}

func Logout(r *http.Request) error {
	tokens := []string{strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")}
	for _, name := range []string{"infinite_canvas_session", "infinite_canvas_admin_session"} {
		if cookie, err := r.Cookie(name); err == nil {
			tokens = append(tokens, cookie.Value)
		}
	}
	seen := map[string]bool{}
	for _, token := range tokens {
		user, ok := CurrentAuthUser(token)
		if ok && !seen[user.ID] {
			if err := repository.RevokeUserSessions(user.ID); err != nil {
				return err
			}
			seen[user.ID] = true
		}
	}
	return nil
}

func RequestClientIP(r *http.Request) string {
	secret := config.Cfg.ReverseProxyToken
	if len(secret) >= 32 && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Proxy-Token")), []byte(secret)) == 1 {
		if ip := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); ip != nil {
			return ip.String()
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}

// Failures are shared by both login endpoints. In-flight reservations prevent parallel bypass.
type loginLimit struct {
	failures, inFlight, level int
	blockedUntil, expiresAt   time.Time
}

var loginLimits = struct {
	sync.Mutex
	items map[string]*loginLimit
}{items: make(map[string]*loginLimit)}

func loginLimitKey(kind, value string) string {
	sum := sha256.Sum256([]byte(value))
	return kind + ":" + hex.EncodeToString(sum[:])
}

func beginLoginAttempt(key string) (func(bool), error) {
	loginLimits.Lock()
	defer loginLimits.Unlock()
	now := time.Now()
	for k, record := range loginLimits.items {
		if record.inFlight == 0 && !record.expiresAt.After(now) {
			delete(loginLimits.items, k)
		}
	}
	record := loginLimits.items[key]
	if record == nil {
		if len(loginLimits.items) >= 10000 {
			return nil, authError{429, "登录请求过于频繁，请稍后再试", 60}
		}
		record = &loginLimit{}
		loginLimits.items[key] = record
	}
	if record.blockedUntil.After(now) {
		return nil, authError{429, "登录尝试过于频繁，请稍后再试", int(time.Until(record.blockedUntil).Seconds()) + 1}
	}
	if !record.blockedUntil.IsZero() {
		record.failures = 0
		record.blockedUntil = time.Time{}
	}
	if record.failures+record.inFlight >= 5 {
		return nil, authError{429, "登录请求过于频繁，请稍后再试", 1}
	}
	record.inFlight++
	record.expiresAt = now.Add(24 * time.Hour)
	return func(success bool) {
		loginLimits.Lock()
		defer loginLimits.Unlock()
		record.inFlight--
		if success {
			if strings.HasPrefix(key, "account:") {
				record.failures = 0
				record.level = 0
				record.blockedUntil = time.Time{}
			}
			return
		}
		record.failures++
		if record.failures >= 5 && !record.blockedUntil.After(time.Now()) {
			record.blockedUntil = time.Now().Add(15 * time.Minute * time.Duration(1<<record.level))
			if record.level < 2 {
				record.level++
			}
		}
	}, nil
}

func recordLogin(identifier, ip, requestID string, admin bool, err error) {
	result := "success"
	if err != nil {
		result = "failed"
	}
	status := 200
	if failure, ok := err.(authError); ok {
		status = failure.status
	} else if err != nil {
		status = 500
	}
	slog.Info("auth_login", "account_hash", loginLimitKey("account", strings.ToLower(identifier)), "client_ip", ip, "request_id", requestID, "admin", admin, "result", result, "status", status)
}

func LoginChallengeSiteKey() string {
	if config.Cfg.TurnstileSiteKey == "" || config.Cfg.TurnstileSecretKey == "" || config.Cfg.TurnstileHostnames == "" {
		return ""
	}
	return config.Cfg.TurnstileSiteKey
}

func verifyLoginChallenge(ctx context.Context, token, ip string) error {
	if token == "" || len(token) > 2048 {
		return authError{401, "请先完成人机验证", 0}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"secret": {config.Cfg.TurnstileSecretKey}, "response": {token}}
	if parsed := net.ParseIP(ip); parsed != nil && !parsed.IsLoopback() && !parsed.IsPrivate() {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return authError{503, "登录验证暂不可用，请稍后重试", 0}
	}
	defer response.Body.Close()
	var result struct {
		Success  bool   `json:"success"`
		Hostname string `json:"hostname"`
		Action   string `json:"action"`
	}
	if response.StatusCode != 200 || json.NewDecoder(io.LimitReader(response.Body, 16384)).Decode(&result) != nil {
		return authError{503, "登录验证暂不可用，请稍后重试", 0}
	}
	for _, hostname := range strings.Split(config.Cfg.TurnstileHostnames, ",") {
		if result.Success && result.Action == "login" && strings.EqualFold(result.Hostname, strings.TrimSpace(hostname)) {
			return nil
		}
	}
	return authError{401, "人机验证失败，请重新验证", 0}
}
