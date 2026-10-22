package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Paths and host used by the built-in account pool.
const (
	// AdminPathPrefix is where the account pool admin API is mounted, behind
	// 幻图's super-admin middleware.
	AdminPathPrefix = "/api/admin/account-pool"
	// InternalHost is the sentinel host 幻图 dials to reach the pool
	// in-process (see gateway.WrapDialContext). It never resolves over DNS.
	InternalHost = "nexa.internal"
)

type Config struct {
	AppEnv          string
	PostgresDSN     string
	DBSchema        string
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	GeneratedRoot   string
	ObjectRoot      string
	RustFSEndpoint  string
	RustFSBucket    string
	RustFSAccessKey string
	RustFSSecretKey string
}

// ErrNotConfigured means no PostgreSQL is available for the pool.
var ErrNotConfigured = fmt.Errorf("account pool needs PostgreSQL: set STORAGE_DRIVER=postgres with DATABASE_DSN, or POOL_POSTGRES_DSN")

// EmbeddedDSNFile is written by the Docker entrypoint when it starts the
// embedded PostgreSQL, so commands run with `docker exec` find it too.
const EmbeddedDSNFile = "/run/postgresql/huantu-pool.dsn"

// ResolvePostgresDSN returns the pool database: POOL_POSTGRES_DSN when set,
// otherwise 幻图's own DATABASE_DSN when 幻图 runs on PostgreSQL, otherwise
// the container's embedded PostgreSQL if it is running.
func ResolvePostgresDSN() string {
	if v := strings.TrimSpace(os.Getenv("POOL_POSTGRES_DSN")); v != "" {
		return v
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_DRIVER"))) {
	case "postgres", "postgresql":
		return strings.TrimSpace(os.Getenv("DATABASE_DSN"))
	}
	if data, err := os.ReadFile(embeddedDSNFile); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}

var embeddedDSNFile = EmbeddedDSNFile

func Load() (*Config, error) {
	// 幻图 loads .env itself (godotenv); the pool only reads POOL_* keys plus
	// 幻图's STORAGE_DRIVER / DATABASE_DSN.
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		AppEnv:        envString("APP_ENV", "production"),
		PostgresDSN:   ResolvePostgresDSN(),
		DBSchema:      envString("DB_SCHEMA", "gateway"),
		RedisAddr:     envString("REDIS_ADDR", ""),
		RedisPassword: envString("REDIS_PASSWORD", ""),
		RedisDB:       envInt("REDIS_DB", 0),
		// Generated outputs and uploaded reference images live under 幻图's
		// data dir so the production volume (/app/data) persists them.
		GeneratedRoot: filepath.Clean(envString("GENERATED_ROOT", filepath.Join(wd, "data", "generated"))),
		// Used for generated media when RustFS is not configured.
		ObjectRoot:      filepath.Clean(envString("OBJECT_ROOT", filepath.Join(wd, "data", "pool-objects"))),
		RustFSEndpoint:  envString("RUSTFS_ENDPOINT", ""),
		RustFSBucket:    envString("RUSTFS_BUCKET", ""),
		RustFSAccessKey: envString("RUSTFS_ACCESS_KEY", ""),
		RustFSSecretKey: envString("RUSTFS_SECRET_KEY", ""),
	}
	if cfg.PostgresDSN == "" {
		return nil, ErrNotConfigured
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	appEnv := strings.ToLower(strings.TrimSpace(c.AppEnv))
	if appEnv != "development" && appEnv != "production" {
		return fmt.Errorf("POOL_APP_ENV must be either development or production")
	}
	if _, err := pgx.ParseConfig(c.PostgresDSN); err != nil {
		return fmt.Errorf("parse pool PostgreSQL DSN: %w", err)
	}
	// Object storage is optional: without it image caching is skipped and
	// stored videos cannot be replayed. Only validate it when configured.
	if strings.TrimSpace(c.RustFSEndpoint) == "" {
		return nil
	}
	if err := validateServiceEndpoint("RUSTFS_ENDPOINT", c.RustFSEndpoint); err != nil {
		return err
	}
	if err := validateIdentifier("RUSTFS_BUCKET", c.RustFSBucket); err != nil {
		return err
	}
	for _, secret := range []struct {
		name   string
		value  string
		minLen int
	}{
		{name: "RUSTFS_ACCESS_KEY", value: c.RustFSAccessKey, minLen: 16},
		{name: "RUSTFS_SECRET_KEY", value: c.RustFSSecretKey, minLen: 32},
	} {
		if err := validateProductionSecret(secret.name, secret.value, secret.minLen); err != nil {
			return err
		}
	}
	return nil
}

func validateServiceEndpoint(name, raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s must be an absolute HTTP(S) endpoint in production", name)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must not contain a query, user info, or fragment in production", name)
	}
	if err := validateURLPort(name, parsed); err != nil {
		return err
	}
	return nil
}

func validateURLPort(name string, parsed *url.URL) error {
	if parsed == nil || parsed.Port() == "" {
		return nil
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s contains an invalid port in production", name)
	}
	return nil
}

func validateIdentifier(name, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" || exampleValue(value) {
		return fmt.Errorf("%s is required and cannot use an example value in production", name)
	}
	return nil
}

func validateProductionSecret(name, raw string, minLen int) error {
	value := strings.TrimSpace(raw)
	if len([]byte(value)) < minLen || exampleValue(value) {
		return fmt.Errorf("%s must be at least %d bytes and cannot use a default or example value in production", name, minLen)
	}
	return nil
}

func exampleValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(normalized, "replace-with-") || strings.Contains(normalized, "changeme") || strings.Contains(normalized, "example") {
		return true
	}
	switch normalized {
	case "postgres", "password", "secret", "admin", "rustfs", "minioadmin":
		return true
	default:
		return false
	}
}

func isLocalHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsUnspecified()
	}
	return false
}

// loadDotEnv loads a .env file (KEY=VALUE per line) into the process environment
// before config is read. Real environment variables always win — .env only fills
// keys that aren't already set. Searches ENV_FILE, then walks up from the working
// directory so it works whether the binary runs from backend/ or the repo root.
func loadDotEnv() {
	for _, path := range dotEnvCandidates() {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		applyDotEnv(string(data))
		return
	}
}

func dotEnvCandidates() []string {
	var out []string
	if v := strings.TrimSpace(os.Getenv("ENV_FILE")); v != "" {
		out = append(out, v)
	}
	wd, err := os.Getwd()
	if err != nil {
		return out
	}
	dir := wd
	for i := 0; i < 4; i++ {
		out = append(out, filepath.Join(dir, ".env"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return out
}

func applyDotEnv(content string) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		// Real env wins: only set keys that aren't already present.
		if _, ok := os.LookupEnv(key); !ok {
			_ = os.Setenv(key, val)
		}
	}
}

func envString(key, fallback string) string {
	if v, ok := lookupGatewayEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, ok := lookupGatewayEnv(key); ok && strings.TrimSpace(v) != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return fallback
}

func envList(key string, fallback []string) []string {
	if v, ok := lookupGatewayEnv(key); ok && strings.TrimSpace(v) != "" {
		parts := strings.Split(v, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			s := strings.TrimSpace(part)
			if s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := lookupGatewayEnv(key); ok && strings.TrimSpace(v) != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

// lookupGatewayEnv reads pool settings from POOL_<KEY> so they never collide
// with 幻图's own environment (PORT, PUBLIC_BASE_URL, ...).
func lookupGatewayEnv(key string) (string, bool) {
	return os.LookupEnv("POOL_" + key)
}
