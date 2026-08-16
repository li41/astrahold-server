package main

import (
	"net"
	"strings"
	"sync"
	"time"
)

const sessionLoginAbuseMaxTrackedSources = 4096

type sessionLoginAbuseEntry struct {
	windowStart time.Time
	attempts    int
	lastSeen    time.Time
}

type sessionLoginAbuseGuard struct {
	mu         sync.Mutex
	now        func() time.Time
	window     time.Duration
	maxAttempts int
	maxEntries int
	entries    map[string]sessionLoginAbuseEntry
}

func newSessionLoginAbuseGuard(window time.Duration, maxAttempts int, now func() time.Time) *sessionLoginAbuseGuard {
	if now == nil {
		now = time.Now
	}
	return &sessionLoginAbuseGuard{
		now:         now,
		window:      window,
		maxAttempts: maxAttempts,
		maxEntries:  sessionLoginAbuseMaxTrackedSources,
		entries:     make(map[string]sessionLoginAbuseEntry),
	}
}

func (g *sessionLoginAbuseGuard) Allow(remoteAddr string) (bool, time.Duration) {
	if g == nil || g.window <= 0 || g.maxAttempts <= 0 || g.maxEntries <= 0 {
		return true, 0
	}
	source := sessionLoginSourceIP(remoteAddr)
	if source == "" {
		return false, g.window
	}
	now := g.now().UTC()

	g.mu.Lock()
	defer g.mu.Unlock()
	if entry, exists := g.entries[source]; exists {
		if !now.Before(entry.windowStart.Add(g.window)) {
			entry = sessionLoginAbuseEntry{windowStart: now, attempts: 1, lastSeen: now}
			g.entries[source] = entry
			return true, 0
		}
		entry.lastSeen = now
		if entry.attempts >= g.maxAttempts {
			g.entries[source] = entry
			retry := entry.windowStart.Add(g.window).Sub(now)
			if retry < time.Second {
				retry = time.Second
			}
			return false, retry
		}
		entry.attempts++
		g.entries[source] = entry
		return true, 0
	}

	if len(g.entries) >= g.maxEntries {
		for key, entry := range g.entries {
			if !now.Before(entry.windowStart.Add(g.window)) {
				delete(g.entries, key)
			}
		}
	}
	if len(g.entries) >= g.maxEntries {
		return false, g.window
	}
	g.entries[source] = sessionLoginAbuseEntry{windowStart: now, attempts: 1, lastSeen: now}
	return true, 0
}

func sessionLoginSourceIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		ip := net.ParseIP(host)
		if ip == nil {
			return ""
		}
		return ip.String()
	}
	ip := net.ParseIP(remoteAddr)
	if ip == nil {
		return ""
	}
	return ip.String()
}
