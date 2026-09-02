package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	grayWolfArchetypeID      = "wolf-gray-01"
	grayWolfPeltArchetypeID  = "item_gray_wolf_pelt"
	itemPickupRangeMeters    = float32(2.5)
	itemDropSpawnOffsetMeters = float32(0.75)
	firstItemDropEntityID    = world.EntityID(1 << 63)
)

var (
	ErrInvalidPickupIntent = errors.New("worldruntime: invalid pickup intent")
	ErrItemDropNotFound    = errors.New("worldruntime: item drop not found")
	ErrItemDropWrongLayer  = errors.New("worldruntime: item drop wrong layer")
	ErrItemDropOutOfRange  = errors.New("worldruntime: item drop out of range")
	ErrItemDropIDExhausted = errors.New("worldruntime: item drop entity id exhausted")
)

type itemDropState struct {
	ItemArchetypeID string
	Quantity        uint32
}

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

func (r *Runtime) spawnMonsterItemDrop(monster world.EntityState, report *StepReport) {
	if monster.Kind != world.EntityMonster || monster.ArchetypeID != grayWolfArchetypeID {
		return
	}

	dropID, err := r.allocateItemDropEntityID()
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "spawn_item_drop", Err: err})
		return
	}
	position := monster.Transform.Position
	position.X += itemDropSpawnOffsetMeters
	entity := world.EntityState{
		ID:          dropID,
		Kind:        world.EntityItemDrop,
		ArchetypeID: grayWolfPeltArchetypeID,
		Transform:   world.Transform{Position: position},
	}
	if err := r.world.Spawn(entity, 0, 0.15, 0); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "spawn_item_drop", Err: err})
		return
	}
	r.itemDrops[dropID] = itemDropState{ItemArchetypeID: grayWolfPeltArchetypeID, Quantity: 1}
}

func (r *Runtime) allocateItemDropEntityID() (world.EntityID, error) {
	if r.nextItemDropEntityID == 0 {
		r.nextItemDropEntityID = firstItemDropEntityID
	}
	for attempts := 0; attempts < 1024; attempts++ {
		candidate := r.nextItemDropEntityID
		r.nextItemDropEntityID++
		if r.nextItemDropEntityID == 0 {
			return 0, ErrItemDropIDExhausted
		}
		if _, exists := r.world.Entity(candidate); !exists {
			return candidate, nil
		}
	}
	return 0, ErrItemDropIDExhausted
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
	drop, ok := r.itemDrops[request.DropEntityID]
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrItemDropNotFound})
		return
	}
	dropEntity, ok := r.world.Entity(request.DropEntityID)
	if !ok || dropEntity.Kind != world.EntityItemDrop {
		delete(r.itemDrops, request.DropEntityID)
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
	// Inventory.Add validates stack capacity/overflow before any drop removal. World.Remove is an
	// infallible world-owner mutation, so success cannot leave a half-applied pickup transaction.
	if err := inv.Add(drop.ItemArchetypeID, drop.Quantity); err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: err})
		return
	}
	r.world.Remove(request.DropEntityID)
	delete(r.itemDrops, request.DropEntityID)
	r.sessionInventoryPending[s.ID] = struct{}{}
}
