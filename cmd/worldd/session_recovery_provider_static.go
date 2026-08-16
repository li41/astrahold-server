package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

const (
	sessionRecoveryProviderSchemaVersion          uint16 = 1
	sessionRecoveryDeliveredProviderSchemaVersion uint16 = 2
	sessionRecoveryRequestRandomBytes                    = 24
	sessionRecoveryDeliveryProofKeyBytes                 = 32
	sessionRecoveryMaxActiveChallenges                   = 4096
	sessionRecoveryDeliveryProofDomain                   = "astrahold-recovery-delivery-v1"
)

type sessionRecoveryProviderDefinition struct {
	SchemaVersion     uint16                             `json:"schema_version"`
	Revision          string                             `json:"revision"`
	ProofKeyBase64URL string                             `json:"proof_key_base64url,omitempty"`
	Delivery          *sessionRecoveryDeliveryDefinition `json:"delivery,omitempty"`
	Subjects          []sessionRecoveryProviderSubject   `json:"subjects"`
}

type sessionRecoveryDeliveryDefinition struct {
	Adapter        string `json:"adapter"`
	Revision       string `json:"revision"`
	InboxDir       string `json:"inbox_dir,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`
	CredentialFile string `json:"credential_file,omitempty"`
	CAFile         string `json:"ca_file,omitempty"`
	RequestTimeout string `json:"request_timeout,omitempty"`
	MaxAttempts    int    `json:"max_attempts,omitempty"`
	RetryBackoff   string `json:"retry_backoff,omitempty"`
}

type sessionRecoveryProviderSubject struct {
	LoginID            string `json:"login_id"`
	RecoveryCodeSHA256 string `json:"recovery_code_sha256,omitempty"`
	Destination        string `json:"destination,omitempty"`
}

type staticSessionRecoveryChallenge struct {
	subject  accountrecovery.Subject
	verifier [sha256.Size]byte
	active   bool
	expires  time.Time
	attempts int
}

type staticSessionRecoveryProvider struct {
	revision     string
	subjects     map[string][sha256.Size]byte
	destinations map[string]string
	dummy        [sha256.Size]byte
	proofKey     [sessionRecoveryDeliveryProofKeyBytes]byte
	delivery     accountrecovery.DeliveryAdapter
	ttl          time.Duration
	maxAttempts  int
	maxActive    int
	now          func() time.Time
	random       io.Reader

	mu         sync.Mutex
	challenges map[string]staticSessionRecoveryChallenge
}

func loadStaticSessionRecoveryProvider(
	path string,
	ttl time.Duration,
	maxAttempts int,
	now func() time.Time,
	random io.Reader,
) (*staticSessionRecoveryProvider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read recovery provider config %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition sessionRecoveryProviderDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode recovery provider: %v", errSessionLoginConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: recovery provider trailing JSON value", errSessionLoginConfig)
		}
		return nil, fmt.Errorf("%w: recovery provider trailing data: %v", errSessionLoginConfig, err)
	}

	switch definition.SchemaVersion {
	case sessionRecoveryProviderSchemaVersion:
		return newStaticSessionRecoveryProvider(definition, ttl, maxAttempts, now, random)
	case sessionRecoveryDeliveredProviderSchemaVersion:
		if definition.Delivery == nil {
			return nil, fmt.Errorf("%w: schema-v2 recovery delivery config is required", errSessionLoginConfig)
		}
		adapter, err := buildSessionRecoveryDeliveryAdapter(*definition.Delivery)
		if err != nil {
			return nil, err
		}
		if definition.Delivery.Adapter == sessionRecoveryFilesystemAdapterMethod {
			for index, item := range definition.Subjects {
				if !validFilesystemRecoveryDestination(item.Destination) {
					return nil, fmt.Errorf("%w: schema-v2 recovery subject[%d] invalid filesystem destination", errSessionLoginConfig, index)
				}
			}
		}
		return newDeliveredSessionRecoveryProvider(definition, adapter, ttl, maxAttempts, now, random)
	default:
		return nil, fmt.Errorf("%w: unsupported recovery provider schema_version %d", errSessionLoginConfig, definition.SchemaVersion)
	}
}

func buildSessionRecoveryDeliveryAdapter(definition sessionRecoveryDeliveryDefinition) (accountrecovery.DeliveryAdapter, error) {
	switch definition.Adapter {
	case sessionRecoveryFilesystemAdapterMethod:
		if definition.Endpoint != "" || definition.CredentialFile != "" || definition.CAFile != "" || definition.RequestTimeout != "" || definition.MaxAttempts != 0 || definition.RetryBackoff != "" {
			return nil, fmt.Errorf("%w: filesystem recovery delivery cannot set https adapter fields", errSessionLoginConfig)
		}
		return newFilesystemRecoveryDeliveryAdapter(definition.InboxDir, definition.Revision)
	case sessionRecoveryHTTPAdapterMethod:
		if definition.InboxDir != "" {
			return nil, fmt.Errorf("%w: https recovery delivery cannot set inbox_dir", errSessionLoginConfig)
		}
		requestTimeout, err := parseRecoveryDeliveryDuration(definition.RequestTimeout, sessionRecoveryHTTPDefaultTimeout, "request_timeout")
		if err != nil {
			return nil, err
		}
		retryBackoff, err := parseRecoveryDeliveryDuration(definition.RetryBackoff, sessionRecoveryHTTPDefaultRetryBackoff, "retry_backoff")
		if err != nil {
			return nil, err
		}
		return newHTTPRecoveryDeliveryAdapter(definition.Endpoint, definition.CredentialFile, definition.CAFile, definition.Revision, requestTimeout, definition.MaxAttempts, retryBackoff)
	default:
		return nil, fmt.Errorf("%w: unsupported schema-v2 recovery delivery adapter %q", errSessionLoginConfig, definition.Adapter)
	}
}

func parseRecoveryDeliveryDuration(value string, fallback time.Duration, field string) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%w: https recovery delivery %s: %v", errSessionLoginConfig, field, err)
	}
	return parsed, nil
}

func newStaticSessionRecoveryProvider(
	definition sessionRecoveryProviderDefinition,
	ttl time.Duration,
	maxAttempts int,
	now func() time.Time,
	random io.Reader,
) (*staticSessionRecoveryProvider, error) {
	if definition.SchemaVersion != sessionRecoveryProviderSchemaVersion || strings.TrimSpace(definition.Revision) == "" || definition.Revision != strings.TrimSpace(definition.Revision) || len(definition.Subjects) == 0 || definition.ProofKeyBase64URL != "" || definition.Delivery != nil {
		return nil, errSessionLoginConfig
	}
	if ttl < time.Minute || ttl > time.Hour || maxAttempts < 1 || maxAttempts > 20 || random == nil {
		return nil, errSessionLoginConfig
	}
	if now == nil {
		now = time.Now
	}
	subjects := make(map[string][sha256.Size]byte, len(definition.Subjects))
	var dummy [sha256.Size]byte
	for index, item := range definition.Subjects {
		loginID := strings.TrimSpace(item.LoginID)
		if loginID == "" || loginID != item.LoginID || len(loginID) > accountrecovery.MaxLoginIDBytes {
			return nil, fmt.Errorf("%w: recovery subject[%d] login_id must be 1..%d trimmed bytes", errSessionLoginConfig, index, accountrecovery.MaxLoginIDBytes)
		}
		if item.Destination != "" {
			return nil, fmt.Errorf("%w: schema-v1 recovery subject[%d] cannot set destination", errSessionLoginConfig, index)
		}
		if _, exists := subjects[loginID]; exists {
			return nil, fmt.Errorf("%w: duplicate recovery login_id %q", errSessionLoginConfig, loginID)
		}
		if len(item.RecoveryCodeSHA256) != sha256.Size*2 || strings.ToLower(item.RecoveryCodeSHA256) != item.RecoveryCodeSHA256 {
			return nil, fmt.Errorf("%w: recovery subject[%d] recovery_code_sha256 must be 64 lowercase hex characters", errSessionLoginConfig, index)
		}
		decoded, err := hex.DecodeString(item.RecoveryCodeSHA256)
		if err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("%w: recovery subject[%d] recovery_code_sha256", errSessionLoginConfig, index)
		}
		var digest [sha256.Size]byte
		copy(digest[:], decoded)
		if index == 0 {
			dummy = digest
		}
		subjects[loginID] = digest
	}
	return &staticSessionRecoveryProvider{
		revision:    definition.Revision,
		subjects:    subjects,
		dummy:       dummy,
		ttl:         ttl,
		maxAttempts: maxAttempts,
		maxActive:   sessionRecoveryMaxActiveChallenges,
		now:         now,
		random:      random,
		challenges:  make(map[string]staticSessionRecoveryChallenge),
	}, nil
}

func newDeliveredSessionRecoveryProvider(
	definition sessionRecoveryProviderDefinition,
	delivery accountrecovery.DeliveryAdapter,
	ttl time.Duration,
	maxAttempts int,
	now func() time.Time,
	random io.Reader,
) (*staticSessionRecoveryProvider, error) {
	if definition.SchemaVersion != sessionRecoveryDeliveredProviderSchemaVersion || strings.TrimSpace(definition.Revision) == "" || definition.Revision != strings.TrimSpace(definition.Revision) || len(definition.Subjects) == 0 || delivery == nil || strings.TrimSpace(delivery.Method()) == "" || strings.TrimSpace(delivery.Revision()) == "" {
		return nil, errSessionLoginConfig
	}
	if ttl < time.Minute || ttl > time.Hour || maxAttempts < 1 || maxAttempts > 20 || random == nil {
		return nil, errSessionLoginConfig
	}
	if now == nil {
		now = time.Now
	}
	decodedKey, err := base64.RawURLEncoding.DecodeString(definition.ProofKeyBase64URL)
	if err != nil || len(decodedKey) != sessionRecoveryDeliveryProofKeyBytes {
		clear(decodedKey)
		return nil, fmt.Errorf("%w: schema-v2 proof_key_base64url must decode to exactly %d bytes", errSessionLoginConfig, sessionRecoveryDeliveryProofKeyBytes)
	}
	defer clear(decodedKey)

	destinations := make(map[string]string, len(definition.Subjects))
	ownedDestinations := make(map[string]string, len(definition.Subjects))
	for index, item := range definition.Subjects {
		loginID := strings.TrimSpace(item.LoginID)
		if loginID == "" || loginID != item.LoginID || len(loginID) > accountrecovery.MaxLoginIDBytes {
			return nil, fmt.Errorf("%w: recovery subject[%d] login_id must be 1..%d trimmed bytes", errSessionLoginConfig, index, accountrecovery.MaxLoginIDBytes)
		}
		if item.RecoveryCodeSHA256 != "" {
			return nil, fmt.Errorf("%w: schema-v2 recovery subject[%d] cannot set recovery_code_sha256", errSessionLoginConfig, index)
		}
		destination := strings.TrimSpace(item.Destination)
		if destination == "" || destination != item.Destination || len(destination) > accountrecovery.MaxDeliveryDestinationBytes {
			return nil, fmt.Errorf("%w: schema-v2 recovery subject[%d] destination must be 1..%d trimmed bytes", errSessionLoginConfig, index, accountrecovery.MaxDeliveryDestinationBytes)
		}
		if _, exists := destinations[loginID]; exists {
			return nil, fmt.Errorf("%w: duplicate recovery login_id %q", errSessionLoginConfig, loginID)
		}
		if owner, exists := ownedDestinations[destination]; exists {
			return nil, fmt.Errorf("%w: recovery destination %q is already owned by login_id %q", errSessionLoginConfig, destination, owner)
		}
		destinations[loginID] = destination
		ownedDestinations[destination] = loginID
	}

	provider := &staticSessionRecoveryProvider{
		revision:     definition.Revision,
		destinations: destinations,
		delivery:     delivery,
		ttl:          ttl,
		maxAttempts:  maxAttempts,
		maxActive:    sessionRecoveryMaxActiveChallenges,
		now:          now,
		random:       random,
		challenges:   make(map[string]staticSessionRecoveryChallenge),
	}
	copy(provider.proofKey[:], decodedKey)
	dummyMAC := hmac.New(sha256.New, provider.proofKey[:])
	_, _ = dummyMAC.Write([]byte("astrahold-recovery-delivery-dummy-v1"))
	dummyProof := []byte(base64.RawURLEncoding.EncodeToString(dummyMAC.Sum(nil)))
	provider.dummy = sha256.Sum256(dummyProof)
	clear(dummyProof)
	return provider, nil
}

func (p *staticSessionRecoveryProvider) Method() string {
	if p != nil && p.delivery != nil {
		return "hmac-sha256-generation-delivery"
	}
	return "sha256-high-entropy-recovery-code"
}

func (p *staticSessionRecoveryProvider) Revision() string {
	if p == nil {
		return ""
	}
	return p.revision
}

func (p *staticSessionRecoveryProvider) Begin(ctx context.Context, subject accountrecovery.Subject) (accountrecovery.Challenge, error) {
	if p == nil || !subject.Valid() || p.random == nil || p.now == nil {
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return accountrecovery.Challenge{}, ctx.Err()
	default:
	}

	verifier := p.dummy
	found := false
	destination := ""
	var rawProof []byte
	if p.delivery != nil {
		destination, found = p.destinations[subject.LoginID]
		rawProof = p.deliveredProof(subject)
		verifier = sha256.Sum256(rawProof)
	} else {
		verifier, found = p.subjects[subject.LoginID]
		if !found {
			verifier = p.dummy
		}
	}
	defer clear(rawProof)

	p.mu.Lock()
	now := p.now().UTC()
	p.pruneLocked(now)
	if len(p.challenges) >= p.maxActive {
		p.mu.Unlock()
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}

	var requestID string
	for attempt := 0; attempt < 4; attempt++ {
		var entropy [sessionRecoveryRequestRandomBytes]byte
		if _, err := io.ReadFull(p.random, entropy[:]); err != nil {
			p.mu.Unlock()
			return accountrecovery.Challenge{}, fmt.Errorf("%w: recovery request entropy: %v", accountrecovery.ErrUnavailable, err)
		}
		requestID = base64.RawURLEncoding.EncodeToString(entropy[:])
		clear(entropy[:])
		if _, collision := p.challenges[requestID]; !collision {
			break
		}
		requestID = ""
	}
	if requestID == "" {
		p.mu.Unlock()
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	expires := now.Add(p.ttl)
	active := subject.Eligible && found && p.delivery == nil
	p.challenges[requestID] = staticSessionRecoveryChallenge{
		subject:  subject,
		verifier: verifier,
		active:   active,
		expires:  expires,
	}
	p.mu.Unlock()

	challenge := accountrecovery.Challenge{RequestID: requestID, ExpiresAt: expires}
	if !challenge.Valid() {
		p.Consume(ctx, requestID)
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	if p.delivery == nil || !subject.Eligible || !found {
		return challenge, nil
	}

	deliveryErr := p.delivery.Deliver(ctx, accountrecovery.Delivery{
		RequestID:   requestID,
		Destination: destination,
		Proof:       rawProof,
		ExpiresAt:   expires,
	})
	if deliveryErr != nil {
		if ctx.Err() != nil {
			p.Consume(context.Background(), requestID)
			return accountrecovery.Challenge{}, ctx.Err()
		}
		// Per-subject transport failure is intentionally mapped to the same public
		// accepted challenge as unknown/ineligible subjects. The reserved state
		// remains non-authorizing, so no delivered proof can be redeemed.
		return challenge, nil
	}

	p.mu.Lock()
	state, exists := p.challenges[requestID]
	if exists && p.now().UTC().Before(state.expires) {
		state.active = true
		p.challenges[requestID] = state
	}
	p.mu.Unlock()
	return challenge, nil
}

func (p *staticSessionRecoveryProvider) deliveredProof(subject accountrecovery.Subject) []byte {
	mac := hmac.New(sha256.New, p.proofKey[:])
	_, _ = mac.Write([]byte(sessionRecoveryDeliveryProofDomain))
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(len(subject.LoginID)))
	_, _ = mac.Write(length[:])
	_, _ = mac.Write([]byte(subject.LoginID))
	binary.BigEndian.PutUint32(length[:], uint32(len(subject.AccountID)))
	_, _ = mac.Write(length[:])
	_, _ = mac.Write([]byte(subject.AccountID))
	var generation [8]byte
	binary.BigEndian.PutUint64(generation[:], subject.CredentialVersion)
	_, _ = mac.Write(generation[:])
	return []byte(base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
}

func (p *staticSessionRecoveryProvider) Verify(ctx context.Context, requestID string, proof []byte) (accountrecovery.Grant, error) {
	if p == nil || requestID == "" || len(requestID) > accountrecovery.MaxRequestIDBytes || len(proof) == 0 || len(proof) > accountrecovery.MaxProofBytes {
		return accountrecovery.Grant{}, accountrecovery.ErrRejected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return accountrecovery.Grant{}, ctx.Err()
	default:
	}
	digest := sha256.Sum256(proof)

	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now().UTC()
	state, exists := p.challenges[requestID]
	verifier := p.dummy
	if exists {
		verifier = state.verifier
	}
	matched := subtle.ConstantTimeCompare(digest[:], verifier[:]) == 1
	active := exists && now.Before(state.expires) && state.attempts < p.maxAttempts
	if active {
		state.attempts++
		p.challenges[requestID] = state
	}
	if !active || !state.active || !matched {
		return accountrecovery.Grant{}, accountrecovery.ErrRejected
	}
	grant := accountrecovery.Grant{
		AccountID:         state.subject.AccountID,
		CredentialVersion: state.subject.CredentialVersion,
	}
	if !grant.Valid() {
		return accountrecovery.Grant{}, accountrecovery.ErrRejected
	}
	return grant, nil
}

func (p *staticSessionRecoveryProvider) Consume(_ context.Context, requestID string) {
	if p == nil || requestID == "" {
		return
	}
	p.mu.Lock()
	delete(p.challenges, requestID)
	p.mu.Unlock()
}

func (p *staticSessionRecoveryProvider) pruneLocked(now time.Time) {
	for requestID, challenge := range p.challenges {
		if !now.Before(challenge.expires) {
			delete(p.challenges, requestID)
		}
	}
}

var _ accountrecovery.Provider = (*staticSessionRecoveryProvider)(nil)
