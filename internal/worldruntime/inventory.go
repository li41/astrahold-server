package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const defaultInventoryCarryCapacity = uint64(100)

// First playable weight profile. Unknown future ItemArchetypeIDs still receive the policy's
// one-unit default rather than becoming weightless; authored overrides can move to content data
// when the item catalog grows beyond this vertical slice.
var defaultInventoryUnitWeights = map[string]uint32{
	"item_minor_healing_potion": 1,
	"item_minor_mana_potion":    1,
	"item_training_blade":       8,
	"item_gray_wolf_pelt":       2,
}

func newCharacterInventory(maxStacks int) *inventory.Inventory {
	return inventory.NewWithWeightPolicy(maxStacks, inventory.WeightPolicy{
		MaxWeight:         defaultInventoryCarryCapacity,
		DefaultUnitWeight: 1,
		UnitWeights:       defaultInventoryUnitWeights,
	})
}

func validateStarterInventory(maxStacks int, stacks []inventory.Stack) error {
	inv := newCharacterInventory(maxStacks)
	for _, stack := range stacks {
		if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) ensureSessionInventory(s *session.Session) {
	if s == nil {
		return
	}
	identity := s.CharacterIdentity.ID
	if _, ok := r.inventories[identity]; !ok {
		inv := newCharacterInventory(r.config.InventoryMaxStacks)
		for _, stack := range r.config.StarterInventory {
			// New validates this exact bootstrap contract. Keeping mutation here inside the
			// world owner makes the authoritative creation point explicit.
			if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil {
				panic(err)
			}
		}
		r.inventories[identity] = inv
	}
	r.sessionInventoryPending[s.ID] = struct{}{}
}

func (r *Runtime) removeSessionInventoryDelivery(id session.ID) {
	delete(r.sessionInventoryPending, id)
}

// replicatePendingInventories sends the paired authoritative Inventory + Equipment view.
// If Equipment delivery backpressures after Inventory succeeds, the pending marker remains and
// the next tick safely resends the same Inventory revision before retrying Equipment.
func (r *Runtime) replicatePendingInventories(tick uint64, report *StepReport) {
	if len(r.sessionInventoryPending) == 0 {
		return
	}
	for _, s := range r.sessions.List() {
		if _, pending := r.sessionInventoryPending[s.ID]; !pending {
			continue
		}
		inv := r.inventories[s.CharacterIdentity.ID]
		if inv == nil {
			delete(r.sessionInventoryPending, s.ID)
			continue
		}

		stacks := inv.Snapshot()
		items := make([]protocol.InventoryItemStack, 0, len(stacks))
		for _, stack := range stacks {
			items = append(items, protocol.InventoryItemStack{ArchetypeID: stack.ArchetypeID, Quantity: stack.Quantity})
		}
		inventoryMessage := protocol.InventorySnapshot{
			Revision:           inv.Revision(),
			CurrentCarryWeight: inv.CurrentWeight(),
			MaxCarryWeight:     inv.MaxWeight(),
			Items:              items,
		}
		inventoryEnvelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    inventoryMessage,
		}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(inventoryEnvelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) {
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: inventoryEnvelope.Delivery, MessageType: inventoryMessage.Type(), Err: err})
			}
			continue
		}

		slots := make([]protocol.EquipmentSlotState, 0, 1)
		if mainHand := inv.MainHand(); mainHand != "" {
			slots = append(slots, protocol.EquipmentSlotState{Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: mainHand})
		}
		equipmentMessage := protocol.EquipmentSnapshot{Revision: inv.EquipmentRevision(), Slots: slots}
		equipmentEnvelope := protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: tick,
			Message:    equipmentMessage,
		}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(equipmentEnvelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) {
				report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: equipmentEnvelope.Delivery, MessageType: equipmentMessage.Type(), Err: err})
			}
			continue
		}

		delete(r.sessionInventoryPending, s.ID)
	}
	for id := range r.sessionInventoryPending {
		if _, ok := r.sessions.Get(id); !ok {
			delete(r.sessionInventoryPending, id)
		}
	}
}
