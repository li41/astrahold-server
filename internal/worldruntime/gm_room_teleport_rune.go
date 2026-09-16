package worldruntime

import (
	"errors"
	"fmt"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const (
	// AstraholdTeleportRuneItemArchetypeID is the stable gameplay identity for 星壘傳送符.
	AstraholdTeleportRuneItemArchetypeID = "item_astrahold_teleport_rune"
	// The rune is intentionally account-restricted. This value is compared only against the
	// Server-authenticated account subject carried by the live Session; it is never Client input.
	astraHoldTeleportRuneAccountSubject = "1"
)

var (
	ErrAstraholdTeleportRuneAccountDenied = errors.New("worldruntime: astrahold teleport rune account denied")
	ErrAstraholdTeleportRuneAlreadyInGMRoom = errors.New("worldruntime: astrahold teleport rune already in gm room")
)

func isAstraholdTeleportRune(itemArchetypeID string) bool {
	return itemArchetypeID == AstraholdTeleportRuneItemArchetypeID
}

// applyAstraholdTeleportRune runs after ownership/session/action-sequence validation on the
// single world-owner path. The Client supplies only ItemArchetypeID; account eligibility,
// destination world, position and facing are all Server-owned.
func (r *Runtime) applyAstraholdTeleportRune(
	name string,
	s *session.Session,
	clientActionSequence uint32,
	inv *inventory.Inventory,
	report *StepReport,
) {
	if s == nil || inv == nil || report == nil {
		return
	}
	if inv.Quantity(AstraholdTeleportRuneItemArchetypeID) == 0 {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, inventory.ErrInsufficient, 0, report)
		return
	}
	state, ok := r.characters.State(s.EntityID)
	if !ok {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, character.ErrCharacterNotFound, 0, report)
		return
	}
	if state.Defeated {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, character.ErrCharacterDefeated, 0, report)
		return
	}
	if s.AuthenticationSubject() != astraHoldTeleportRuneAccountSubject {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, ErrAstraholdTeleportRuneAccountDenied, 0, report)
		return
	}
	if r.characterStateWorld.MapID == string(gameplayworld.MapIDGMRoom) {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, ErrAstraholdTeleportRuneAlreadyInGMRoom, 0, report)
		return
	}
	if r.characterStateOutbox == nil {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, ErrMapExitPersistenceUnavailable, 0, report)
		return
	}

	target := gameplayworld.GMRoomTransferTarget()
	request := MapExitRequest{
		DestinationWorld: characterstate.WorldRef{
			MapID:          string(target.MapID),
			WorldID:        target.WorldID,
			Revision:       target.Revision,
			GameplaySHA256: target.GameplaySHA256,
		},
		DestinationTransform: target.Transform,
	}
	if err := validateMapExitDestination(r.characterStateWorld, request); err != nil {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, err, 0, report)
		return
	}

	binding, snapshot, ok := r.captureCharacterStateSnapshot(s.ID, s.EntityID, nil)
	if !ok {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, ErrMapExitSnapshotUnavailable, 0, report)
		return
	}
	inventoryState, err := removeInventoryStateStack(snapshot.Inventory, AstraholdTeleportRuneItemArchetypeID, 1)
	if err != nil {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, err, 0, report)
		return
	}
	snapshot.Inventory = inventoryState
	snapshot.World = request.DestinationWorld
	snapshot.Position = request.DestinationTransform.Position
	snapshot.Yaw = request.DestinationTransform.Yaw

	removed, err := r.sessions.Remove(s.ID)
	if err != nil {
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, err, 0, report)
		return
	}
	if _, err := r.characterStateOutbox.Enqueue(binding, snapshot); err != nil {
		if rollbackErr := r.sessions.Add(removed); rollbackErr != nil {
			panic(fmt.Sprintf("worldruntime: teleport rune session rollback failed: %v", rollbackErr))
		}
		report.Metrics.CharacterStateSaveIntentFailures++
		r.rejectItemUse(name, s, clientActionSequence, AstraholdTeleportRuneItemArchetypeID, err, 0, report)
		return
	}
	report.Metrics.CharacterStateSaveIntentsEnqueued++

	// The durable transfer snapshot is already accepted by the outbox. No other world-owner
	// inventory writer can race here, so failure would indicate an internal invariant violation.
	if err := inv.Remove(AstraholdTeleportRuneItemArchetypeID, 1); err != nil {
		panic("worldruntime: teleport rune inventory invariant violated: " + err.Error())
	}
	r.sendItemUseResult(removed, protocol.ItemUseResult{
		ClientActionSequence: clientActionSequence,
		ItemArchetypeID:      AstraholdTeleportRuneItemArchetypeID,
		Outcome:              protocol.ItemUseOutcomeUsed,
	}, report)
	r.removeSiegeParticipant(removed)
	r.cleanupRemovedSession(removed, report)
}

// removeInventoryStateStack creates a canonical durable inventory with quantity removed without
// mutating the live inventory. Cross-map persistence can therefore fail and roll back the Session
// registry without ever consuming the item in authoritative source state.
func removeInventoryStateStack(state characterstate.InventoryState, itemArchetypeID string, quantity uint32) (characterstate.InventoryState, error) {
	if !state.Initialized || itemArchetypeID == "" || quantity == 0 {
		return characterstate.InventoryState{}, inventory.ErrInsufficient
	}
	stacks, err := state.Stacks()
	if err != nil {
		return characterstate.InventoryState{}, err
	}
	instances, err := state.Instances()
	if err != nil {
		return characterstate.InventoryState{}, err
	}
	equipment, err := state.Equipment()
	if err != nil {
		return characterstate.InventoryState{}, err
	}
	equippedInstances, err := state.EquipmentInstances()
	if err != nil {
		return characterstate.InventoryState{}, err
	}

	found := false
	out := make([]characterstate.InventoryStack, 0, len(stacks))
	for _, stack := range stacks {
		if stack.ItemArchetypeID != itemArchetypeID {
			out = append(out, stack)
			continue
		}
		if found || stack.Quantity < quantity {
			return characterstate.InventoryState{}, inventory.ErrInsufficient
		}
		found = true
		if stack.Quantity > quantity {
			stack.Quantity -= quantity
			out = append(out, stack)
		}
	}
	if !found {
		return characterstate.InventoryState{}, inventory.ErrInsufficient
	}
	return characterstate.NewInventoryStateWithSlots(out, instances, equipment, equippedInstances)
}
