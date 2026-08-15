package worldruntime

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterIdentityMissing  = errors.New("worldruntime: character identity missing")
	ErrCharacterIdentityActive   = errors.New("worldruntime: character identity already active")
	ErrCharacterIdentityConflict = errors.New("worldruntime: entity character identity conflict")
)

type characterIdentityRegistry struct {
	byEntity          map[world.EntityID]characteridentity.Binding
	entityByCharacter map[characteridentity.ID]world.EntityID
}

func newCharacterIdentityRegistry() *characterIdentityRegistry {
	return &characterIdentityRegistry{
		byEntity:          make(map[world.EntityID]characteridentity.Binding),
		entityByCharacter: make(map[characteridentity.ID]world.EntityID),
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

func (r *characterIdentityRegistry) validateAdmission(identity characteridentity.Binding) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrCharacterAdmissionRequiresTrustedIdentity
	}
	if entityID, ok := r.entityByCharacter[identity.ID]; ok {
		return fmt.Errorf("%w: character=%s current_entity=%d", ErrCharacterIdentityActive, identity.ID, entityID)
	}
	return nil
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
