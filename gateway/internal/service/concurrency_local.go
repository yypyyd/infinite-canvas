package service

import (
	"sync"
	"time"
)

// localSlots is the in-process equivalent of the Redis sorted-set limiter and
// round-robin counters used when no Redis is configured.
type localSlots struct {
	mu      sync.Mutex
	slots   map[string]map[string]time.Time // key -> token -> expiry
	cursors map[string]uint64
	now     func() time.Time
}

func newLocalSlots() *localSlots {
	return &localSlots{slots: map[string]map[string]time.Time{}, cursors: map[string]uint64{}, now: time.Now}
}

func (l *localSlots) pruneLocked(key string, now time.Time) map[string]time.Time {
	set := l.slots[key]
	for token, expiry := range set {
		if !expiry.After(now) {
			delete(set, token)
		}
	}
	if len(set) == 0 {
		delete(l.slots, key)
		return nil
	}
	return set
}

func (l *localSlots) acquire(key string, max int, ttl time.Duration, token string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	set := l.pruneLocked(key, now)
	if _, held := set[token]; !held && max > 0 && len(set) >= max {
		return false
	}
	if set == nil {
		set = map[string]time.Time{}
		l.slots[key] = set
	}
	set[token] = now.Add(ttl)
	return true
}

func (l *localSlots) release(key, token string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if set := l.slots[key]; set != nil {
		delete(set, token)
		if len(set) == 0 {
			delete(l.slots, key)
		}
	}
}

func (l *localSlots) counts(keys []string) map[string]int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	out := make(map[string]int64, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		var n int64
		for _, expiry := range l.slots[key] {
			if expiry.After(now) {
				n++
			}
		}
		out[key] = n
	}
	return out
}

func (l *localSlots) next(key string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := l.cursors[key]
	l.cursors[key] = n + 1
	return n
}
