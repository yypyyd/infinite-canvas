package adobe

import (
	"fmt"
	"strings"
	"testing"
)

func TestSanitizeUpstreamBodyRedactsCredentials(t *testing.T) {
	body := []byte(`{"error":"timeout_error","access_token":"abc123","detail":"Authorization: Bearer eyJhbGciOi.xyz",` +
		`"jwt":"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2ln","url":"https://cdn.adobe.io/x.png?X-Amz-Signature=deadbeef&a=1","cookie":"ims_sid=zzz"}`)
	got := sanitizeUpstreamBody(body, 0)
	for _, secret := range []string{"abc123", "eyJhbGciOi", "deadbeef", "ims_sid=zzz", "c2ln"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked: %s", secret, got)
		}
	}
	if !strings.Contains(got, "timeout_error") || !strings.Contains(got, "https://cdn.adobe.io/x.png") {
		t.Fatalf("diagnostic content lost: %s", got)
	}
}

func TestSanitizeUpstreamBodyClipsOnRuneBoundary(t *testing.T) {
	got := sanitizeUpstreamBody([]byte(strings.Repeat("繁", 200)), 300)
	if !strings.HasSuffix(got, "…") || strings.ContainsRune(got, '\uFFFD') || len(got) > 300+len("…") {
		t.Fatalf("bad clip: len=%d", len(got))
	}
}

func TestLogUpstreamResponseOnlyLogsNotableReplies(t *testing.T) {
	var lines []string
	old := logUpstream
	logUpstream = func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	defer func() { logUpstream = old }()

	logUpstreamResponse("poll", 200, []byte(`{"status":"IN_PROGRESS"}`))
	if len(lines) != 0 {
		t.Fatalf("plain progress must not be logged: %v", lines)
	}
	logUpstreamResponse("submit https://firefly-3p.ff.adobe.io/v2/3p-images/generate-async?api_key=k1", 400, []byte(`{"message":"bad size","token":"t0p"}`))
	logUpstreamResponse("submit x", 200, []byte(`{"error_code":"timeout_error","message":"system under load"}`))
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %v", lines)
	}
	if !strings.Contains(lines[0], "status=400") || !strings.Contains(lines[0], "bad size") || strings.Contains(lines[0], "t0p") || strings.Contains(lines[0], "k1") {
		t.Fatalf("unexpected first line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "system under load") {
		t.Fatalf("busy reply not logged: %s", lines[1])
	}
}
