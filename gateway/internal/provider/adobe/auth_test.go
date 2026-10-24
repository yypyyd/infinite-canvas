package adobe

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestResolveARPSessionTokenUsesExportedCookie(t *testing.T) {
	// The literal '+' is significant in standard base64 cookie values.
	browser := base64.StdEncoding.EncodeToString([]byte(`{"sid":"browser-session","ftr":"~~~","ark":"browser-challenge-token","bfp":"browser-fingerprint"}`))
	if !strings.Contains(browser, "+") {
		t.Fatal("fixture must contain a base64 plus")
	}
	base := NewARPSessionToken()
	for _, tc := range []struct{ name, stored, cookie, want string }{
		{"raw_cookie", "", "other=secret; sherlockToken=" + browser + "; end=secret", browser},
		{"encoded_cookie", base, "sherlockToken=" + url.PathEscape(browser), browser},
		{"quoted_cookie", base, `sherlockToken="` + browser + `"`, browser},
		{"explicit_session", "opaque-browser-export", "sherlockToken=" + browser, "opaque-browser-export"},
		{"no_cookie_session", base, "other=secret", base},
		{"invalid_cookie", base, "sherlockToken=not-base64", base},
		{"missing_sid", base, "sherlockToken=" + base64.StdEncoding.EncodeToString([]byte(`{"ftr":"test"}`)), base},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveARPSessionToken(tc.stored, tc.cookie); got != tc.want {
				t.Fatal("unexpected session selection")
			}
		})
	}
	if !isBaseARPSessionToken(ResolveARPSessionToken("", "other=secret")) {
		t.Fatal("missing cookie must retain the sid fallback")
	}
}

func TestSubmissionARPSessionTokenKeepsBaseSid(t *testing.T) {
	base := NewARPSessionToken()
	first := SubmissionARPSessionToken(base)
	second := SubmissionARPSessionToken(base)
	if isBaseARPSessionToken(first) {
		t.Fatal("submit token must carry Forter feature fields")
	}
	raw1, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatal(err)
	}
	raw2, err := base64.StdEncoding.DecodeString(second)
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]any
	if json.Unmarshal(raw1, &a) != nil || json.Unmarshal(raw2, &b) != nil {
		t.Fatal("submit tokens must decode")
	}
	if a["sid"] != b["sid"] {
		t.Fatalf("sid rotated across submits: %v vs %v", a["sid"], b["sid"])
	}
	baseRaw, _ := base64.StdEncoding.DecodeString(base)
	var basePayload map[string]any
	_ = json.Unmarshal(baseRaw, &basePayload)
	if a["sid"] != basePayload["sid"] {
		t.Fatalf("submit sid lost the stored base sid: %v vs %v", a["sid"], basePayload["sid"])
	}
	if strings.TrimSpace(stringValue(a["ftr"])) == "" || a["ftr"] == b["ftr"] {
		t.Fatal("Forter feature token must refresh per submit")
	}
	rich := base64.StdEncoding.EncodeToString([]byte(`{"sid":"keep-me","ftr":"old","ark":"x","bfp":"y"}`))
	if got := SubmissionARPSessionToken(rich); got != rich {
		t.Fatal("rich browser ARP must pass through unchanged")
	}
}
