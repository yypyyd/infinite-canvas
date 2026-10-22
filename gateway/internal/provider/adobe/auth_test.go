package adobe

import (
	"encoding/base64"
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
