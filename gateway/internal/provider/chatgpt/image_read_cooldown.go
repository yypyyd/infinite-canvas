package chatgpt

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type conversationReadKey struct {
	account        [32]byte
	conversationID string
}

// A 429 for conversation reads applies to the credential. Retain its pause
// across recovery rounds and concurrent image tasks, without retaining tokens.
type imageReadCooldowns struct {
	mu       sync.Mutex
	accounts map[[32]byte]time.Time
}

func (p *imageReadCooldowns) until(key [32]byte) time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.accounts[key]
}

func (p *imageReadCooldowns) deferUntil(key [32]byte, until time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.accounts == nil {
		p.accounts = make(map[[32]byte]time.Time)
	}
	now := time.Now()
	for k, t := range p.accounts {
		if !t.After(now) {
			delete(p.accounts, k)
		}
	}
	if until.After(p.accounts[key]) {
		p.accounts[key] = until
	}
}

func (p *imageReadCooldowns) wait(ctx context.Context, key [32]byte) bool {
	for ctx.Err() == nil {
		delay := time.Until(p.until(key))
		if delay <= 0 {
			return true
		}
		if !waitImageRead(ctx, delay) {
			return false
		}
	}
	return false
}

func waitImageRead(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

// Retry-After permits either delay-seconds or an HTTP date.
func imageReadRetryAt(fallback time.Time, header string) time.Time {
	header = strings.TrimSpace(header)
	var until time.Time
	if seconds, err := strconv.ParseUint(header, 10, 63); err == nil {
		// Saturate at the maximum representable duration.
		until = time.Now().Add(time.Duration(min(seconds, uint64(math.MaxInt64/int64(time.Second)))) * time.Second)
	} else {
		until, _ = http.ParseTime(header)
	}
	if until.After(fallback) {
		return until
	}
	return fallback
}
