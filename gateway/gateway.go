// Package gateway is 幻图's built-in account pool (the former Nexa gateway).
//
// It exposes only what 幻图 itself needs:
//
//   - Mount: the super-admin control plane on /api/admin/account-pool/*.
//   - WrapDialContext / InstallDefaultTransport: the in-process route that
//     幻图 uses to call the pool at http://nexa.internal/v1. There is no
//     network listener and no external API key: the pool is not reachable
//     from outside the 幻图 process.
//   - Models: the pool's enabled models, which 幻图 offers as a built-in
//     channel without any channel configuration.
//
// The pool stores its tables in its own PostgreSQL schema (POOL_DB_SCHEMA,
// default "gateway"). By default it shares 幻图's database
// (STORAGE_DRIVER=postgres + DATABASE_DSN); POOL_POSTGRES_DSN overrides that.
// Redis is optional (POOL_REDIS_ADDR); without it, coordination is in-process.
package gateway

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/yypyyd/infinite-canvas/gateway/internal/bootstrap"
	"github.com/yypyyd/infinite-canvas/gateway/internal/config"
	"github.com/yypyyd/infinite-canvas/gateway/internal/http/middleware"
	"github.com/yypyyd/infinite-canvas/gateway/internal/http/router"
	"github.com/yypyyd/infinite-canvas/gateway/internal/model"
)

const (
	// InternalHost is the sentinel host for in-process calls.
	InternalHost = config.InternalHost
	// InternalBaseURL is the base URL of the built-in channel.
	InternalBaseURL = "http://" + config.InternalHost + "/v1"
	// AdminPrefix is the admin control plane base path.
	AdminPrefix = config.AdminPathPrefix
)

// ErrNotConfigured is returned by Start when no PostgreSQL is available.
var ErrNotConfigured = config.ErrNotConfigured

// Module is a running account pool.
type Module struct {
	app      *bootstrap.App
	listener *memoryListener
	server   *http.Server
	models   modelCache
}

var current atomic.Pointer[Module]

// Disabled reports whether the pool was explicitly turned off with
// POOL_ENABLED=false. The pool is otherwise on whenever PostgreSQL is
// available.
func Disabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("POOL_ENABLED"))) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

// Current returns the running module, or nil when the pool is not running.
func Current() *Module { return current.Load() }

// Start boots the pool (migrations, optional Redis, maintenance workers) and
// the in-process listener, and registers it as the current module. It returns
// ErrNotConfigured when no PostgreSQL is available.
func Start(ctx context.Context) (*Module, error) {
	app, err := bootstrap.NewApp(ctx)
	if err != nil {
		return nil, err
	}
	m := &Module{app: app, listener: newMemoryListener()}

	internal := gin.New()
	internal.Use(middleware.RequestID(), gin.Recovery())
	builtin := app.Builtin
	auth := middleware.UseCredential(func() *model.APICredential { return builtin })
	router.MountV1(internal.Group("/v1"), auth, app.Handlers)
	// Older 幻图 tasks may have stored URLs under the former public prefix.
	router.MountV1(internal.Group("/api/gateway/v1"), auth, app.Handlers)
	m.server = &http.Server{Handler: internal, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := m.server.Serve(m.listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("account pool in-process server: %v", err)
		}
	}()
	current.Store(m)
	return m, nil
}

// Mount registers the admin control plane on the 幻图 engine. adminAuth must
// enforce 幻图's super-admin role.
func (m *Module) Mount(engine *gin.Engine, adminAuth gin.HandlerFunc) {
	router.MountAdmin(engine.Group(AdminPrefix, adminAuth), m.app.Handlers)
}

// Close stops the in-process server and releases DB/Redis.
func (m *Module) Close() error {
	current.CompareAndSwap(m, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = m.server.Shutdown(ctx)
	_ = m.listener.Close()
	return m.app.Close()
}

// DialFunc matches net.Dialer.DialContext / http.Transport.DialContext.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// WrapDialContext routes connections for InternalHost to the in-process
// gateway and leaves every other address to next. When the gateway is not
// running the sentinel host fails fast instead of reaching DNS.
func WrapDialContext(next DialFunc) DialFunc {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			host = address
		}
		if strings.EqualFold(host, InternalHost) {
			m := current.Load()
			if m == nil {
				return nil, errors.New("内置账号池未运行（需要 PostgreSQL）")
			}
			return m.listener.dial(ctx)
		}
		if next == nil {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, address)
		}
		return next(ctx, network, address)
	}
}

// IsInternalURL reports whether raw points at the built-in gateway.
func IsInternalURL(host string) bool {
	return strings.EqualFold(strings.TrimSpace(host), InternalHost)
}

var installOnce sync.Once

// InstallDefaultTransport makes http.DefaultTransport (and therefore every
// zero-value http.Client) able to reach InternalHost.
func InstallDefaultTransport() {
	installOnce.Do(func() {
		if transport, ok := http.DefaultTransport.(*http.Transport); ok {
			transport.DialContext = WrapDialContext(transport.DialContext)
		}
	})
}

// memoryListener is an in-memory net.Listener backed by net.Pipe.
type memoryListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newMemoryListener() *memoryListener {
	return &memoryListener{conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *memoryListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.conns:
		return conn, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *memoryListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *memoryListener) Addr() net.Addr { return memoryAddr{} }

func (l *memoryListener) dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.closed:
		_ = client.Close()
		_ = server.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = client.Close()
		_ = server.Close()
		return nil, ctx.Err()
	}
}

type memoryAddr struct{}

func (memoryAddr) Network() string { return "memory" }
func (memoryAddr) String() string  { return InternalHost }

// Health is the connectivity state of the pool's dependencies. Values are
// "ok", "unavailable" or, for Redis when not configured, "disabled". Error
// details are never included.
type Health struct {
	Status   string `json:"status"`
	Database string `json:"database"`
	Redis    string `json:"redis"`
}

// withDeadline returns when fn does or ctx ends, whichever is first, because
// drivers may enforce their own (longer) socket timeouts instead of ctx.
func withDeadline(ctx context.Context, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CheckHealth pings the gateway Postgres and Redis concurrently. Each ping is
// bounded by timeout (and ctx). It returns the first error for logging.
func (m *Module) CheckHealth(ctx context.Context, timeout time.Duration) (Health, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var dbErr, redisErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		dbErr = errors.New("gateway database not configured")
		if m.app.DB != nil {
			if sqlDB, err := m.app.DB.DB(); err != nil {
				dbErr = err
			} else {
				dbErr = withDeadline(ctx, func() error { return sqlDB.PingContext(ctx) })
			}
		}
	}()
	go func() {
		defer wg.Done()
		if m.app.Redis != nil {
			redisErr = withDeadline(ctx, func() error { return m.app.Redis.Ping(ctx).Err() })
		}
	}()
	wg.Wait()
	h := Health{Status: "ok", Database: "ok", Redis: "ok"}
	if m.app.Redis == nil {
		h.Redis = "disabled"
	}
	if dbErr != nil {
		h.Status, h.Database = "unavailable", "unavailable"
	}
	if redisErr != nil {
		h.Status, h.Redis = "unavailable", "unavailable"
	}
	if dbErr != nil {
		return h, fmt.Errorf("gateway database: %w", dbErr)
	}
	if redisErr != nil {
		return h, fmt.Errorf("gateway redis: %w", redisErr)
	}
	return h, nil
}

// MountUnavailable answers the admin control plane with 503 and reason when
// the pool is not running, so the admin pages can say what is missing.
func MountUnavailable(engine *gin.Engine, adminAuth gin.HandlerFunc, reason string) {
	engine.Group(AdminPrefix, adminAuth).Any("/*path", func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"code": "pool_unavailable", "message": reason}})
	})
}
