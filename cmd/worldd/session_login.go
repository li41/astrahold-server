package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

const (
	sessionLoginSchemaVersion          uint16 = 1
	sessionLoginMaxIDBytes                    = 128
	sessionLoginMaxSecretBytes                = 256
	sessionLoginMaxRequestBytes               = 4096
	issuedSessionCredentialRandomBytes        = 32
	issuedSessionCredentialMaxTTL             = 24 * time.Hour
	issuedSessionCredentialMinTTL             = time.Minute
	issuedSessionScopePrefix                  = "issued-v1:"
)

var (
	sessionLoginAccountFile = flag.String(
		"session-login-account-file",
		"",
		"Optional server-side account verifier map (schema v1 high-entropy SHA-256, schema v2 Argon2id password, or durable schema v3/v4 account store) used to issue short-lived opaque session credentials",
	)
	sessionLoginTLSListen = flag.String(
		"session-login-tls-listen",
		"",
		"TLS 1.3 HTTPS listen address for formal session credential issuance",
	)
	sessionLoginTLSCertFile = flag.String(
		"session-login-tls-cert",
		"",
		"PEM certificate chain for -session-login-tls-listen; reloaded with its key on SIGHUP",
	)
	sessionLoginTLSKeyFile = flag.String(
		"session-login-tls-key",
		"",
		"PEM private key for -session-login-tls-listen; reloaded with its certificate on SIGHUP",
	)
	issuedSessionCredentialTTL = flag.Duration(
		"session-credential-ttl",
		15*time.Minute,
		"Lifetime of an issued opaque session credential (1m..24h)",
	)
	sessionLoginIPAttemptWindow = flag.Duration(
		"session-login-ip-attempt-window",
		time.Minute,
		"Fixed source-IP login attempt window used before password KDF work (1s..1h)",
	)
	sessionLoginIPMaxAttempts = flag.Int(
		"session-login-ip-max-attempts",
		30,
		"Maximum login POST attempts per observed TLS source IP in one attempt window (1..10000)",
	)
)

var (
	errSessionLoginConfig             = errors.New("worldd: invalid session login config")
	errSessionLoginRejected           = errors.New("worldd: session login rejected")
	errSessionLoginRuntimeUnavailable = errors.New("worldd: session login runtime unavailable")
	errSessionLoginAuthModeConflict   = errors.New("worldd: session login issuance and static trusted-character auth are mutually exclusive")
)

type sessionLoginDefinition struct {
	SchemaVersion uint16                `json:"schema_version"`
	Revision      string                `json:"revision"`
	Accounts      []sessionLoginAccount `json:"accounts"`
}

type sessionLoginAccount struct {
	LoginID             string `json:"login_id"`
	LoginSecretSHA256   string `json:"login_secret_sha256"`
	CharacterID         string `json:"character_id"`
	AllowActiveTakeover bool   `json:"allow_active_takeover,omitempty"`
}

type staticSessionLoginAccount struct {
	secretDigest [sha256.Size]byte
	grant        sessioncredential.Grant
}

type staticSessionLoginAuthenticator struct {
	revision string
	accounts map[string]staticSessionLoginAccount
}

func loadStaticSessionLoginAuthenticator(path string) (*staticSessionLoginAuthenticator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session login config %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition sessionLoginDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", errSessionLoginConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionLoginConfig)
		}
		return nil, fmt.Errorf("%w: trailing data: %v", errSessionLoginConfig, err)
	}
	return newStaticSessionLoginAuthenticator(definition)
}

func newStaticSessionLoginAuthenticator(definition sessionLoginDefinition) (*staticSessionLoginAuthenticator, error) {
	if definition.SchemaVersion != sessionLoginSchemaVersion || strings.TrimSpace(definition.Revision) == "" || len(definition.Accounts) == 0 {
		return nil, errSessionLoginConfig
	}
	accounts := make(map[string]staticSessionLoginAccount, len(definition.Accounts))
	for index, item := range definition.Accounts {
		loginID := strings.TrimSpace(item.LoginID)
		if loginID == "" || loginID != item.LoginID || len(loginID) > sessionLoginMaxIDBytes {
			return nil, fmt.Errorf("%w: account[%d] login_id must be 1..%d trimmed bytes", errSessionLoginConfig, index, sessionLoginMaxIDBytes)
		}
		if _, exists := accounts[loginID]; exists {
			return nil, fmt.Errorf("%w: duplicate login_id %q", errSessionLoginConfig, loginID)
		}
		if len(item.LoginSecretSHA256) != sha256.Size*2 || strings.ToLower(item.LoginSecretSHA256) != item.LoginSecretSHA256 {
			return nil, fmt.Errorf("%w: account[%d] login_secret_sha256 must be 64 lowercase hex characters", errSessionLoginConfig, index)
		}
		decoded, err := hex.DecodeString(item.LoginSecretSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("%w: account[%d] login_secret_sha256", errSessionLoginConfig, index)
		}
		binding, err := characteridentity.NewTrusted(item.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("%w: account[%d] character_id: %v", errSessionLoginConfig, index, err)
		}
		var digest [sha256.Size]byte
		copy(digest[:], decoded)
		accounts[loginID] = staticSessionLoginAccount{
			secretDigest: digest,
			grant: sessioncredential.Grant{
				Identity:            binding,
				AllowActiveTakeover: item.AllowActiveTakeover,
			},
		}
	}
	return &staticSessionLoginAuthenticator{revision: definition.Revision, accounts: accounts}, nil
}

func (a *staticSessionLoginAuthenticator) Authenticate(ctx context.Context, loginID string, secret []byte) (sessioncredential.Grant, error) {
	if a == nil || len(a.accounts) == 0 || loginID == "" || len(loginID) > sessionLoginMaxIDBytes || len(secret) == 0 || len(secret) > sessionLoginMaxSecretBytes {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	if ctx != nil {
		select {
		case <-ctx.Done():
			return sessioncredential.Grant{}, ctx.Err()
		default:
		}
	}
	account, ok := a.accounts[loginID]
	if !ok {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	digest := sha256.Sum256(secret)
	if subtle.ConstantTimeCompare(digest[:], account.secretDigest[:]) != 1 {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	if !account.grant.Valid() {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	return account.grant, nil
}

type sessionLoginRuntime struct {
	accountPath      string
	accountAuth      *sessionAccountAuthRuntime
	abuseGuard       *sessionLoginAbuseGuard
	recoveryProvider accountrecovery.Provider
	recoveryGuard    *sessionLoginAbuseGuard
	provider         *reloadableTrustedCharacterCredentialProvider
	listener         net.Listener
	tlsCertificate   *reloadableTLSCertificate
	sourceAttributor *sessionSourceAttributor
	ttl              time.Duration
	now              func() time.Time
	random           io.Reader
	changed          chan struct{}

	mu            sync.Mutex
	replaceScopes func([]string) int
	revision      uint64
}

func sessionLoginConfigurationRequested() bool {
	return strings.TrimSpace(*sessionLoginAccountFile) != "" ||
		strings.TrimSpace(*sessionLoginTLSListen) != "" ||
		strings.TrimSpace(*sessionLoginTLSCertFile) != "" ||
		strings.TrimSpace(*sessionLoginTLSKeyFile) != "" ||
		strings.TrimSpace(*sessionRecoveryProviderFile) != "" ||
		strings.TrimSpace(*sessionLoginTrustedProxyCIDRs) != "" ||
		strings.TrimSpace(*sessionLoginForwardedHeader) != "" ||
		strings.TrimSpace(*sessionLoginTrustedProxyMTLSFile) != "" ||
		strings.TrimSpace(*sessionLoginTrustedProxyEdgePolicyFile) != "" ||
		sessionEdgeConnectionRetirementRequested()
}

func loadSessionLoginRuntime(tcpAddress string) (*sessionLoginRuntime, error) {
	if !sessionLoginConfigurationRequested() {
		return nil, nil
	}
	accountPath := strings.TrimSpace(*sessionLoginAccountFile)
	listenAddress := strings.TrimSpace(*sessionLoginTLSListen)
	certFile := strings.TrimSpace(*sessionLoginTLSCertFile)
	keyFile := strings.TrimSpace(*sessionLoginTLSKeyFile)
	if accountPath == "" || listenAddress == "" || certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("%w: session-login-account-file, session-login-tls-listen, session-login-tls-cert, and session-login-tls-key must be set together", errSessionLoginConfig)
	}
	if *issuedSessionCredentialTTL < issuedSessionCredentialMinTTL || *issuedSessionCredentialTTL > issuedSessionCredentialMaxTTL {
		return nil, fmt.Errorf("%w: session-credential-ttl must be between %s and %s", errSessionLoginConfig, issuedSessionCredentialMinTTL, issuedSessionCredentialMaxTTL)
	}
	if *sessionLoginIPAttemptWindow < time.Second || *sessionLoginIPAttemptWindow > time.Hour {
		return nil, fmt.Errorf("%w: session-login-ip-attempt-window must be between 1s and 1h", errSessionLoginConfig)
	}
	if *sessionLoginIPMaxAttempts < 1 || *sessionLoginIPMaxAttempts > 10000 {
		return nil, fmt.Errorf("%w: session-login-ip-max-attempts must be between 1 and 10000", errSessionLoginConfig)
	}
	sourceAttributor, err := loadSessionSourceAttributor()
	if err != nil {
		return nil, err
	}
	if sessionEdgeConnectionRetirementRequested() && (sourceAttributor == nil || sourceAttributor.edgePolicy == nil) {
		return nil, fmt.Errorf("%w: session-login-trusted-proxy-edge-retire-old-connections requires session-login-trusted-proxy-edge-policy-file", errSessionLoginConfig)
	}
	if err := validateTrustedCharacterAuthListenAddress(tcpAddress); err != nil {
		return nil, err
	}
	if _, _, err := net.SplitHostPort(listenAddress); err != nil {
		return nil, fmt.Errorf("%w: session login listen address: %v", errSessionLoginConfig, err)
	}
	authenticator, err := loadSessionAccountAuthenticator(accountPath)
	if err != nil {
		return nil, err
	}
	accountAuth, err := newSessionAccountAuthRuntime(authenticator)
	if err != nil {
		return nil, err
	}
	recoveryProvider, recoveryGuard, err := loadSessionRecoveryProvider(accountAuth)
	if err != nil {
		return nil, err
	}
	if staticRecovery, ok := recoveryProvider.(*staticSessionRecoveryProvider); ok {
		recoveryProvider, err = wrapSessionRecoveryProviderForRuntime(staticRecovery)
		if err != nil {
			return nil, err
		}
	}
	tlsCertificate, err := newReloadableTLSCertificate(certFile, keyFile, time.Now)
	if err != nil {
		return nil, fmt.Errorf("%w: load session login certificate: %v", errSessionLoginConfig, err)
	}
	tlsConfig, err := sourceAttributor.TLSConfig(tlsCertificate)
	if err != nil {
		return nil, fmt.Errorf("%w: configure session login TLS edge trust: %v", errSessionLoginConfig, err)
	}
	base, err := net.Listen("tcp", listenAddress)
	if err != nil {
		return nil, fmt.Errorf("open session login listener: %w", err)
	}
	initial := newEmptyIssuedSessionCredentialProvider(time.Now)
	provider, err := newReloadableTrustedCharacterCredentialProvider(initial)
	if err != nil {
		_ = base.Close()
		return nil, err
	}
	return &sessionLoginRuntime{
		accountPath:      accountPath,
		accountAuth:      accountAuth,
		abuseGuard:       newSessionLoginAbuseGuard(*sessionLoginIPAttemptWindow, *sessionLoginIPMaxAttempts, time.Now),
		recoveryProvider: recoveryProvider,
		recoveryGuard:    recoveryGuard,
		provider:         provider,
		listener:         tls.NewListener(base, tlsConfig),
		tlsCertificate:   tlsCertificate,
		sourceAttributor: sourceAttributor,
		ttl:              *issuedSessionCredentialTTL,
		now:              time.Now,
		random:           rand.Reader,
		changed:          make(chan struct{}, 1),
	}, nil
}

func newEmptyIssuedSessionCredentialProvider(now func() time.Time) *staticTrustedCharacterCredentialProvider {
	if now == nil {
		now = time.Now
	}
	return &staticTrustedCharacterCredentialProvider{
		schemaVersion:   trustedCharacterAuthSchemaVersion,
		revision:        "issued-session-0",
		credentials:     make(map[[sha256.Size]byte]staticTrustedCharacterCredentialEntry),
		credentialsByID: make(map[string]staticTrustedCharacterCredentialEntry),
		now:             now,
	}
}

func cloneIssuedSessionCredentialProvider(current *staticTrustedCharacterCredentialProvider, revision uint64, now func() time.Time) *staticTrustedCharacterCredentialProvider {
	next := newEmptyIssuedSessionCredentialProvider(now)
	next.revision = fmt.Sprintf("issued-session-%d", revision)
	if current == nil {
		return next
	}
	for digest, entry := range current.credentials {
		next.credentials[digest] = entry
	}
	for credentialID, entry := range current.credentialsByID {
		next.credentialsByID[credentialID] = entry
	}
	return next
}

func pruneIssuedSessionCredentials(provider *staticTrustedCharacterCredentialProvider, now time.Time) {
	if provider == nil || now.IsZero() {
		return
	}
	for digest, entry := range provider.credentials {
		if err := entry.lifecycle.ValidateAt(now.UTC()); err == nil {
			continue
		}
		delete(provider.credentials, digest)
		if entry.credentialID != "" {
			delete(provider.credentialsByID, entry.credentialID)
		}
	}
}

func (r *sessionLoginRuntime) Issue(ctx context.Context, grant sessioncredential.Grant) (sessioncredential.IssuedCredential, error) {
	if r == nil || r.provider == nil || r.random == nil || r.now == nil || !grant.Valid() {
		return sessioncredential.IssuedCredential{}, errSessionLoginRuntimeUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return sessioncredential.IssuedCredential{}, ctx.Err()
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceScopes == nil {
		return sessioncredential.IssuedCredential{}, errSessionLoginRuntimeUnavailable
	}
	if !r.accountAuth.grantCurrent(grant) {
		return sessioncredential.IssuedCredential{}, errSessionLoginRejected
	}
	now := r.now().UTC()
	nextRevision := r.revision + 1
	next := cloneIssuedSessionCredentialProvider(r.provider.snapshot(), nextRevision, r.now)
	pruneIssuedSessionCredentials(next, now)

	var token string
	var digest [sha256.Size]byte
	for attempt := 0; attempt < 4; attempt++ {
		var randomBytes [issuedSessionCredentialRandomBytes]byte
		if _, err := io.ReadFull(r.random, randomBytes[:]); err != nil {
			return sessioncredential.IssuedCredential{}, fmt.Errorf("issue session credential random: %w", err)
		}
		token = base64.RawURLEncoding.EncodeToString(randomBytes[:])
		clear(randomBytes[:])
		digest = sha256.Sum256([]byte(token))
		if _, collision := next.credentials[digest]; !collision {
			break
		}
		token = ""
	}
	if token == "" {
		return sessioncredential.IssuedCredential{}, fmt.Errorf("issue session credential: repeated random collision")
	}

	expiresAt := now.Add(r.ttl)
	credentialID := "issued:" + hex.EncodeToString(digest[:16])
	scope := issuedSessionScopePrefix + hex.EncodeToString(digest[:])
	entry := staticTrustedCharacterCredentialEntry{
		credentialID:    credentialID,
		tokenDigest:     digest,
		revocationScope: scope,
		grant: sessioncredential.Grant{
			Identity:                 grant.Identity,
			AllowActiveTakeover:      grant.AllowActiveTakeover,
			RevocationScope:          scope,
			AuthenticationSubject:    grant.AuthenticationSubject,
			AuthenticationGeneration: grant.AuthenticationGeneration,
		},
		lifecycle: sessioncredential.Lifecycle{ExpiresAt: expiresAt},
	}
	next.credentials[digest] = entry
	next.credentialsByID[credentialID] = entry

	// Publish the transport allow-set before the credential provider. During the
	// tiny handoff window the new scope is allowed but the new bearer still cannot
	// resolve, which is fail-closed and prevents an issued proof from outrunning
	// its transport revocation fence.
	r.replaceScopes(activeTrustedCharacterAuthenticationScopes(next, now))
	r.provider.replace(next)
	r.revision = nextRevision
	r.signalChanged()

	issued := sessioncredential.IssuedCredential{Credential: token, ExpiresAt: expiresAt}
	if !issued.Valid() {
		return sessioncredential.IssuedCredential{}, sessioncredential.ErrInvalidIssuedCredential
	}
	return issued, nil
}

func (r *sessionLoginRuntime) Revoke(ctx context.Context, credential []byte) (bool, error) {
	if r == nil || r.provider == nil || r.now == nil || len(credential) == 0 || len(credential) > trustedCharacterAuthMaxCredentialBytes {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}
	digest := sha256.Sum256(credential)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceScopes == nil {
		return false, errSessionLoginRuntimeUnavailable
	}
	current := r.provider.snapshot()
	entry, exists := current.credentials[digest]
	if !exists {
		return false, nil
	}
	now := r.now().UTC()
	nextRevision := r.revision + 1
	next := cloneIssuedSessionCredentialProvider(current, nextRevision, r.now)
	delete(next.credentials, digest)
	if entry.credentialID != "" {
		delete(next.credentialsByID, entry.credentialID)
	}
	pruneIssuedSessionCredentials(next, now)

	// Revocation fence first: live peers lose realtime lookup before the provider
	// stops resolving the bearer, preserving the S4-F.3 ordering.
	r.replaceScopes(activeTrustedCharacterAuthenticationScopes(next, now))
	r.provider.replace(next)
	r.revision = nextRevision
	r.signalChanged()
	return true, nil
}

func (r *sessionLoginRuntime) expireAt(now time.Time) int {
	if r == nil || r.provider == nil || r.now == nil || now.IsZero() {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.replaceScopes == nil {
		return 0
	}
	current := r.provider.snapshot()
	nextRevision := r.revision + 1
	next := cloneIssuedSessionCredentialProvider(current, nextRevision, r.now)
	before := len(next.credentials)
	pruneIssuedSessionCredentials(next, now.UTC())
	if len(next.credentials) == before {
		return 0
	}
	retired := r.replaceScopes(activeTrustedCharacterAuthenticationScopes(next, now.UTC()))
	r.provider.replace(next)
	r.revision = nextRevision
	return retired
}

func (r *sessionLoginRuntime) signalChanged() {
	if r == nil || r.changed == nil {
		return
	}
	select {
	case r.changed <- struct{}{}:
	default:
	}
}

func (r *sessionLoginRuntime) Addr() net.Addr {
	if r == nil || r.listener == nil {
		return nil
	}
	return r.listener.Addr()
}

func (r *sessionLoginRuntime) TLSCertificateSnapshot() tlsCertificateSnapshot {
	if r == nil || r.tlsCertificate == nil {
		return tlsCertificateSnapshot{}
	}
	return r.tlsCertificate.Snapshot()
}

func (r *sessionLoginRuntime) reloadTLSCertificate() (tlsCertificateReloadResult, error) {
	if r == nil || r.tlsCertificate == nil {
		return tlsCertificateReloadResult{}, errTLSCertificateReloadConfig
	}
	return r.tlsCertificate.Reload()
}

func (r *sessionLoginRuntime) Close() error {
	if r == nil || r.listener == nil {
		return nil
	}
	return r.listener.Close()
}

type sessionLoginRequest struct {
	LoginID     string `json:"login_id"`
	LoginSecret string `json:"login_secret"`
}

type sessionLoginResponse struct {
	SessionCredential string `json:"session_credential"`
	ExpiresAt         string `json:"expires_at"`
}

func (r *sessionLoginRuntime) handler() http.Handler {
	mux := http.NewServeMux()
	loginHandler := http.Handler(http.HandlerFunc(r.handleSessionLogin))
	if r.sourceAttributor != nil {
		loginHandler = r.sourceAttributor.wrap(loginHandler)
	}
	mux.Handle("/v1/session/login", loginHandler)
	mux.HandleFunc("/v1/session/logout", r.handleSessionLogout)
	if r.recoveryProvider != nil {
		recoveryRequestHandler := http.Handler(http.HandlerFunc(r.handleRecoveryRequest))
		recoveryResetHandler := http.Handler(http.HandlerFunc(r.handleRecoveryReset))
		if r.sourceAttributor != nil {
			recoveryRequestHandler = r.sourceAttributor.wrap(recoveryRequestHandler)
			recoveryResetHandler = r.sourceAttributor.wrap(recoveryResetHandler)
		}
		mux.Handle("/v1/account/recovery/request", recoveryRequestHandler)
		mux.Handle("/v1/account/recovery/reset", recoveryResetHandler)
	}
	return mux
}

func (r *sessionLoginRuntime) handleSessionLogin(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeSessionLoginError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if r.abuseGuard != nil {
		allowed, retry := r.abuseGuard.Allow(request.RemoteAddr)
		if !allowed {
			seconds := int((retry + time.Second - 1) / time.Second)
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeSessionLoginError(w, http.StatusTooManyRequests, "login_throttled")
			return
		}
	}
	request.Body = http.MaxBytesReader(w, request.Body, sessionLoginMaxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input sessionLoginRequest
	if err := decoder.Decode(&input); err != nil {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	secret := []byte(input.LoginSecret)
	defer clear(secret)
	grant, err := r.accountAuth.Authenticate(request.Context(), input.LoginID, secret)
	if err != nil {
		writeSessionLoginError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	issued, err := r.Issue(request.Context(), grant)
	if err != nil {
		if errors.Is(err, errSessionLoginRejected) {
			writeSessionLoginError(w, http.StatusUnauthorized, "invalid_credentials")
			return
		}
		writeSessionLoginError(w, http.StatusServiceUnavailable, "issuance_unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeSessionLoginJSON(w, http.StatusOK, sessionLoginResponse{
		SessionCredential: issued.Credential,
		ExpiresAt:         issued.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

func (r *sessionLoginRuntime) handleSessionLogout(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeSessionLoginError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		writeSessionLoginError(w, http.StatusUnauthorized, "invalid_session")
		return
	}
	credential := []byte(strings.TrimPrefix(authorization, prefix))
	defer clear(credential)
	if len(credential) == 0 || len(credential) > trustedCharacterAuthMaxCredentialBytes {
		writeSessionLoginError(w, http.StatusUnauthorized, "invalid_session")
		return
	}
	_, err := r.Revoke(request.Context(), credential)
	if err != nil {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "revocation_unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func writeSessionLoginError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Cache-Control", "no-store")
	writeSessionLoginJSON(w, status, map[string]string{"error": code})
}

func writeSessionLoginJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (r *sessionLoginRuntime) serve(ctx context.Context) error {
	if r == nil || r.listener == nil || r.accountAuth == nil || r.provider == nil {
		return errSessionLoginRuntimeUnavailable
	}
	server := &http.Server{
		Handler:           r.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       30 * time.Second,
		ConnState: func(connection net.Conn, state http.ConnState) {
			if r.sourceAttributor == nil || connection == nil || connection.RemoteAddr() == nil {
				return
			}
			r.sourceAttributor.observeEdgeConnectionState(connection, state)
		},
	}
	shutdownDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(shutdownCtx)
			cancel()
		case <-shutdownDone:
		}
	}()
	err := server.Serve(r.listener)
	close(shutdownDone)
	if errors.Is(err, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return err
}

func runIssuedSessionCredentialRuntime(
	ctx context.Context,
	reloadSignals <-chan os.Signal,
	runtime *sessionLoginRuntime,
	replaceScopes func([]string) int,
	logf func(string, ...any),
) {
	if ctx == nil || runtime == nil || runtime.provider == nil || replaceScopes == nil {
		return
	}
	if logf == nil {
		logf = func(string, ...any) {}
	}
	runtime.mu.Lock()
	runtime.replaceScopes = replaceScopes
	runtime.mu.Unlock()
	replaceScopes(activeTrustedCharacterAuthenticationScopes(runtime.provider.snapshot(), time.Now().UTC()))

	serviceDone := make(chan error, 1)
	go func() { serviceDone <- runtime.serve(ctx) }()
	_, _, durableReload := runtime.accountAuth.reloadMetadata()
	reloadMode := "restart-only"
	if durableReload {
		reloadMode = "sighup"
	}
	tlsSnapshot := runtime.TLSCertificateSnapshot()
	sourceAttribution, trustedProxyPrefixes := sessionSourceAttributionMetadata(runtime.sourceAttributor)
	proxyAuth, proxyTrust := sessionProxyMTLSMetadataForAttributor(runtime.sourceAttributor)
	edgePolicy := sessionEdgePolicyMetadataForAttributor(runtime.sourceAttributor)
	edgeConnectionCutover := runtime.sourceAttributor.edgeConnectionCutoverMode()
	logf("session login issuance: enabled=true revision=%s listen=%s min_tls=1.3 session_ttl=%s account_auth=%s account_reload=%s tls_reload=sighup tls_generation=%d tls_not_after=%s source_attribution=%s trusted_proxy_prefixes=%d forwarded_max_hops=%d proxy_auth=%s proxy_trust_generation=%d proxy_trust_revision=%s proxy_identity_count=%d edge_policy_generation=%d edge_policy_revision=%s edge_policy_bindings=%d edge_policy_connection_cutover=%s login_ip_limit=%d/%s restart_persistence=false", runtime.accountAuth.Revision(), runtime.Addr(), runtime.ttl, runtime.accountAuth.Method(), reloadMode, tlsSnapshot.Generation, tlsSnapshot.NotAfter.UTC().Format(time.RFC3339Nano), sourceAttribution, trustedProxyPrefixes, sessionSourceAttributionMaxHops, proxyAuth, proxyTrust.Generation, proxyTrust.Revision, proxyTrust.IdentityCount, edgePolicy.Generation, edgePolicy.Revision, edgePolicy.BindingCount, edgeConnectionCutover, runtime.abuseGuard.maxAttempts, runtime.abuseGuard.window)
	leafRevocation := sessionLeafRevocationMetadataForAttributor(runtime.sourceAttributor)
	if leafRevocation.Generation != 0 {
		logf("session trusted proxy leaf revocation: enabled=true generation=%d revision=%s revoked_credentials=%d identifier=spki-sha256", leafRevocation.Generation, leafRevocation.Revision, leafRevocation.RevokedCredentialCount)
	}
	if runtime.recoveryProvider != nil {
		recoveryReloadMode, recoveryGeneration := sessionRecoveryReloadMetadata(runtime.recoveryProvider)
		logf("session recovery: enabled=true provider=%s revision=%s challenge_ttl=%s challenge_max_attempts=%d recovery_ip_limit=%d/%s durable_schema=%d recovery_reload=%s generation=%d source_attribution=%s proxy_auth=%s edge_policy_generation=%d", runtime.recoveryProvider.Method(), runtime.recoveryProvider.Revision(), *sessionRecoveryChallengeTTL, *sessionRecoveryChallengeMaxAttempts, runtime.recoveryGuard.maxAttempts, runtime.recoveryGuard.window, sessionDurableAccountSchemaVersion, recoveryReloadMode, recoveryGeneration, sourceAttribution, proxyAuth, edgePolicy.Generation)
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
			_ = runtime.Close()
			return
		case err := <-serviceDone:
			stopTrustedCharacterAuthTimer(timer)
			if err != nil {
				logf("session login issuance stopped with error: %v", err)
			}
			return
		case <-runtime.changed:
			stopTrustedCharacterAuthTimer(timer)
			continue
		case <-reloadSignals:
			stopTrustedCharacterAuthTimer(timer)
			tlsResult, err := runtime.reloadTLSCertificate()
			if err != nil {
				currentTLS := runtime.TLSCertificateSnapshot()
				logf("session login TLS certificate reload rejected; last-known-good retained: generation=%d err=%v", currentTLS.Generation, err)
			} else {
				logf("session login TLS certificate reload applied: previous_generation=%d generation=%d not_after=%s", tlsResult.PreviousGeneration, tlsResult.Generation, tlsResult.NotAfter.UTC().Format(time.RFC3339Nano))
			}
			if runtime.sourceAttributor != nil && runtime.sourceAttributor.edgePolicy != nil {
				edgeResult, err := runtime.sourceAttributor.edgePolicy.Reload()
				if err != nil {
					currentEdge := runtime.sourceAttributor.edgePolicy.Snapshot()
					logf("session trusted proxy edge policy reload rejected; last-known-good retained: generation=%d revision=%s header=%s bindings=%d err=%v", currentEdge.Generation, currentEdge.Revision, currentEdge.HeaderMode, currentEdge.BindingCount, err)
				} else {
					retiredConnections := runtime.sourceAttributor.retireOldEdgeConnections(edgeResult.Generation)
					logf("session trusted proxy edge policy reload applied: previous_generation=%d generation=%d previous_revision=%s revision=%s previous_header=%s header=%s roots=%d bindings=%d prefixes=%d identities=%d connection_cutover=%s retired_connections=%d", edgeResult.PreviousGeneration, edgeResult.Generation, edgeResult.PreviousRevision, edgeResult.Revision, edgeResult.PreviousHeaderMode, edgeResult.HeaderMode, edgeResult.RootCount, edgeResult.BindingCount, edgeResult.PrefixCount, edgeResult.IdentityCount, runtime.sourceAttributor.edgeConnectionCutoverMode(), retiredConnections)
				}

				if runtime.sourceAttributor.edgePolicy.leafRevocation != nil {
					leafResult, leafErr := runtime.sourceAttributor.edgePolicy.ReloadLeafRevocation()
					if leafErr != nil {
						currentLeaf := runtime.sourceAttributor.edgePolicy.LeafRevocationSnapshot()
						logf("session trusted proxy leaf revocation reload rejected; last-known-good retained: generation=%d revision=%s revoked_credentials=%d err=%v", currentLeaf.Generation, currentLeaf.Revision, currentLeaf.RevokedCredentialCount, leafErr)
					} else {
						retiredConnections := runtime.sourceAttributor.retireOldEdgeConnections(runtime.sourceAttributor.edgePolicy.Snapshot().Generation)
						state := "no-op"
						if leafResult.AuthorityChanged {
							state = "applied"
						}
						logf("session trusted proxy leaf revocation reload %s: previous_generation=%d generation=%d previous_revision=%s revision=%s revoked_credentials=%d connection_cutover=%s retired_connections=%d", state, leafResult.PreviousGeneration, leafResult.Generation, leafResult.PreviousRevision, leafResult.Revision, leafResult.RevokedCredentialCount, runtime.sourceAttributor.edgeConnectionCutoverMode(), retiredConnections)
					}
				}
			} else if runtime.sourceAttributor != nil && runtime.sourceAttributor.proxyMTLS != nil {
				proxyResult, err := runtime.sourceAttributor.proxyMTLS.Reload()
				if err != nil {
					currentProxy := runtime.sourceAttributor.proxyMTLS.Snapshot()
					logf("session trusted proxy mTLS reload rejected; last-known-good retained: generation=%d revision=%s err=%v", currentProxy.Generation, currentProxy.Revision, err)
				} else {
					logf("session trusted proxy mTLS reload applied: previous_generation=%d generation=%d previous_revision=%s revision=%s roots=%d identities=%d", proxyResult.PreviousGeneration, proxyResult.Generation, proxyResult.PreviousRevision, proxyResult.Revision, proxyResult.RootCount, proxyResult.IdentityCount)
				}
			}
			if _, _, ok := runtime.accountAuth.reloadMetadata(); !ok {
				logf("session login account verifier is restart-only; SIGHUP does not reload schema v1/v2 issuance accounts")
			} else {
				accountResult, err := runtime.reloadDurableAccounts(time.Now().UTC())
				if err != nil {
					logf("session login durable account reload rejected; last-known-good retained: err=%v", err)
				} else {
					logf("session login durable account reload applied: previous_revision=%s revision=%s removed_bearers=%d retired_peers=%d", accountResult.PreviousRevision, accountResult.Revision, accountResult.RemovedBearers, accountResult.RetiredPeers)
				}
			}
			if runtime.recoveryProvider != nil {
				if _, ok := runtime.recoveryProvider.(*reloadableSessionRecoveryProvider); ok {
					recoveryResult, err := runtime.reloadRecoveryProvider()
					if err != nil {
						_, generation := sessionRecoveryReloadMetadata(runtime.recoveryProvider)
						logf("session recovery reload rejected; last-known-good retained: generation=%d revision=%s err=%v", generation, runtime.recoveryProvider.Revision(), err)
					} else {
						logf("session recovery reload applied: previous_generation=%d generation=%d previous_revision=%s revision=%s previous_provider=%s provider=%s retained_challenges=%d retired_challenges=%d", recoveryResult.PreviousGeneration, recoveryResult.Generation, recoveryResult.PreviousRevision, recoveryResult.Revision, recoveryResult.PreviousMethod, recoveryResult.Method, recoveryResult.RetainedChallenges, recoveryResult.RetiredChallenges)
					}
				} else {
					logf("session recovery provider is restart-only; SIGHUP does not reload schema-v1 recovery provider")
				}
			}
		case <-timerC:
			retired := runtime.expireAt(time.Now().UTC())
			if retired > 0 {
				logf("issued session credential expiry retired peers: retired_peers=%d", retired)
			}
		}
	}
}
