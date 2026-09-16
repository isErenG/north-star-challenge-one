package enrich

import (
	"context"
	"sync"
	"time"
)

// Limiter is a minimal token-interval limiter: at most one call per interval
// per key. Keys let the website source pace per host.
type Limiter struct {
	mu       sync.Mutex
	interval time.Duration
	next     map[string]time.Time
}

func NewLimiter(perSecond float64) *Limiter {
	if perSecond <= 0 {
		perSecond = 1
	}
	return &Limiter{interval: time.Duration(float64(time.Second) / perSecond), next: map[string]time.Time{}}
}

// Wait blocks until the key may proceed or ctx ends.
func (l *Limiter) Wait(ctx context.Context, key string) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next[key]
	if at.Before(now) {
		at = now
	}
	l.next[key] = at.Add(l.interval)
	l.mu.Unlock()
	select {
	case <-time.After(time.Until(at)):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
