package adobe

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ErrGuestSession means IMS answered the cookie exchange with an anonymous
// guest token (guest_allowed=true). Such tokens look valid but every Firefly
// generation endpoint rejects them with 401 "Invalid or expired token", so an
// import must fail loudly instead of becoming an "active" but unusable account.
var ErrGuestSession = fmt.Errorf("%w: adobe cookie has no signed-in session (IMS issued a guest token); export the full cookies of a logged-in browser, including the adobelogin.com session (ims_sid)", ErrAuth)

// isGuestAccessToken inspects only the unsigned JWT payload claims.
func isGuestAccessToken(token string) bool {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return false
	}
	var claims map[string]any
	if json.Unmarshal(raw, &claims) != nil {
		return false
	}
	atp, _ := claims["atp"].(string)
	typ, _ := claims["type"].(string)
	return strings.EqualFold(atp, "guest") || strings.EqualFold(typ, "guest_token")
}
