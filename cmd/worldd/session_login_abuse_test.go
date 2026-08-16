package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionLoginAbuseGuardLimitsObservedSourceIP(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	guard := newSessionLoginAbuseGuard(time.Minute, 2, func() time.Time { return now })
	if allowed, _ := guard.Allow("192.0.2.10:4000"); !allowed {
		t.Fatal("first attempt rejected")
	}
	if allowed, _ := guard.Allow("192.0.2.10:4001"); !allowed {
		t.Fatal("second attempt rejected")
	}
	if allowed, retry := guard.Allow("192.0.2.10:4002"); allowed || retry <= 0 {
		t.Fatalf("third attempt allowed=%t retry=%s", allowed, retry)
	}
	if allowed, _ := guard.Allow("192.0.2.11:4000"); !allowed {
		t.Fatal("independent source IP should have its own window")
	}
	now = now.Add(time.Minute)
	if allowed, _ := guard.Allow("192.0.2.10:4003"); !allowed {
		t.Fatal("source window did not reset")
	}
}

func TestSessionLoginAbuseGuardDoesNotTrustForwardedFor(t *testing.T) {
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC) })
	runtime.abuseGuard = newSessionLoginAbuseGuard(time.Minute, 1, runtime.now)

	first := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(`{"login_id":"x","login_secret":"y"}`))
	first.RemoteAddr = "192.0.2.20:5000"
	first.Header.Set("X-Forwarded-For", "198.51.100.1")
	firstRecorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(firstRecorder, first)
	if firstRecorder.Code == http.StatusTooManyRequests {
		t.Fatal("first request unexpectedly throttled")
	}

	second := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(`{"login_id":"x","login_secret":"y"}`))
	second.RemoteAddr = "192.0.2.20:5001"
	second.Header.Set("X-Forwarded-For", "198.51.100.2")
	secondRecorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(secondRecorder, second)
	if secondRecorder.Code != http.StatusTooManyRequests || secondRecorder.Header().Get("Retry-After") == "" {
		t.Fatalf("status=%d retry-after=%q body=%s", secondRecorder.Code, secondRecorder.Header().Get("Retry-After"), secondRecorder.Body.String())
	}
}
