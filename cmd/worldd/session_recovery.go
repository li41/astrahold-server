package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/accountrecovery"
	"github.com/li41/astrahold-server/internal/accountstore"
)

const (
	sessionRecoveryPasswordMinBytes    = 12
	sessionRecoveryChallengeMinTTL     = time.Minute
	sessionRecoveryChallengeMaxTTL     = time.Hour
	sessionRecoveryDefaultChallengeTTL = 10 * time.Minute
	sessionRecoveryDefaultMaxAttempts  = 5
	sessionRecoveryPasswordSaltBytes   = 16
	sessionRecoveryPasswordDigestBytes = 32
)

var (
	sessionRecoveryProviderFile = flag.String(
		"session-recovery-provider-file",
		"",
		"Optional schema-v1 digest-only high-entropy recovery-code provider for the public recovery exchange; requires durable account schema v4",
	)
	sessionRecoveryChallengeTTL = flag.Duration(
		"session-recovery-challenge-ttl",
		sessionRecoveryDefaultChallengeTTL,
		"Lifetime of a public recovery challenge (1m..1h)",
	)
	sessionRecoveryChallengeMaxAttempts = flag.Int(
		"session-recovery-challenge-max-attempts",
		sessionRecoveryDefaultMaxAttempts,
		"Maximum proof attempts for one public recovery challenge (1..20)",
	)
	sessionRecoveryIPAttemptWindow = flag.Duration(
		"session-recovery-ip-attempt-window",
		time.Minute,
		"Fixed source-IP public recovery attempt window (1s..1h)",
	)
	sessionRecoveryIPMaxAttempts = flag.Int(
		"session-recovery-ip-max-attempts",
		10,
		"Maximum recovery request/reset POST attempts per observed TLS source IP in one window (1..10000)",
	)
)

var (
	errSessionRecoveryRejected    = errors.New("worldd: recovery rejected")
	errSessionRecoveryUnavailable = errors.New("worldd: recovery unavailable")
)

type sessionRecoveryChallengeRequest struct {
	LoginID string `json:"login_id"`
}

type sessionRecoveryChallengeResponse struct {
	RequestID string `json:"request_id"`
	ExpiresAt string `json:"expires_at"`
}

type sessionRecoveryResetRequest struct {
	RequestID     string `json:"request_id"`
	RecoveryProof string `json:"recovery_proof"`
	NewPassword   string `json:"new_password"`
}

type sessionRecoveryResetResult struct {
	Revision       string
	RemovedBearers int
	RetiredPeers   int
}

func loadSessionRecoveryProvider(accountAuth *sessionAccountAuthRuntime) (accountrecovery.Provider, *sessionLoginAbuseGuard, error) {
	path := strings.TrimSpace(*sessionRecoveryProviderFile)
	if path == "" {
		return nil, nil, nil
	}
	if *sessionRecoveryChallengeTTL < sessionRecoveryChallengeMinTTL || *sessionRecoveryChallengeTTL > sessionRecoveryChallengeMaxTTL {
		return nil, nil, fmt.Errorf("%w: session-recovery-challenge-ttl must be between %s and %s", errSessionLoginConfig, sessionRecoveryChallengeMinTTL, sessionRecoveryChallengeMaxTTL)
	}
	if *sessionRecoveryChallengeMaxAttempts < 1 || *sessionRecoveryChallengeMaxAttempts > 20 {
		return nil, nil, fmt.Errorf("%w: session-recovery-challenge-max-attempts must be between 1 and 20", errSessionLoginConfig)
	}
	if *sessionRecoveryIPAttemptWindow < time.Second || *sessionRecoveryIPAttemptWindow > time.Hour {
		return nil, nil, fmt.Errorf("%w: session-recovery-ip-attempt-window must be between 1s and 1h", errSessionLoginConfig)
	}
	if *sessionRecoveryIPMaxAttempts < 1 || *sessionRecoveryIPMaxAttempts > 10000 {
		return nil, nil, fmt.Errorf("%w: session-recovery-ip-max-attempts must be between 1 and 10000", errSessionLoginConfig)
	}
	if accountAuth == nil {
		return nil, nil, fmt.Errorf("%w: recovery requires durable account schema v4", errSessionLoginConfig)
	}
	authenticator, ok := accountAuth.snapshot().(*durableSessionLoginAuthenticator)
	if !ok || !authenticator.RecoveryEnabled() {
		return nil, nil, fmt.Errorf("%w: recovery requires durable account schema_version %d", errSessionLoginConfig, sessionDurableAccountSchemaVersion)
	}
	provider, err := loadStaticSessionRecoveryProvider(
		path,
		*sessionRecoveryChallengeTTL,
		*sessionRecoveryChallengeMaxAttempts,
		time.Now,
		rand.Reader,
	)
	if err != nil {
		return nil, nil, err
	}
	guard := newSessionLoginAbuseGuard(*sessionRecoveryIPAttemptWindow, *sessionRecoveryIPMaxAttempts, time.Now)
	return provider, guard, nil
}

func (r *sessionLoginRuntime) recoverySubject(loginID string) (accountrecovery.Subject, bool) {
	if r == nil || r.accountAuth == nil {
		return accountrecovery.Subject{}, false
	}
	authenticator, ok := r.accountAuth.snapshot().(*durableSessionLoginAuthenticator)
	if !ok || !authenticator.RecoveryEnabled() {
		return accountrecovery.Subject{}, false
	}
	return authenticator.RecoverySubject(loginID), true
}

func (r *sessionLoginRuntime) recoveryPasswordPolicy() (argon2idPasswordHash, bool) {
	if r == nil || r.accountAuth == nil {
		return argon2idPasswordHash{}, false
	}
	authenticator, ok := r.accountAuth.snapshot().(*durableSessionLoginAuthenticator)
	if !ok || !authenticator.RecoveryEnabled() {
		return argon2idPasswordHash{}, false
	}
	return authenticator.RecoveryPasswordPolicy()
}

func (r *sessionLoginRuntime) allowRecoveryAttempt(w http.ResponseWriter, request *http.Request) bool {
	if r == nil || r.recoveryGuard == nil {
		return true
	}
	allowed, retry := r.recoveryGuard.Allow(request.RemoteAddr)
	if allowed {
		return true
	}
	seconds := int((retry + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeSessionLoginError(w, http.StatusTooManyRequests, "recovery_throttled")
	return false
}

func (r *sessionLoginRuntime) handleRecoveryRequest(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeSessionLoginError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !r.allowRecoveryAttempt(w, request) {
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, sessionLoginMaxRequestBytes)
	decoder := jsonDecoderDisallowUnknown(request.Body)
	var input sessionRecoveryChallengeRequest
	if err := decoder.Decode(&input); err != nil || decodeTrailingJSON(decoder) != nil {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if input.LoginID == "" || input.LoginID != strings.TrimSpace(input.LoginID) || len(input.LoginID) > accountrecovery.MaxLoginIDBytes {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	subject, ok := r.recoverySubject(input.LoginID)
	if !ok || r.recoveryProvider == nil {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	challenge, err := r.recoveryProvider.Begin(request.Context(), subject)
	if err != nil {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	writeSessionLoginJSON(w, http.StatusAccepted, sessionRecoveryChallengeResponse{
		RequestID: challenge.RequestID,
		ExpiresAt: challenge.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
}

func (r *sessionLoginRuntime) handleRecoveryReset(w http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeSessionLoginError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !r.allowRecoveryAttempt(w, request) {
		return
	}
	request.Body = http.MaxBytesReader(w, request.Body, sessionLoginMaxRequestBytes)
	decoder := jsonDecoderDisallowUnknown(request.Body)
	var input sessionRecoveryResetRequest
	if err := decoder.Decode(&input); err != nil || decodeTrailingJSON(decoder) != nil {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if input.RequestID == "" || input.RequestID != strings.TrimSpace(input.RequestID) || len(input.RequestID) > accountrecovery.MaxRequestIDBytes {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	proof := []byte(input.RecoveryProof)
	defer clear(proof)
	password := []byte(input.NewPassword)
	defer clear(password)
	if len(proof) == 0 || len(proof) > accountrecovery.MaxProofBytes || !validSessionRecoveryPassword(password) {
		writeSessionLoginError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if r.recoveryProvider == nil {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	grant, err := r.recoveryProvider.Verify(request.Context(), input.RequestID, proof)
	if err != nil {
		if errors.Is(err, accountrecovery.ErrRejected) {
			writeSessionLoginError(w, http.StatusUnauthorized, "invalid_recovery")
			return
		}
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	policy, ok := r.recoveryPasswordPolicy()
	if !ok {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	phc, err := hashSessionRecoveryPassword(password, policy, rand.Reader)
	if err != nil {
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	if _, err := r.resetDurableAccountFromRecovery(grant, phc, time.Now().UTC()); err != nil {
		if errors.Is(err, errSessionRecoveryRejected) {
			writeSessionLoginError(w, http.StatusUnauthorized, "invalid_recovery")
			return
		}
		writeSessionLoginError(w, http.StatusServiceUnavailable, "recovery_unavailable")
		return
	}
	r.recoveryProvider.Consume(request.Context(), input.RequestID)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(http.StatusNoContent)
}

func (r *sessionLoginRuntime) resetDurableAccountFromRecovery(grant accountrecovery.Grant, passwordPHC string, now time.Time) (sessionRecoveryResetResult, error) {
	if r == nil || r.accountAuth == nil || r.provider == nil || r.replaceScopes == nil || strings.TrimSpace(r.accountPath) == "" || !grant.Valid() || strings.TrimSpace(passwordPHC) == "" {
		return sessionRecoveryResetResult{}, errSessionRecoveryUnavailable
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	_, currentStoreRevision, ok := r.accountAuth.reloadMetadata()
	currentAuthenticator, durable := r.accountAuth.snapshot().(*durableSessionLoginAuthenticator)
	if !ok || !durable || !currentAuthenticator.RecoveryEnabled() {
		return sessionRecoveryResetResult{}, errSessionRecoveryUnavailable
	}
	definition, err := accountstore.Load(r.accountPath)
	if err != nil {
		return sessionRecoveryResetResult{}, fmt.Errorf("%w: %v", errSessionRecoveryUnavailable, err)
	}
	if definition.SchemaVersion != accountstore.SchemaVersion || definition.Revision != currentStoreRevision {
		return sessionRecoveryResetResult{}, errSessionRecoveryUnavailable
	}
	accountIndex := -1
	for index := range definition.Accounts {
		account := definition.Accounts[index]
		if account.AccountID == grant.AccountID {
			accountIndex = index
			break
		}
	}
	if accountIndex < 0 {
		return sessionRecoveryResetResult{}, errSessionRecoveryRejected
	}
	account := &definition.Accounts[accountIndex]
	if account.DisabledAt != "" || account.CredentialVersion != grant.CredentialVersion || account.CredentialVersion == ^uint64(0) {
		return sessionRecoveryResetResult{}, errSessionRecoveryRejected
	}
	account.CredentialVersion++
	account.PasswordArgon2ID = passwordPHC
	account.PasswordChangedAt = now.Format(time.RFC3339Nano)
	filtered := definition.RecoveryGrants[:0]
	for _, candidate := range definition.RecoveryGrants {
		if candidate.AccountID != account.AccountID {
			filtered = append(filtered, candidate)
		}
	}
	definition.RecoveryGrants = filtered
	previousStoreRevision := definition.Revision
	definition.Revision++
	nextAuthenticator, err := newDurableSessionLoginAuthenticator(definition)
	if err != nil {
		return sessionRecoveryResetResult{}, fmt.Errorf("%w: next account snapshot: %v", errSessionRecoveryUnavailable, err)
	}
	if err := accountstore.SaveIfRevision(r.accountPath, previousStoreRevision, definition); err != nil {
		return sessionRecoveryResetResult{}, fmt.Errorf("%w: durable account CAS: %v", errSessionRecoveryUnavailable, err)
	}

	currentProvider := r.provider.snapshot()
	nextRevision := r.revision + 1
	nextProvider := cloneIssuedSessionCredentialProvider(currentProvider, nextRevision, r.now)
	pruneIssuedSessionCredentials(nextProvider, now)
	removed := 0
	for digest, entry := range nextProvider.credentials {
		subject := entry.grant.AuthenticationSubject
		generation := entry.grant.AuthenticationGeneration
		if subject == "" || generation == "" || nextAuthenticator.GenerationActive(subject, generation) {
			continue
		}
		delete(nextProvider.credentials, digest)
		if entry.credentialID != "" {
			delete(nextProvider.credentialsByID, entry.credentialID)
		}
		removed++
	}
	retired := r.replaceScopes(activeTrustedCharacterAuthenticationScopes(nextProvider, now))
	r.provider.replace(nextProvider)
	r.revision = nextRevision
	r.accountAuth.replace(nextAuthenticator)
	r.signalChanged()
	return sessionRecoveryResetResult{
		Revision:       nextAuthenticator.Revision(),
		RemovedBearers: removed,
		RetiredPeers:   retired,
	}, nil
}

func validSessionRecoveryPassword(password []byte) bool {
	return len(password) >= sessionRecoveryPasswordMinBytes && len(password) <= sessionLoginMaxSecretBytes && !strings.ContainsAny(string(password), "\r\n")
}

func hashSessionRecoveryPassword(password []byte, policy argon2idPasswordHash, random io.Reader) (string, error) {
	if !validSessionRecoveryPassword(password) || random == nil || policy.memory == 0 || policy.time == 0 || policy.threads == 0 || len(policy.digest) != sessionRecoveryPasswordDigestBytes {
		return "", errSessionRecoveryUnavailable
	}
	var salt [sessionRecoveryPasswordSaltBytes]byte
	if _, err := io.ReadFull(random, salt[:]); err != nil {
		return "", fmt.Errorf("%w: password salt: %v", errSessionRecoveryUnavailable, err)
	}
	digest := argon2.IDKey(password, salt[:], policy.time, policy.memory, policy.threads, sessionRecoveryPasswordDigestBytes)
	defer clear(digest)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		sessionPasswordArgon2Version,
		policy.memory,
		policy.time,
		policy.threads,
		base64.RawStdEncoding.EncodeToString(salt[:]),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// Small wrappers keep recovery request parsing identical to login parsing while
// allowing the handlers to reject unknown fields and trailing JSON uniformly.
func jsonDecoderDisallowUnknown(reader io.Reader) *json.Decoder {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	return decoder
}

func decodeTrailingJSON(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}
