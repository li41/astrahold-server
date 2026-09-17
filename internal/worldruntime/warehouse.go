package worldruntime

import (
	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/warehouse"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	warehouseKeeperBlockerID = "stock-keeper"
	gmWarehouseSubject       = "1"
)

var gmWarehouseItems = []protocol.WarehouseItemStack{
	{ItemArchetypeID: WeaponEnhancementScrollItemArchetypeID, Quantity: 0},
	{ItemArchetypeID: ArmorEnhancementScrollItemArchetypeID, Quantity: 0},
}

func (r *Runtime) queueWarehouseMessage(sessionID session.ID, message protocol.Message) {
	r.pendingResourceMessages[sessionID] = append(r.pendingResourceMessages[sessionID], message)
}

func (r *Runtime) queueWarehouseResult(command useActionCommand, outcome protocol.WarehouseOutcome, reason protocol.WarehouseRejectionReason) {
	intent := *command.warehouse
	r.queueWarehouseMessage(command.sessionID, protocol.WarehouseResult{
		ClientActionSequence: command.sequence,
		Operation:            intent.Operation,
		Outcome:              outcome,
		Reason:               reason,
		ItemArchetypeID:      intent.ItemArchetypeID,
		Quantity:             intent.Quantity,
	})
}

func (r *Runtime) queuePersonalWarehouseSnapshot(sessionID session.ID, storage *warehouse.Storage) {
	stacks := storage.Snapshot()
	items := make([]protocol.WarehouseItemStack, len(stacks))
	for i, stack := range stacks {
		items[i] = protocol.WarehouseItemStack{ItemArchetypeID: stack.ItemArchetypeID, Quantity: stack.Quantity}
	}
	r.queueWarehouseMessage(sessionID, protocol.WarehouseSnapshot{Items: items})
}

func (r *Runtime) queueGMWarehouseSnapshot(sessionID session.ID) {
	items := make([]protocol.WarehouseItemStack, len(gmWarehouseItems))
	copy(items, gmWarehouseItems)
	r.queueWarehouseMessage(sessionID, protocol.WarehouseSnapshot{Items: items})
}

func warehouseKeeperInRange(position world.Position, blocker gameplayworld.Blocker) bool {
	if position.Layer != blocker.Layer {
		return false
	}
	dx := float32(0)
	if position.X < blocker.Bounds.MinX {
		dx = blocker.Bounds.MinX - position.X
	} else if position.X > blocker.Bounds.MaxX {
		dx = position.X - blocker.Bounds.MaxX
	}
	dz := float32(0)
	if position.Z < blocker.Bounds.MinZ {
		dz = blocker.Bounds.MinZ - position.Z
	} else if position.Z > blocker.Bounds.MaxZ {
		dz = position.Z - blocker.Bounds.MaxZ
	}
	return dx*dx+dz*dz <= npcInteractionRangeMeters*npcInteractionRangeMeters
}

func gmWarehouseItemAllowed(itemArchetypeID string) bool {
	switch itemArchetypeID {
	case WeaponEnhancementScrollItemArchetypeID, ArmorEnhancementScrollItemArchetypeID:
		return true
	default:
		return false
	}
}

func personalWarehouseStackAllowed(itemArchetypeID string) bool {
	if itemArchetypeID == "" {
		return false
	}
	// Equipment archetypes are not stack-storage content. Unique equipment instances are tracked
	// separately by inventory and therefore cannot enter this path at all.
	_, isEquipment := defaultEquipmentCatalog.Resolve(itemArchetypeID)
	return !isEquipment
}

func (r *Runtime) applyWarehouseCommand(name string, command useActionCommand, report *StepReport) {
	if command.warehouse == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: inventory.ErrInvalidQuantity})
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
	// A Reliable warehouse intent is consumed exactly once even when gameplay rejects it.
	s.MarkProcessedAction(command.sequence)

	state, ok := r.characters.State(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: character.ErrCharacterNotFound})
		return
	}
	if state.Defeated {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionDefeated)
		return
	}
	if r.characterStateWorld.MapID != string(gameplayworld.MapIDGMRoom) {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionWrongMap)
		return
	}
	if r.dynamic == nil {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
		return
	}
	blocker, err := r.dynamic.BlockerDefinition(warehouseKeeperBlockerID)
	if err != nil {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
		return
	}
	enabled, err := r.dynamic.BlockerEnabled(warehouseKeeperBlockerID)
	if err != nil || !enabled {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
		return
	}
	entity, ok := r.world.Entity(s.EntityID)
	if !ok {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: command.sessionID, Err: ErrSessionEntityNotFound})
		return
	}
	if !warehouseKeeperInRange(entity.Transform.Position, blocker) {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionOutOfRange)
		return
	}

	intent := *command.warehouse
	switch intent.Operation {
	case protocol.WarehouseOperationServices:
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeOpened, "")
		return
	case protocol.WarehouseOperationOpenGM, protocol.WarehouseOperationWithdrawGM:
		if s.AuthenticationSubject() != gmWarehouseSubject {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionNotAuthorized)
			return
		}
	case protocol.WarehouseOperationOpenPersonal, protocol.WarehouseOperationDepositPersonal, protocol.WarehouseOperationWithdrawPersonal:
	default:
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInvalidRequest)
		return
	}

	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
		return
	}
	storage := r.warehouses[s.CharacterIdentity.ID]
	if storage == nil {
		storage = warehouse.New()
		r.warehouses[s.CharacterIdentity.ID] = storage
	}

	switch intent.Operation {
	case protocol.WarehouseOperationOpenGM:
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeOpened, "")
		r.queueGMWarehouseSnapshot(command.sessionID)
	case protocol.WarehouseOperationWithdrawGM:
		if intent.Quantity == 0 || !gmWarehouseItemAllowed(intent.ItemArchetypeID) {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInvalidRequest)
			return
		}
		if err := inv.Add(intent.ItemArchetypeID, intent.Quantity); err != nil {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInventoryRejected)
			return
		}
		r.sessionInventoryPending[command.sessionID] = struct{}{}
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeWithdrawn, "")
		r.queueGMWarehouseSnapshot(command.sessionID)
	case protocol.WarehouseOperationOpenPersonal:
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeOpened, "")
		r.queuePersonalWarehouseSnapshot(command.sessionID, storage)
	case protocol.WarehouseOperationDepositPersonal:
		if intent.Quantity == 0 {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInvalidRequest)
			return
		}
		if !personalWarehouseStackAllowed(intent.ItemArchetypeID) {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionEquipmentNotSupported)
			return
		}
		if inv.Quantity(intent.ItemArchetypeID) < intent.Quantity {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInsufficientInventory)
			return
		}
		if uint64(storage.Quantity(intent.ItemArchetypeID))+uint64(intent.Quantity) > uint64(^uint32(0)) {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
			return
		}
		if err := inv.Remove(intent.ItemArchetypeID, intent.Quantity); err != nil {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInsufficientInventory)
			return
		}
		if err := storage.Add(intent.ItemArchetypeID, intent.Quantity); err != nil {
			_ = inv.Add(intent.ItemArchetypeID, intent.Quantity)
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
			return
		}
		r.sessionInventoryPending[command.sessionID] = struct{}{}
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeDeposited, "")
		r.queuePersonalWarehouseSnapshot(command.sessionID, storage)
	case protocol.WarehouseOperationWithdrawPersonal:
		if intent.Quantity == 0 {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInvalidRequest)
			return
		}
		if !personalWarehouseStackAllowed(intent.ItemArchetypeID) {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionEquipmentNotSupported)
			return
		}
		if storage.Quantity(intent.ItemArchetypeID) < intent.Quantity {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInsufficientWarehouse)
			return
		}
		if err := inv.Add(intent.ItemArchetypeID, intent.Quantity); err != nil {
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionInventoryRejected)
			return
		}
		if err := storage.Remove(intent.ItemArchetypeID, intent.Quantity); err != nil {
			_ = inv.Remove(intent.ItemArchetypeID, intent.Quantity)
			r.queueWarehouseResult(command, protocol.WarehouseOutcomeRejected, protocol.WarehouseRejectionServerRejected)
			return
		}
		r.sessionInventoryPending[command.sessionID] = struct{}{}
		r.queueWarehouseResult(command, protocol.WarehouseOutcomeWithdrawn, "")
		r.queuePersonalWarehouseSnapshot(command.sessionID, storage)
	}
}
