package adobe

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

func fakeJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	return "eyJhbGciOiJSUzI1NiJ9." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}

func TestIsGuestAccessTokenDetectsGuestClaims(t *testing.T) {
	guest := fakeJWT(t, map[string]any{"iss": "https://ims-na1.adobelogin.com", "atp": "guest", "client_id": "clio-playground-web"})
	if !isGuestAccessToken(guest) {
		t.Fatal("atp=guest token must be detected")
	}
	user := fakeJWT(t, map[string]any{"type": "access_token", "as": "ims-na1", "user_id": "ABC@AdobeID"})
	if isGuestAccessToken(user) {
		t.Fatal("signed-in IMS token must not be treated as guest")
	}
	for _, bad := range []string{"", "not-a-jwt", "a.%%%.c"} {
		if isGuestAccessToken(bad) {
			t.Fatalf("malformed token %q must not be classified as guest", bad)
		}
	}
}

func TestErrGuestSessionIsAuthFailure(t *testing.T) {
	if !errors.Is(ErrGuestSession, ErrAuth) {
		t.Fatal("guest session must be handled as an auth failure")
	}
}
