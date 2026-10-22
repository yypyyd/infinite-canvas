// Package gateway embeds the former standalone Nexa API gateway into 幻图.
//
// It exposes three things to the host application:
//
//   - Mount: the OpenAI-compatible data plane (Bearer sk-* keys) on /v1 and
//     /api/gateway/v1, plus the admin control plane on /api/admin/gateway/*
//     behind the middleware supplied by 幻图 (super admin only).
//   - WrapDialContext / InstallDefaultTransport: an in-process route so that
//     幻图 can use the gateway as a built-in model channel at
//     http://nexa.internal/v1 without a network hop or a stored API key.
//   - Enabled / Start / Close lifecycle helpers.
//
// The gateway keeps its own PostgreSQL schema (GATEWAY_DB_SCHEMA, default
// "gateway") and Redis, configured through GATEWAY_* environment variables.
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
	// InternalHost is the sentinel host for in-process calls. Configure a 幻图
	// OpenAI-compatible channel with base URL InternalBaseURL to use it.
	InternalHost = config.InternalHost
	// InternalBaseURL is the base URL of the built-in channel.
	InternalBaseURL = "http://" + config.InternalHost + "/v1"
	// PublicV1Prefix is the public OpenAI-compatible base path.
	PublicV1Prefix = config.V1PathPrefix
	// AdminPrefix is the admin control plane base path.
	AdminPrefix = config.AdminPathPrefix
)

// Module is a running gateway.
type Module struct {
	app      *bootstrap.App
	listener *memoryListener
	server   *http.Server
	builtin  atomic.Pointer[model.APICredential]
}

var current atomic.Pointer[Module]

// Enabled reports whether GATEWAY_ENABLED is truthy.
func Enabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// Current returns the running module, or nil when the gateway is disabled.
func Current() *Module { return current.Load() }

// Start boots the gateway (migrations, Redis, maintenance workers) and the
// in-process listener, and registers it as the current module.
func Start(ctx context.Context) (*Module, error) {
	app, err := bootstrap.NewApp(ctx)
	if err != nil {
		return nil, err
	}
	m := &Module{app: app, listener: newMemoryListener()}
	m.builtin.Store(app.Builtin)

	internal := gin.New()
	internal.Use(middleware.RequestID(), gin.Recovery())
	auth := middleware.UseCredential(m.builtin.Load)
	router.MountV1(internal.Group("/v1"), auth, app.Handlers)
	router.MountV1(internal.Group(PublicV1Prefix), auth, app.Handlers)
	m.server = &http.Server{Handler: internal, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := m.server.Serve(m.listener); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			log.Printf("gateway in-process server: %v", err)
		}
	}()
	current.Store(m)
	return m, nil
}

// Mount registers the public data plane and the admin control plane on the
// 幻图 engine. adminAuth must enforce 幻图's super-admin role.
func (m *Module) Mount(engine *gin.Engine, adminAuth gin.HandlerFunc) {
	external := middleware.RequireAPICredential(m.app.Credentials)
	router.MountV1(engine.Group("/v1"), external, m.app.Handlers)
	router.MountV1(engine.Group(PublicV1Prefix), external, m.app.Handlers)
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
				return nil, errors.New("内置网关未启用（GATEWAY_ENABLED=false）")
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

// Health is the connectivity state of the gateway's dependencies. Values are
// "ok" or "unavailable"; error details are never included.
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
		redisErr = errors.New("gateway redis not configured")
		if m.app.Redis != nil {
			redisErr = withDeadline(ctx, func() error { return m.app.Redis.Ping(ctx).Err() })
		}
	}()
	wg.Wait()
	h := Health{Status: "ok", Database: "ok", Redis: "ok"}
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
