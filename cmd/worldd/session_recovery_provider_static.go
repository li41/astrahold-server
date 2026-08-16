package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
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
	sessionRecoveryProviderSchemaVersion uint16 = 1
	sessionRecoveryRequestRandomBytes           = 24
	sessionRecoveryMaxActiveChallenges          = 4096
)

type sessionRecoveryProviderDefinition struct {
	SchemaVersion uint16                           `json:"schema_version"`
	Revision      string                           `json:"revision"`
	Subjects      []sessionRecoveryProviderSubject `json:"subjects"`
}

type sessionRecoveryProviderSubject struct {
	LoginID            string `json:"login_id"`
	RecoveryCodeSHA256 string `json:"recovery_code_sha256"`
}

type staticSessionRecoveryChallenge struct {
	subject  accountrecovery.Subject
	verifier [sha256.Size]byte
	active   bool
	expires  time.Time
	attempts int
}

type staticSessionRecoveryProvider struct {
	revision    string
	subjects    map[string][sha256.Size]byte
	dummy       [sha256.Size]byte
	ttl         time.Duration
	maxAttempts int
	maxActive   int
	now         func() time.Time
	random      io.Reader

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
	return newStaticSessionRecoveryProvider(definition, ttl, maxAttempts, now, random)
}

func newStaticSessionRecoveryProvider(
	definition sessionRecoveryProviderDefinition,
	ttl time.Duration,
	maxAttempts int,
	now func() time.Time,
	random io.Reader,
) (*staticSessionRecoveryProvider, error) {
	if definition.SchemaVersion != sessionRecoveryProviderSchemaVersion || strings.TrimSpace(definition.Revision) == "" || definition.Revision != strings.TrimSpace(definition.Revision) || len(definition.Subjects) == 0 {
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

func (p *staticSessionRecoveryProvider) Method() string {
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

	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now().UTC()
	p.pruneLocked(now)
	if len(p.challenges) >= p.maxActive {
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}

	var requestID string
	for attempt := 0; attempt < 4; attempt++ {
		var entropy [sessionRecoveryRequestRandomBytes]byte
		if _, err := io.ReadFull(p.random, entropy[:]); err != nil {
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
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	verifier, found := p.subjects[subject.LoginID]
	if !found {
		verifier = p.dummy
	}
	expires := now.Add(p.ttl)
	p.challenges[requestID] = staticSessionRecoveryChallenge{
		subject:  subject,
		verifier: verifier,
		active:   subject.Eligible && found,
		expires:  expires,
	}
	challenge := accountrecovery.Challenge{RequestID: requestID, ExpiresAt: expires}
	if !challenge.Valid() {
		delete(p.challenges, requestID)
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	return challenge, nil
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
