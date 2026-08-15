package worldruntime

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const characterAdmissionLeaseDuration = 60 * time.Second

var (
	ErrCharacterIdentityMissing              = errors.New("worldruntime: character identity missing")
	ErrCharacterIdentityActive               = errors.New("worldruntime: character identity already active")
	ErrCharacterIdentityConflict             = errors.New("worldruntime: entity character identity conflict")
	ErrCharacterAdmissionReserved            = errors.New("worldruntime: character admission already reserved")
	ErrCharacterAdmissionLeaseRequired       = errors.New("worldruntime: character admission lease required")
	ErrCharacterAdmissionLeaseInvalid        = errors.New("worldruntime: character admission lease invalid")
	ErrCharacterAdmissionLeaseExpired        = errors.New("worldruntime: character admission lease expired")
	ErrCharacterAdmissionGenerationExhausted = errors.New("worldruntime: character admission generation exhausted")
)

// CharacterAdmissionLease is a process-local fencing token issued by the world owner for
// one trusted CharacterID admission attempt. It is never sent to the Client and is not a
// durable/distributed lock. Generation prevents an older release from clearing a newer lease.
type CharacterAdmissionLease struct {
	CharacterID characteridentity.ID
	Generation  uint64
	ExpiresAt   time.Time
}

func (l CharacterAdmissionLease) Valid() bool {
	return l.CharacterID != "" && l.Generation != 0 && !l.ExpiresAt.IsZero()
}

type characterAdmissionOperation struct {
	identity  characteridentity.Binding
	lease     *CharacterAdmissionLease
	ownership *SessionOwnershipFence
	release   bool
}

type characterIdentityRegistry struct {
	byEntity                map[world.EntityID]characteridentity.Binding
	entityByCharacter       map[characteridentity.ID]world.EntityID
	admissionByCharacter    map[characteridentity.ID]CharacterAdmissionLease
	ownershipByCharacter    map[characteridentity.ID]SessionOwnershipFence
	ownershipBySession      map[session.ID]SessionOwnershipFence
	nextAdmissionGeneration uint64
	nextOwnershipEpoch      uint64
}

func newCharacterIdentityRegistry() *characterIdentityRegistry {
	return &characterIdentityRegistry{
		byEntity:             make(map[world.EntityID]characteridentity.Binding),
		entityByCharacter:    make(map[characteridentity.ID]world.EntityID),
		admissionByCharacter: make(map[characteridentity.ID]CharacterAdmissionLease),
		ownershipByCharacter: make(map[characteridentity.ID]SessionOwnershipFence),
		ownershipBySession:   make(map[session.ID]SessionOwnershipFence),
	}
}

func (r *characterIdentityRegistry) validateSession(s *session.Session) error {
	if s == nil || !s.CharacterIdentity.Valid() {
		return ErrCharacterIdentityMissing
	}
	if current, ok := r.byEntity[s.EntityID]; ok && current != s.CharacterIdentity {
		return fmt.Errorf("%w: entity=%d current=%s incoming=%s", ErrCharacterIdentityConflict, s.EntityID, current.ID, s.CharacterIdentity.ID)
	}
	if entityID, ok := r.entityByCharacter[s.CharacterIdentity.ID]; ok && entityID != s.EntityID {
		return fmt.Errorf("%w: character=%s current_entity=%d incoming_entity=%d", ErrCharacterIdentityActive, s.CharacterIdentity.ID, entityID, s.EntityID)
	}
	return nil
}

// validateAdmission executes reserve, release, and S3-F.20 connection-plan operations because
// each must stay in the same world-owner FIFO position used by S3-F.14. A normal admission
// still rejects an active CharacterID. Connection-plan mode supplies ownership and treats an
// exact active trusted owner as a successful takeover candidate instead of an admission error.
func (r *characterIdentityRegistry) validateAdmission(operation characterAdmissionOperation) error {
	if operation.release {
		r.releaseAdmission(operation.lease)
		return nil
	}
	identity := operation.identity
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted || operation.lease == nil {
		return ErrCharacterAdmissionRequiresTrustedIdentity
	}
	if entityID, ok := r.entityByCharacter[identity.ID]; ok {
		if operation.ownership == nil {
			return fmt.Errorf("%w: character=%s current_entity=%d", ErrCharacterIdentityActive, identity.ID, entityID)
		}
		ownership, err := r.currentOwnership(identity)
		if err != nil {
			return err
		}
		if ownership.EntityID != entityID {
			return fmt.Errorf("%w: character=%s current_entity=%d ownership_entity=%d", ErrCharacterIdentityConflict, identity.ID, entityID, ownership.EntityID)
		}
		*operation.ownership = ownership
		return nil
	}

	now := time.Now()
	if current, ok := r.admissionByCharacter[identity.ID]; ok {
		if now.Before(current.ExpiresAt) {
			return fmt.Errorf("%w: character=%s generation=%d", ErrCharacterAdmissionReserved, identity.ID, current.Generation)
		}
		delete(r.admissionByCharacter, identity.ID)
	}
	if r.nextAdmissionGeneration == math.MaxUint64 {
		return ErrCharacterAdmissionGenerationExhausted
	}
	r.nextAdmissionGeneration++
	lease := CharacterAdmissionLease{
		CharacterID: identity.ID,
		Generation:  r.nextAdmissionGeneration,
		ExpiresAt:   now.Add(characterAdmissionLeaseDuration),
	}
	r.admissionByCharacter[identity.ID] = lease
	*operation.lease = lease
	return nil
}

// validateJoinAdmission fences a reserved trusted admission. Existing internal/direct trusted
// joins remain valid when no reservation exists, but while a live reservation is present only
// its exact generation may commit the CharacterID.
func (r *characterIdentityRegistry) validateJoinAdmission(identity characteridentity.Binding, lease *CharacterAdmissionLease) error {
	if identity.Assurance != characteridentity.AssuranceTrusted {
		if lease != nil {
			return ErrCharacterAdmissionLeaseInvalid
		}
		return nil
	}

	now := time.Now()
	current, ok := r.admissionByCharacter[identity.ID]
	if ok && !now.Before(current.ExpiresAt) {
		delete(r.admissionByCharacter, identity.ID)
		ok = false
	}
	if lease == nil {
		if ok {
			return fmt.Errorf("%w: character=%s generation=%d", ErrCharacterAdmissionLeaseRequired, identity.ID, current.Generation)
		}
		return nil
	}
	if !lease.Valid() || lease.CharacterID != identity.ID {
		return ErrCharacterAdmissionLeaseInvalid
	}
	if !now.Before(lease.ExpiresAt) {
		return fmt.Errorf("%w: character=%s generation=%d", ErrCharacterAdmissionLeaseExpired, identity.ID, lease.Generation)
	}
	if !ok || current.Generation != lease.Generation || current.CharacterID != lease.CharacterID {
		return fmt.Errorf("%w: character=%s generation=%d", ErrCharacterAdmissionLeaseInvalid, identity.ID, lease.Generation)
	}
	return nil
}

func (r *characterIdentityRegistry) consumeAdmission(lease CharacterAdmissionLease) {
	current, ok := r.admissionByCharacter[lease.CharacterID]
	if ok && current.Generation == lease.Generation {
		delete(r.admissionByCharacter, lease.CharacterID)
	}
}

// releaseAdmission is intentionally idempotent. A missing, expired, or stale generation is
// a no-op; critically, an old release can never remove a newer reservation.
func (r *characterIdentityRegistry) releaseAdmission(lease *CharacterAdmissionLease) {
	if lease == nil || !lease.Valid() {
		return
	}
	current, ok := r.admissionByCharacter[lease.CharacterID]
	if !ok || current.Generation != lease.Generation {
		return
	}
	delete(r.admissionByCharacter, lease.CharacterID)
}

func (r *characterIdentityRegistry) bindSession(s *session.Session) {
	r.byEntity[s.EntityID] = s.CharacterIdentity
	r.entityByCharacter[s.CharacterIdentity.ID] = s.EntityID
}

func (r *characterIdentityRegistry) binding(entityID world.EntityID) (characteridentity.Binding, bool) {
	binding, ok := r.byEntity[entityID]
	return binding, ok
}

// removeEntity releases the active world ownership binding. Historical journal records
// already carry their immutable CharacterID and are unaffected by EntityID reuse.
func (r *characterIdentityRegistry) removeEntity(entityID world.EntityID) {
	binding, ok := r.byEntity[entityID]
	if !ok {
		return
	}
	delete(r.byEntity, entityID)
	if current, ok := r.entityByCharacter[binding.ID]; ok && current == entityID {
		delete(r.entityByCharacter, binding.ID)
	}
}
