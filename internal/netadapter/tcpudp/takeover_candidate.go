package tcpudp

import (
	"errors"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

const (
	defaultCharacterTakeoverCandidateTTL = 10 * time.Second
	defaultCharacterTakeoverCooldown     = 2 * time.Second
)

var (
	ErrInvalidCharacterTakeoverCandidate  = errors.New("tcpudp: invalid trusted character takeover candidate lease")
	ErrCharacterTakeoverCandidateReserved = errors.New("tcpudp: trusted character already has an active takeover candidate")
	ErrCharacterTakeoverCandidateExpired  = errors.New("tcpudp: trusted character takeover candidate lease expired")
	ErrCharacterTakeoverCandidateStale    = errors.New("tcpudp: trusted character takeover candidate lease is stale")
	ErrCharacterTakeoverCoolingDown       = errors.New("tcpudp: trusted character active takeover is cooling down")
)

type characterTakeoverCandidateLease struct {
	CharacterID        characteridentity.ID
	CandidateSessionID session.ID
	ExpectedOwnership  worldruntime.SessionOwnershipFence
	Generation         uint64
	ExpiresAt          time.Time
}

func (l characterTakeoverCandidateLease) Valid() bool {
	return l.CharacterID != "" &&
		l.CandidateSessionID != 0 &&
		l.ExpectedOwnership.Valid() &&
		l.ExpectedOwnership.CharacterID == l.CharacterID &&
		l.Generation != 0 &&
		!l.ExpiresAt.IsZero()
}

type characterTakeoverCandidateState struct {
	lease         characterTakeoverCandidateLease
	cooldownOwner worldruntime.SessionOwnershipFence
	cooldownUntil time.Time
}

type takeoverCandidateGate struct {
	mu         sync.Mutex
	ttl        time.Duration
	cooldown   time.Duration
	now        func() time.Time
	generation uint64
	states     map[characteridentity.ID]characterTakeoverCandidateState
}

func newTakeoverCandidateGate(ttl, cooldown time.Duration) *takeoverCandidateGate {
	if ttl <= 0 {
		ttl = defaultCharacterTakeoverCandidateTTL
	}
	if cooldown < 0 {
		cooldown = 0
	}
	return &takeoverCandidateGate{
		ttl:      ttl,
		cooldown: cooldown,
		now:      time.Now,
		states:   make(map[characteridentity.ID]characterTakeoverCandidateState),
	}
}

func (g *takeoverCandidateGate) acquire(request CharacterTakeoverRequest) (characterTakeoverCandidateLease, error) {
	if g == nil || !request.Valid() {
		return characterTakeoverCandidateLease{}, ErrInvalidCharacterTakeoverCandidate
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()

	// Active takeover is already a cold path, so opportunistically remove expired cooldown-only
	// entries here rather than retaining one historical map entry for every character that ever
	// completed a takeover. Active candidate leases are never swept by another CharacterID.
	g.cleanupExpiredCooldownsLocked(now)

	state := g.states[request.Identity.ID]
	if state.lease.Valid() {
		if now.Before(state.lease.ExpiresAt) {
			return characterTakeoverCandidateLease{}, ErrCharacterTakeoverCandidateReserved
		}
		state.lease = characterTakeoverCandidateLease{}
	}
	if !state.cooldownUntil.IsZero() {
		if now.Before(state.cooldownUntil) && state.cooldownOwner == request.ExpectedOwnership {
			g.states[request.Identity.ID] = state
			return characterTakeoverCandidateLease{}, ErrCharacterTakeoverCoolingDown
		}
		// Cooldown is lazy-expiry and exact-owner bound. If the current world-owner fence
		// differs, the cooldown belongs to an older ownership generation and must not block it.
		state.cooldownOwner = worldruntime.SessionOwnershipFence{}
		state.cooldownUntil = time.Time{}
	}

	g.generation++
	if g.generation == 0 {
		g.generation++
	}
	lease := characterTakeoverCandidateLease{
		CharacterID:        request.Identity.ID,
		CandidateSessionID: request.CandidateSessionID,
		ExpectedOwnership:  request.ExpectedOwnership,
		Generation:         g.generation,
		ExpiresAt:          now.Add(g.ttl),
	}
	state.lease = lease
	g.states[request.Identity.ID] = state
	return lease, nil
}

func (g *takeoverCandidateGate) validate(lease characterTakeoverCandidateLease) error {
	if g == nil || !lease.Valid() {
		return ErrInvalidCharacterTakeoverCandidate
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()

	state, ok := g.states[lease.CharacterID]
	if !ok || !sameTakeoverCandidateLease(state.lease, lease) {
		return ErrCharacterTakeoverCandidateStale
	}
	if !now.Before(state.lease.ExpiresAt) {
		state.lease = characterTakeoverCandidateLease{}
		g.storeOrDeleteLocked(lease.CharacterID, state)
		return ErrCharacterTakeoverCandidateExpired
	}
	return nil
}

func (g *takeoverCandidateGate) release(lease characterTakeoverCandidateLease) {
	if g == nil || !lease.Valid() {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	state, ok := g.states[lease.CharacterID]
	if !ok || !sameTakeoverCandidateLease(state.lease, lease) {
		return
	}
	state.lease = characterTakeoverCandidateLease{}
	g.storeOrDeleteLocked(lease.CharacterID, state)
}

func (g *takeoverCandidateGate) commit(lease characterTakeoverCandidateLease, ownership worldruntime.SessionOwnershipFence) error {
	if g == nil || !lease.Valid() || !validTakeoverCandidateCommit(lease, ownership) {
		return ErrInvalidCharacterTakeoverCandidate
	}
	now := g.now()
	g.mu.Lock()
	defer g.mu.Unlock()

	state, ok := g.states[lease.CharacterID]
	if !ok || !sameTakeoverCandidateLease(state.lease, lease) {
		return ErrCharacterTakeoverCandidateStale
	}
	if !now.Before(state.lease.ExpiresAt) {
		state.lease = characterTakeoverCandidateLease{}
		g.storeOrDeleteLocked(lease.CharacterID, state)
		return ErrCharacterTakeoverCandidateExpired
	}

	state.lease = characterTakeoverCandidateLease{}
	if g.cooldown > 0 {
		state.cooldownOwner = ownership
		state.cooldownUntil = now.Add(g.cooldown)
	}
	g.storeOrDeleteLocked(lease.CharacterID, state)
	return nil
}

func (g *takeoverCandidateGate) cleanupExpiredCooldownsLocked(now time.Time) {
	for characterID, state := range g.states {
		if state.lease.Valid() || state.cooldownUntil.IsZero() || now.Before(state.cooldownUntil) {
			continue
		}
		delete(g.states, characterID)
	}
}

func (g *takeoverCandidateGate) storeOrDeleteLocked(characterID characteridentity.ID, state characterTakeoverCandidateState) {
	if !state.lease.Valid() && state.cooldownUntil.IsZero() {
		delete(g.states, characterID)
		return
	}
	g.states[characterID] = state
}

func sameTakeoverCandidateLease(a, b characterTakeoverCandidateLease) bool {
	return a.Valid() && b.Valid() &&
		a.CharacterID == b.CharacterID &&
		a.CandidateSessionID == b.CandidateSessionID &&
		a.ExpectedOwnership == b.ExpectedOwnership &&
		a.Generation == b.Generation
}

func validTakeoverCandidateCommit(lease characterTakeoverCandidateLease, ownership worldruntime.SessionOwnershipFence) bool {
	return ownership.Valid() &&
		ownership.SessionID == lease.CandidateSessionID &&
		ownership.EntityID == lease.ExpectedOwnership.EntityID &&
		ownership.CharacterID == lease.CharacterID &&
		ownership.Epoch > lease.ExpectedOwnership.Epoch
}
