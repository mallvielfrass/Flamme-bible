package server

import (
	"sync"
	"time"
)

type hitWindow struct {
	limit  int
	window time.Duration
}

type ipLimiter struct {
	mu    sync.Mutex
	hits  map[string][]time.Time
	rules map[string]hitWindow
}

func newAuthLimiter() *ipLimiter {
	return &ipLimiter{
		hits: map[string][]time.Time{},
		rules: map[string]hitWindow{
			"/register": {limit: 5, window: time.Minute},
			"/login":    {limit: 10, window: time.Minute},
			"/refresh":  {limit: 30, window: time.Minute},
		},
	}
}

func (l *ipLimiter) allow(ip, path string, now time.Time) bool {
	if l == nil {
		return true
	}
	rule, ok := l.rules[path]
	if !ok {
		return true
	}
	key := path + "\n" + ip
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-rule.window)
	prev := l.hits[key]
	kept := prev[:0]
	for _, ts := range prev {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= rule.limit {
		if len(kept) == 0 {
			delete(l.hits, key)
		} else {
			l.hits[key] = kept
		}
		return false
	}
	l.hits[key] = append(kept, now)
	return true
}
