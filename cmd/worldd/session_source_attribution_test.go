package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSessionSourceAttributorRequiresCompleteConfiguration(t *testing.T) {
	if _, err := newSessionSourceAttributor("x-forwarded-for", ""); err == nil {
		t.Fatal("forwarded header without trusted proxy allowlist must fail")
	}
	if _, err := newSessionSourceAttributor("", "127.0.0.1/32"); err == nil {
		t.Fatal("trusted proxy allowlist without forwarded header must fail")
	}
	if _, err := newSessionSourceAttributor("x-real-ip", "127.0.0.1/32"); err == nil {
		t.Fatal("unsupported forwarded header mode must fail")
	}
	if attributor, err := newSessionSourceAttributor("", ""); err != nil || attributor != nil {
		t.Fatalf("disabled attribution=%v err=%v want nil,nil", attributor, err)
	}
}

func TestSessionSourceAttributorUntrustedPeerIgnoresForwardingMetadata(t *testing.T) {
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.1:5000"
	request.Header.Set("X-Forwarded-For", "malformed, deliberately-not-an-ip")
	request.Header.Set("Forwarded", `for="also-malformed"`)
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "127.0.0.1" {
		t.Fatalf("source=%q want socket peer 127.0.0.1", source)
	}
}

func TestSessionSourceAttributorXForwardedForStripsTrustedHopsRightToLeft(t *testing.T) {
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32,10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:5000"
	request.Header.Set("X-Forwarded-For", "198.51.100.20, 10.0.0.9")
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "198.51.100.20" {
		t.Fatalf("source=%q want 198.51.100.20", source)
	}
}

func TestSessionSourceAttributorNormalizesIPv4MappedIPv6(t *testing.T) {
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:5000"
	request.Header.Set("X-Forwarded-For", "::ffff:198.51.100.30")
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "198.51.100.30" {
		t.Fatalf("source=%q want normalized IPv4", source)
	}
}

func TestSessionSourceAttributorForwardedQuotedIPv6AndMultiHop(t *testing.T) {
	attributor, err := newSessionSourceAttributor("forwarded", "127.0.0.2/32,2001:db8::/32")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
	request.RemoteAddr = "127.0.0.2:5000"
	request.Header.Set("Forwarded", `for=198.51.100.40;proto=https, for="[2001:db8::5]:443";by=127.0.0.2`)
	source, err := attributor.sourceIP(request)
	if err != nil {
		t.Fatal(err)
	}
	if source != "198.51.100.40" {
		t.Fatalf("source=%q want 198.51.100.40", source)
	}
}

func TestSessionSourceAttributorTrustedPeerMalformedMetadataFailsClosed(t *testing.T) {
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		values []string
	}{
		{name: "missing"},
		{name: "malformed", values: []string{"not-an-ip"}},
		{name: "multiple-fields", values: []string{"198.51.100.1", "198.51.100.2"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
			request.RemoteAddr = "127.0.0.2:5000"
			for _, value := range testCase.values {
				request.Header.Add("X-Forwarded-For", value)
			}
			if _, err := attributor.sourceIP(request); err == nil {
				t.Fatal("trusted malformed forwarding metadata unexpectedly accepted")
			}
		})
	}
}

func TestSessionSourceAttributorForwardedRejectsObfuscatedAndDuplicateFor(t *testing.T) {
	attributor, err := newSessionSourceAttributor("forwarded", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{
		"for=unknown",
		"for=_hidden",
		"for=198.51.100.1;for=198.51.100.2",
		"proto=https",
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/session/login", nil)
		request.RemoteAddr = "127.0.0.2:5000"
		request.Header.Set("Forwarded", value)
		if _, err := attributor.sourceIP(request); err == nil {
			t.Fatalf("Forwarded value %q unexpectedly accepted", value)
		}
	}
}

func TestSessionSourceAttributionFeedsExistingLoginGuard(t *testing.T) {
	now := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	runtime := newTestSessionLoginRuntime(t, func() time.Time { return now })
	runtime.abuseGuard = newSessionLoginAbuseGuard(time.Minute, 1, runtime.now)
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	runtime.sourceAttributor = attributor

	request := func(clientIP string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(`{"login_id":"x","login_secret":"y"}`))
		r.RemoteAddr = "127.0.0.2:5000"
		r.Header.Set("X-Forwarded-For", clientIP)
		return r
	}

	first := httptest.NewRecorder()
	runtime.handler().ServeHTTP(first, request("198.51.100.50"))
	if first.Code == http.StatusTooManyRequests {
		t.Fatal("first client attempt unexpectedly throttled")
	}
	secondClient := httptest.NewRecorder()
	runtime.handler().ServeHTTP(secondClient, request("198.51.100.51"))
	if secondClient.Code == http.StatusTooManyRequests {
		t.Fatal("independent forwarded client unexpectedly shared bucket")
	}
	secondFirstClient := httptest.NewRecorder()
	runtime.handler().ServeHTTP(secondFirstClient, request("198.51.100.50"))
	if secondFirstClient.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d want 429", secondFirstClient.Code)
	}
}

func TestSessionSourceAttributionMiddlewareRejectsMalformedTrustedPostBeforeAuth(t *testing.T) {
	runtime := newTestSessionLoginRuntime(t, time.Now)
	attributor, err := newSessionSourceAttributor("x-forwarded-for", "127.0.0.2/32")
	if err != nil {
		t.Fatal(err)
	}
	runtime.sourceAttributor = attributor
	request := httptest.NewRequest(http.MethodPost, "/v1/session/login", strings.NewReader(`{"login_id":"x","login_secret":"y"}`))
	request.RemoteAddr = "127.0.0.2:5000"
	request.Header.Set("X-Forwarded-For", "invalid")
	recorder := httptest.NewRecorder()
	runtime.handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_request") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
