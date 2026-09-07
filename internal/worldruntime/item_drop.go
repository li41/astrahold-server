package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	itemPickupRangeMeters     = float32(2.5)
	itemDropSpawnOffsetMeters = float32(0.75)
	itemDropCollisionRadius   = float32(0.15)
	// Existing JSON EntitySpawn encodes entity_id as a number. Keep generated drop IDs below
	// IEEE-754's exact integer ceiling so Unreal JSON parsing never rounds authoritative identity.
	firstItemDropEntityID = world.EntityID(8_000_000_000_000_000)
	maxJsonExactEntityID  = world.EntityID((1 << 53) - 1)
)

var (
	ErrInvalidPickupIntent = errors.New("worldruntime: invalid pickup intent")
	ErrInvalidItemDrop     = errors.New("worldruntime: invalid item drop")
	ErrItemDropNotFound    = errors.New("worldruntime: item drop not found")
	ErrItemDropWrongLayer  = errors.New("worldruntime: item drop wrong layer")
	ErrItemDropOutOfRange  = errors.New("worldruntime: item drop out of range")
	ErrItemDropIDExhausted = errors.New("worldruntime: item drop entity id exhausted")
)

func validatePickupIntent(intent protocol.ClientPickupItem) error {
	if intent.DropEntityID == 0 {
		return ErrInvalidPickupIntent
	}
	return nil
}

func (r *Runtime) EnqueuePickupItem(id session.ID, sequence uint32, intent protocol.ClientPickupItem) error {
	if id == 0 || sequence == 0 {
		return ErrInvalidPickupIntent
	}
	if err := validatePickupIntent(intent); err != nil {
		return err
	}
	payload := intent
	return r.queue.tryPush(useActionCommand{sessionID: id, sequence: sequence, pickup: &payload})
}

// spawnItemDrop materializes one generic authoritative pickup entity. Ground drops are deliberately
// public immediately; the caller decides only whether auto-loot removes the public entity in the same
// owner tick after a successful inventory grant.
func (r *Runtime) spawnItemDrop(itemArchetypeID string, position world.Position) (world.EntityID, error) {
	if itemArchetypeID == "" {
		return 0, ErrInvalidItemDrop
	}
	dropID, err := r.allocateItemDropEntityID()
	if err != nil {
		return 0, err
	}
	entity := world.EntityState{
		ID:          dropID,
		Kind:        world.EntityItemDrop,
		ArchetypeID: itemArchetypeID,
		Transform:   world.Transform{Position: position},
	}
	if err := r.world.Spawn(entity, 0, itemDropCollisionRadius, 0); err != nil {
		return 0, err
	}
	return dropID, nil
}

// allocateItemDropEntityID never intentionally reuses a generated ID during this Runtime lifetime.
// That keeps a fresh drop from racing any observer's Reliable EntityDespawn knowledge.
func (r *Runtime) allocateItemDropEntityID() (world.EntityID, error) {
	id := r.nextItemDropEntityID
	if id < firstItemDropEntityID || id > maxJsonExactEntityID {
		return 0, ErrItemDropIDExhausted
	}
	for {
		if _, exists := r.world.Entity(id); !exists {
			if id == maxJsonExactEntityID {
				r.nextItemDropEntityID = 0
			} else {
				r.nextItemDropEntityID = id + 1
			}
			return id, nil
		}
		if id == maxJsonExactEntityID {
			r.nextItemDropEntityID = 0
			return 0, ErrItemDropIDExhausted
		}
		id++
		r.nextItemDropEntityID = id
	}
}

func (r *Runtime) applyPickupItem(name string, command useActionCommand, report *StepReport) {
	if command.pickup == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrInvalidPickupIntent})
		return
	}
	if command.ownership.Valid() {
		if err := r.characterIdentities.validateOwnership(command.sessionID, command.ownership); err != nil {
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
			return
		}
	}
	s, ok := r.sessions.Get(command.sessionID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: session.ErrSessionNotFound})
		return
	}
	if err := s.ValidateActionSequence(command.sequence); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	// The reliable intent is consumed once the world owner processes it, including legal rejections.
	s.MarkProcessedAction(command.sequence)

	player, ok := r.world.Entity(s.EntityID)
	if !ok || player.Kind != world.EntityPlayer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	request := *command.pickup
	dropEntity, ok := r.world.Entity(request.DropEntityID)
	if !ok || dropEntity.Kind != world.EntityItemDrop || dropEntity.ArchetypeID == "" {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrItemDropNotFound})
		return
	}
	if player.Transform.Position.Layer != dropEntity.Transform.Position.Layer {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrItemDropWrongLayer})
		return
	}
	if player.Transform.Position.DistanceSquared(dropEntity.Transform.Position) > itemPickupRangeMeters*itemPickupRangeMeters {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrItemDropOutOfRange})
		return
	}

	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: errors.New("worldruntime: inventory unavailable")})
		return
	}
	// Add enforces both stack and carry-weight capacity before world removal. If it rejects, the
	// public item remains on the ground and another player may attempt pickup immediately.
	if err := inv.Add(dropEntity.ArchetypeID, 1); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.world.Remove(request.DropEntityID)
	r.sessionInventoryPending[s.ID] = struct{}{}
}
