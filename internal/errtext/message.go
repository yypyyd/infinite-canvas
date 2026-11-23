// Package errtext extracts client-facing messages from generation API responses.
package errtext

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Don't expose reflected credentials, signed URLs, or a proxy's HTML error page.
var privateText = regexp.MustCompile(`(?i)(?:-----BEGIN |\b(?:panic:|Traceback \(most recent call last\))|https?://|://[^ ]+@|\bBearer\s+\S+|\b(?:authorization|cookie|set-cookie|password|api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret)["']?\s*[:=]|\b(?:sk-|ic_live_|eyJ)[A-Za-z0-9_.-]+|[A-Za-z0-9_/-]{48,}|<[!/?a-z][^>]*>)`)

func Text(s string, secrets ...string) string {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) || strings.ContainsRune(s, 0) || privateText.MatchString(s) {
		return ""
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(s, secret) {
			return ""
		}
	}
	runes := []rune(s)
	if len(runes) > 300 {
		return string(runes[:300]) + "…"
	}
	return s
}

// Detail selects just the human-readable reason. A machine code is useful when
// it is the only explanation. Other fields can include prompts or private data.
func Detail(value any, secrets ...string) string {
	switch v := value.(type) {
	case string:
		return Text(v, secrets...)
	case map[string]any:
		for _, key := range []string{"error", "message", "msg", "detail", "reason"} {
			if msg := Detail(v[key], secrets...); msg != "" {
				return msg
			}
		}
		switch data := v["data"].(type) {
		case map[string]any, []any:
			if msg := Detail(data, secrets...); msg != "" {
				return msg
			}
		}
		if code, ok := v["code"].(string); ok {
			return Text(code, secrets...)
		}
	case []any:
		for _, item := range v {
			if msg := Detail(item, secrets...); msg != "" {
				return msg
			}
		}
	}
	return ""
}

func Body(body []byte, secrets ...string) string {
	var value any
	if json.Unmarshal(body, &value) == nil {
		return Detail(value, secrets...)
	}
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") || strings.HasPrefix(text, "<") {
		return ""
	}
	return Text(text, secrets...)
}

func Status(code int) string {
	return strings.TrimSpace(fmt.Sprintf("HTTP %d %s", code, http.StatusText(code)))
}

// Failure persists only the public HTTP result. Internal Go errors and raw
// response dumps are never decoded as public messages when reading old events.
type Failure struct {
	Status int    `json:"generation_http_status"`
	Detail string `json:"message,omitempty"`
}

func (f Failure) Error() string {
	if detail := Text(f.Detail); detail != "" {
		return detail
	}
	return Status(f.Status)
}

func (f Failure) Stored() string {
	f.Detail = Text(f.Detail)
	data, _ := json.Marshal(f)
	return string(data)
}

func FromStored(raw string) *Failure {
	var f Failure
	if !strings.HasPrefix(raw, "{") || json.Unmarshal([]byte(raw), &f) != nil || f.Status < 400 || f.Status > 599 {
		return nil
	}
	return &f
}

func HTTP(code int, body []byte, secrets ...string) string {
	if detail := Body(body, secrets...); detail != "" {
		return detail
	}
	return Status(code)
}
