package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
)

// Helpers kept from the standalone admin auth service, which was removed when
// the gateway was embedded into 幻图 (admin auth is 幻图's super-admin role).

// ParseBearer implements the OpenAI Authorization header shape. It rejects
// alternate schemes, missing values and extra whitespace-separated fields.
func ParseBearer(header string) string {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return ""
	}
	return fields[1]
}

func HashAPIKey(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// V1 still reads boolean feature flags from the retained settings store.
func parseBoolSetting(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func randomSecret(byteLength int) (string, error) {
	if byteLength < 1 {
		return "", errors.New("secret length must be positive")
	}
	raw := make([]byte, byteLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
