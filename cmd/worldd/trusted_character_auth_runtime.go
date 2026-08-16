package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

var (
	errTrustedCharacterAuthRuntimeReloadUnavailable = errors.New("worldd: trusted character auth runtime reload unavailable")
	errTrustedCharacterAuthRuntimeRequiresSchemaV2 = errors.New("worldd: trusted character auth runtime reload requires schema_version 2")
)

type reloadableTrustedCharacterCredentialProvider struct {
	mu      sync.RWMutex
	current *staticTrustedCharacterCredentialProvider
}

func newReloadableTrustedCharacterCredentialProvider(initial *staticTrustedCharacterCredentialProvider) (*reloadableTrustedCharacterCredentialProvider, error) {
	if initial == nil {
		return nil, errTrustedCharacterAuthConfig
	}
	return &reloadableTrustedCharacterCredentialProvider{current: initial}, nil
}

func (p *reloadableTrustedCharacterCredentialProvider) Resolve(ctx context.Context, credential []byte) (sessioncredential.Grant, error) {
	if p == nil {
		return sessioncredential.Grant{}, errTrustedCharacterAuthCredential
	}
	p.mu.RLock()
	current := p.current
	p.mu.RUnlock()
	if current == nil {
		return sessioncredential.Grant{}, errTrustedCharacterAuthCredential
	}
	return current.Resolve(ctx, credential)
}

func (p *reloadableTrustedCharacterCredentialProvider) snapshot() *staticTrustedCharacterCredentialProvider {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}

func (p *reloadableTrustedCharacterCredentialProvider) replace(next *staticTrustedCharacterCredentialProvider) {
	p.mu.Lock()
	p.current = next
	p.mu.Unlock()
}

type trustedCharacterAuthRuntime struct {
	path     string
	provider *reloadableTrustedCharacterCredentialProvider
	issued   *sessionLoginRuntime
}

type trustedCharacterAuthReloadResult struct {
	PreviousRevision string
	Revision         string
	ActiveScopes     int
	RetiredPeers     int
}

func loadRuntimeTrustedCharacterAuthenticator(path, tcpAddress string) (tcpudp.TrustedCharacterConnectionAuthenticator, *trustedCharacterAuthRuntime, string, error) {
	issuedRuntime, err := loadSessionLoginRuntime(tcpAddress)
	if err != nil {
		return nil, nil, "", err
	}
	if issuedRuntime != nil {
		if strings.TrimSpace(path) != "" {
			_ = issuedRuntime.Close()
			return nil, nil, "", errSessionLoginAuthModeConflict
		}
		authenticator, err := newTrustedCharacterAuthenticatorWithProvider(issuedRuntime.provider)
		if err != nil {
			_ = issuedRuntime.Close()
			return nil, nil, "", err
		}
		return authenticator.Authenticate, &trustedCharacterAuthRuntime{
			provider: issuedRuntime.provider,
			issued:   issuedRuntime,
		}, "session-login/" + issuedRuntime.accountAuth.Revision(), nil
	}

	if strings.TrimSpace(path) == "" {
		return nil, nil, "", nil
	}
	if err := validateTrustedCharacterAuthListenAddress(tcpAddress); err != nil {
		return nil, nil, "", err
	}
	initial, err := loadStaticTrustedCharacterCredentialProvider(path)
	if err != nil {
		return nil, nil, "", err
	}
	if initial.schemaVersion != trustedCharacterAuthSchemaVersion {
		authenticator, err := newTrustedCharacterAuthenticatorWithProvider(initial)
		if err != nil {
			return nil, nil, "", err
		}
		// Schema v1 remains startup-compatible but deliberately cannot participate in
		// live proof-generation invalidation because it has no credential_id lifecycle.
		return authenticator.Authenticate, nil, initial.revision, nil
	}
	reloadable, err := newReloadableTrustedCharacterCredentialProvider(initial)
	if err != nil {
		return nil, nil, "", err
	}
	authenticator, err := newTrustedCharacterAuthenticatorWithProvider(reloadable)
	if err != nil {
		return nil, nil, "", err
	}
	return authenticator.Authenticate, &trustedCharacterAuthRuntime{path: path, provider: reloadable}, initial.revision, nil
}

func activeTrustedCharacterAuthenticationScopes(provider *staticTrustedCharacterCredentialProvider, now time.Time) []string {
	if provider == nil || provider.schemaVersion != trustedCharacterAuthSchemaVersion || now.IsZero() {
		return nil
	}
	scopes := make([]string, 0, len(provider.credentialsByID))
	for _, entry := range provider.credentialsByID {
		if entry.revocationScope == "" {
			continue
		}
		if err := entry.lifecycle.ValidateAt(now.UTC()); err != nil {
			continue
		}
		scopes = append(scopes, entry.revocationScope)
	}
	sort.Strings(scopes)
	return scopes
}

func nextTrustedCharacterAuthenticationBoundary(provider *staticTrustedCharacterCredentialProvider, now time.Time) (time.Time, bool) {
	if provider == nil || provider.schemaVersion != trustedCharacterAuthSchemaVersion || now.IsZero() {
		return time.Time{}, false
	}
	var next time.Time
	consider := func(candidate time.Time) {
		if candidate.IsZero() || !candidate.After(now) {
			return
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	for _, entry := range provider.credentialsByID {
		consider(entry.lifecycle.NotBefore)
		consider(entry.lifecycle.ExpiresAt)
		consider(entry.lifecycle.RevokedAt)
	}
	return next, !next.IsZero()
}

func applyTrustedCharacterAuthReload(runtime *trustedCharacterAuthRuntime, replaceScopes func([]string) int, now time.Time) (trustedCharacterAuthReloadResult, error) {
	if runtime == nil || runtime.provider == nil || strings.TrimSpace(runtime.path) == "" || replaceScopes == nil || runtime.issued != nil {
		return trustedCharacterAuthReloadResult{}, errTrustedCharacterAuthRuntimeReloadUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	previous := runtime.provider.snapshot()
	if previous == nil || previous.schemaVersion != trustedCharacterAuthSchemaVersion {
		return trustedCharacterAuthReloadResult{}, errTrustedCharacterAuthRuntimeRequiresSchemaV2
	}
	next, err := loadStaticTrustedCharacterCredentialProvider(runtime.path)
	if err != nil {
		return trustedCharacterAuthReloadResult{}, err
	}
	if next.schemaVersion != trustedCharacterAuthSchemaVersion {
		return trustedCharacterAuthReloadResult{}, errTrustedCharacterAuthRuntimeRequiresSchemaV2
	}

	// Install the transport fence before publishing the new provider. Any in-flight
	// authentication result from the old provider whose proof generation disappeared
	// will fail registerPeer even if it reaches publication after this point.
	scopes := activeTrustedCharacterAuthenticationScopes(next, now)
	retired := replaceScopes(scopes)
	runtime.provider.replace(next)
	return trustedCharacterAuthReloadResult{
		PreviousRevision: previous.revision,
		Revision:         next.revision,
		ActiveScopes:     len(scopes),
		RetiredPeers:     retired,
	}, nil
}

func syncTrustedCharacterAuthScopes(runtime *trustedCharacterAuthRuntime, replaceScopes func([]string) int, now time.Time) int {
	if runtime == nil || runtime.provider == nil || replaceScopes == nil || now.IsZero() {
		return 0
	}
	current := runtime.provider.snapshot()
	return replaceScopes(activeTrustedCharacterAuthenticationScopes(current, now.UTC()))
}

func runTrustedCharacterAuthRuntime(
	ctx context.Context,
	reloadSignals <-chan os.Signal,
	runtime *trustedCharacterAuthRuntime,
	replaceScopes func([]string) int,
	logf func(string, ...any),
) {
	if ctx == nil || runtime == nil || replaceScopes == nil {
		return
	}
	if runtime.issued != nil {
		runIssuedSessionCredentialRuntime(ctx, reloadSignals, runtime.issued, replaceScopes, logf)
		return
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	for {
		current := runtime.provider.snapshot()
		now := time.Now().UTC()
		boundary, hasBoundary := nextTrustedCharacterAuthenticationBoundary(current, now)
		var timer *time.Timer
		var timerC <-chan time.Time
		if hasBoundary {
			delay := time.Until(boundary)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			timerC = timer.C
		}

		select {
		case <-ctx.Done():
			stopTrustedCharacterAuthTimer(timer)
			return
		case <-reloadSignals:
			stopTrustedCharacterAuthTimer(timer)
			result, err := applyTrustedCharacterAuthReload(runtime, replaceScopes, time.Now().UTC())
			if err != nil {
				logf("trusted character auth reload rejected; last-known-good retained: err=%v", err)
				continue
			}
			logf("trusted character auth reload applied: previous_revision=%s revision=%s active_scopes=%d retired_peers=%d", result.PreviousRevision, result.Revision, result.ActiveScopes, result.RetiredPeers)
		case <-timerC:
			retired := syncTrustedCharacterAuthScopes(runtime, replaceScopes, time.Now().UTC())
			if retired > 0 {
				logf("trusted character auth lifecycle boundary retired peers: retired_peers=%d", retired)
			}
		}
	}
}

func stopTrustedCharacterAuthTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func describeTrustedCharacterAuthRuntime(runtime *trustedCharacterAuthRuntime) string {
	if runtime == nil || runtime.provider == nil {
		return "disabled"
	}
	if runtime.issued != nil {
		return fmt.Sprintf("issued-session/login-tls@%s", runtime.issued.Addr())
	}
	current := runtime.provider.snapshot()
	if current == nil {
		return "disabled"
	}
	return fmt.Sprintf("sighup+boundaries/schema-v%d", current.schemaVersion)
}
