package adobe

import (
	"log"
	"regexp"
	"strings"
)

// Upstream diagnostics: Adobe failures were previously collapsed into stable
// categories ("system under load", "submit rejected") without the raw reply,
// which made outages impossible to tell apart from request/schema bugs. These
// helpers log the HTTP status and a short, credential-free body excerpt.

const upstreamLogBodyLimit = 2500

var upstreamSecretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._~+/=-]+`), "Bearer [REDACTED]"},
	{regexp.MustCompile(`eyJ[A-Za-z0-9_-]{6,}\.[A-Za-z0-9_-]{6,}(\.[A-Za-z0-9_-]+)?`), "[JWT]"},
	{regexp.MustCompile(`(?i)"([a-z_]*(token|cookie|secret|password|authorization|api[_-]?key|session)[a-z_]*)"\s*:\s*"[^"]*"`), `"$1":"[REDACTED]"`},
	{regexp.MustCompile(`(?i)((token|cookie|secret|password|authorization|api[_-]?key|signature|sig|x-amz-[a-z-]+)=)[^&\s"']+`), "${1}[REDACTED]"},
	{regexp.MustCompile(`(https?://[^\s"'?]+)\?[^\s"']*`), "$1?[QUERY]"},
}

func sanitizeUpstreamBody(body []byte, limit int) string {
	s := strings.Join(strings.Fields(string(body)), " ")
	for _, p := range upstreamSecretPatterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	if limit > 0 && len(s) > limit {
		cut := limit
		for cut > 0 && !utf8RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }

func upstreamBodyNotable(status int, body []byte) bool {
	if status != 200 {
		return true
	}
	b := strings.ToLower(string(body))
	return strings.Contains(b, "system under load") || strings.Contains(b, "timeout_error") ||
		strings.Contains(b, `"failed"`) || strings.Contains(b, `"error"`)
}

var logUpstream = func(format string, args ...any) { log.Printf(format, args...) }

// logUpstreamResponse records one Adobe reply when it is not a plain success.
// account is the sticky egress session key (pool account id). arpLen is the
// length of x-arp-session-id on the request (0 = cookie-only / missing ARP).
func logUpstreamResponse(op, account string, arpLen, status int, body []byte) {
	if !upstreamBodyNotable(status, body) {
		return
	}
	logUpstream("adobe upstream %s: account=%s sticky_key=%s arp_len=%d status=%d body=%q",
		sanitizeUpstreamBody([]byte(op), 200),
		sanitizeUpstreamBody([]byte(account), 64),
		sanitizeUpstreamBody([]byte(account), 64),
		arpLen,
		status,
		sanitizeUpstreamBody(body, upstreamLogBodyLimit),
	)
}
