package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func (p *staticSessionRecoveryProvider) sessionRecoveryOutboxSnapshot(requestID string) (sessionRecoveryOutboxChallengeSnapshot, bool) {
	if p == nil || requestID == "" {
		return sessionRecoveryOutboxChallengeSnapshot{}, false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	state, exists := p.challenges[requestID]
	if !exists || !state.subject.Valid() {
		return sessionRecoveryOutboxChallengeSnapshot{}, false
	}
	return sessionRecoveryOutboxChallengeSnapshot{
		RequestID:            requestID,
		LoginID:              state.subject.LoginID,
		AccountID:            state.subject.AccountID,
		CredentialVersion:    state.subject.CredentialVersion,
		VerifierSHA256:       hex.EncodeToString(state.verifier[:]),
		Active:               state.active,
		ExpiresAt:            state.expires.UTC(),
		VerificationAttempts: state.attempts,
	}, true
}

func (p *staticSessionRecoveryProvider) restoreSessionRecoveryOutboxRecord(record sessionRecoveryOutboxRecord) error {
	if p == nil {
		return accountrecovery.ErrUnavailable
	}
	verifierBytes, err := hex.DecodeString(record.VerifierSHA256)
	if err != nil || len(verifierBytes) != sha256.Size {
		return fmt.Errorf("%w: invalid durable recovery verifier", errSessionLoginConfig)
	}
	var verifier [sha256.Size]byte
	copy(verifier[:], verifierBytes)
	clear(verifierBytes)
	expiresAt, err := parseSessionRecoveryOutboxTime(record.ExpiresAt)
	if err != nil {
		return err
	}
	subject := accountrecovery.Subject{
		LoginID:           record.LoginID,
		AccountID:         record.AccountID,
		CredentialVersion: record.CredentialVersion,
		Eligible:          true,
	}
	if !subject.Valid() {
		return fmt.Errorf("%w: invalid durable recovery subject", errSessionLoginConfig)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneLocked(p.now().UTC())
	if _, exists := p.challenges[record.RequestID]; exists {
		return fmt.Errorf("%w: duplicate durable recovery request_id", errSessionLoginConfig)
	}
	if len(p.challenges) >= p.maxActive {
		return fmt.Errorf("%w: durable recovery challenges exceed active bound", errSessionLoginConfig)
	}
	active := record.Active && record.VerificationAttempts < p.maxAttempts
	p.challenges[record.RequestID] = staticSessionRecoveryChallenge{
		subject:  subject,
		verifier: verifier,
		active:   active,
		expires:  expiresAt,
		attempts: record.VerificationAttempts,
	}
	return nil
}

func (p *staticSessionRecoveryProvider) deactivateSessionRecoveryChallenge(requestID string) {
	if p == nil || requestID == "" {
		return
	}
	p.mu.Lock()
	state, exists := p.challenges[requestID]
	if exists {
		state.active = false
		p.challenges[requestID] = state
	}
	p.mu.Unlock()
}

func durableSessionRecoveryOutbox(provider *staticSessionRecoveryProvider) *sessionRecoveryDurableOutbox {
	if provider == nil {
		return nil
	}
	outbox, _ := provider.delivery.(*sessionRecoveryDurableOutbox)
	return outbox
}

func publishSessionRecoveryDurableChallenge(provider *staticSessionRecoveryProvider, requestID string) {
	outbox := durableSessionRecoveryOutbox(provider)
	if outbox == nil {
		return
	}
	outbox.PublishChallenge(requestID)
}

func persistSessionRecoveryDurableChallenge(provider *staticSessionRecoveryProvider, requestID string) error {
	outbox := durableSessionRecoveryOutbox(provider)
	if outbox == nil {
		return nil
	}
	if err := outbox.PersistChallenge(provider, requestID); err != nil {
		// A verification-attempt mutation that cannot be made durable must not
		// authorize a reset. Prefer removing the durable challenge entirely; if
		// that cleanup also fails, at least retire the in-memory copy for this
		// process so the request remains fail-closed.
		if deleteErr := outbox.DeleteChallenge(requestID); deleteErr == nil {
			provider.Consume(nil, requestID)
		} else {
			provider.deactivateSessionRecoveryChallenge(requestID)
		}
		return err
	}
	return nil
}

func deleteSessionRecoveryDurableChallenge(provider *staticSessionRecoveryProvider, requestID string) {
	outbox := durableSessionRecoveryOutbox(provider)
	if outbox == nil {
		return
	}
	_ = outbox.DeleteChallenge(requestID)
}

var _ accountrecovery.DeliveryAdapter = (*sessionRecoveryDurableOutbox)(nil)
var _ sessionRecoverySharedDelivery = (*sessionRecoveryDurableOutbox)(nil)
