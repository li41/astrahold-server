package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

const defaultInventoryCarryCapacity = uint64(100)

var defaultInventoryUnitWeights = mustDefaultInventoryUnitWeights()

func mustDefaultInventoryUnitWeights() map[string]uint32 {
	weights := map[string]uint32{
		"item_minor_healing_potion": 1,
		"item_minor_mana_potion":    1,
		"item_training_blade":       8,
		"item_gray_wolf_pelt":       2,
	}
	for itemArchetypeID, weight := range defaultEquipmentCatalog.UnitWeights() {
		if itemArchetypeID == "" || weight == 0 { panic("worldruntime: invalid equipment catalog weight") }
		if _, exists := weights[itemArchetypeID]; exists { panic("worldruntime: equipment catalog collides with base inventory weight") }
		weights[itemArchetypeID] = weight
	}
	return weights
}

func newCharacterInventory(maxStacks int) *inventory.Inventory {
	return inventory.NewWithWeightPolicy(maxStacks, inventory.WeightPolicy{MaxWeight: defaultInventoryCarryCapacity, DefaultUnitWeight: 1, UnitWeights: defaultInventoryUnitWeights})
}

func validateStarterInventory(maxStacks int, stacks []inventory.Stack) error {
	inv := newCharacterInventory(maxStacks)
	for _, stack := range stacks { if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil { return err } }
	return nil
}

func validateDurableEquipmentInstance(instance iteminstance.Instance, kind equipmentcatalog.Kind, slot equipmentcatalog.Slot) error {
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID)
	if !ok {
		return ErrEquipmentItemNotAllowed
	}
	if kind != "" && !equipmentDefinitionAllowed(definition, kind, slot) {
		return ErrEquipmentItemNotAllowed
	}
	return iteminstance.Validate(instance, definition)
}

// Legacy archetype-only equipment is intentionally restricted to low-tier equipment. Mid/high
// equipment must be a unique instance because its Server-rolled affixes are gameplay truth.
func legacyEquipmentArchetypeAllowed(itemArchetypeID string, kind equipmentcatalog.Kind, slot equipmentcatalog.Slot) bool {
	if kind == equipmentcatalog.KindWeapon && slot == equipmentcatalog.SlotMainHand && itemArchetypeID == trainingBladeArchetypeID {
		return true
	}
	definition, ok := defaultEquipmentCatalog.Resolve(itemArchetypeID)
	return ok && definition.Tier == equipmentcatalog.TierLow && equipmentDefinitionAllowed(definition, kind, slot)
}

func durableInventoryState(inv *inventory.Inventory) (characterstate.InventoryState, error) {
	if inv == nil { return characterstate.InventoryState{}, errors.New("worldruntime: inventory unavailable") }

	stacks := inv.Snapshot()
	durableStacks := make([]characterstate.InventoryStack, 0, len(stacks))
	for _, stack := range stacks {
		if definition, ok := defaultEquipmentCatalog.Resolve(stack.ArchetypeID); ok && definition.Tier != equipmentcatalog.TierLow {
			return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed
		}
		durableStacks = append(durableStacks, characterstate.InventoryStack{ItemArchetypeID: stack.ArchetypeID, Quantity: stack.Quantity})
	}

	instances := inv.InstanceSnapshot()
	for _, instance := range instances {
		if err := validateDurableEquipmentInstance(instance, "", ""); err != nil { return characterstate.InventoryState{}, err }
	}

	mainHand := inv.MainHand()
	var mainHandInstance *iteminstance.Instance
	if instance, ok := inv.MainHandInstance(); ok {
		if err := validateDurableEquipmentInstance(instance, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand); err != nil { return characterstate.InventoryState{}, err }
		mainHand = ""
		mainHandInstance = &instance
	} else if mainHand != "" && !legacyEquipmentArchetypeAllowed(mainHand, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand) {
		return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed
	}

	offHand := inv.OffHand()
	var offHandInstance *iteminstance.Instance
	if instance, ok := inv.OffHandInstance(); ok {
		if err := validateDurableEquipmentInstance(instance, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand); err != nil { return characterstate.InventoryState{}, err }
		offHand = ""
		offHandInstance = &instance
	} else if offHand != "" && !legacyEquipmentArchetypeAllowed(offHand, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand) {
		return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed
	}

	return characterstate.NewInventoryStateWithInstances(durableStacks, instances, mainHand, offHand, mainHandInstance, offHandInstance)
}

// restoreCharacterInventory restores classless durable inventory/equipment truth. Equipment
// legality depends only on the actual item and slot; retired ClassID never participates. Unique
// instances are restored exactly as persisted and are never sent through an affix roller.
func restoreCharacterInventory(maxStacks int, state characterstate.InventoryState) (*inventory.Inventory, error) {
	if !state.Initialized { return nil, nil }
	canonical, err := characterstate.CanonicalInventoryState(state)
	if err != nil || canonical != state { return nil, characterstate.ErrInvalidSnapshot }

	stacks, err := state.Stacks(); if err != nil { return nil, err }
	instances, err := state.Instances(); if err != nil { return nil, err }
	mainHandInstance, hasMainHandInstance, err := state.MainHandInstance(); if err != nil { return nil, err }
	offHandInstance, hasOffHandInstance, err := state.OffHandInstance(); if err != nil { return nil, err }

	inv := newCharacterInventory(maxStacks)
	if state.MainHand != "" {
		if !legacyEquipmentArchetypeAllowed(state.MainHand, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand) { return nil, ErrEquipmentItemNotAllowed }
		if err := inv.Add(state.MainHand, 1); err != nil { return nil, err }
		if err := inv.EquipMainHand(state.MainHand); err != nil { return nil, err }
	}
	if hasMainHandInstance {
		if err := validateDurableEquipmentInstance(mainHandInstance, equipmentcatalog.KindWeapon, equipmentcatalog.SlotMainHand); err != nil { return nil, err }
		if err := inv.AddInstance(mainHandInstance); err != nil { return nil, err }
		if err := inv.EquipMainHandInstance(mainHandInstance.ID); err != nil { return nil, err }
	}
	if state.OffHand != "" {
		if !legacyEquipmentArchetypeAllowed(state.OffHand, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand) { return nil, ErrEquipmentItemNotAllowed }
		if err := inv.Add(state.OffHand, 1); err != nil { return nil, err }
		if err := inv.EquipOffHand(state.OffHand); err != nil { return nil, err }
	}
	if hasOffHandInstance {
		if err := validateDurableEquipmentInstance(offHandInstance, equipmentcatalog.KindShield, equipmentcatalog.SlotOffHand); err != nil { return nil, err }
		if err := inv.AddInstance(offHandInstance); err != nil { return nil, err }
		if err := inv.EquipOffHandInstance(offHandInstance.ID); err != nil { return nil, err }
	}
	for _, instance := range instances {
		if err := validateDurableEquipmentInstance(instance, "", ""); err != nil { return nil, err }
		if err := inv.AddInstance(instance); err != nil { return nil, err }
	}
	for _, stack := range stacks {
		if definition, ok := defaultEquipmentCatalog.Resolve(stack.ItemArchetypeID); ok && definition.Tier != equipmentcatalog.TierLow {
			return nil, ErrEquipmentItemNotAllowed
		}
		if err := inv.Add(stack.ItemArchetypeID, stack.Quantity); err != nil { return nil, err }
	}
	return inv, nil
}

func (r *Runtime) ensureSessionInventory(s *session.Session) {
	if s == nil { return }
	identity := s.CharacterIdentity.ID
	if _, ok := r.inventories[identity]; !ok {
		inv := newCharacterInventory(r.config.InventoryMaxStacks)
		for _, stack := range r.config.StarterInventory { if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil { panic(err) } }
		r.inventories[identity] = inv
	}
	// Trusted sessions receive the current Server-owned action resource when one exists. Protocol
	// v27 still presents that generic state through Type117, but no profession identity is restored.
	if s.CharacterIdentity.Assurance == characteridentity.AssuranceTrusted {
		r.queueCurrentActionResourceState(s)
	}
	r.sessionInventoryPending[s.ID] = struct{}{}
}

func (r *Runtime) removeSessionInventoryDelivery(id session.ID) { delete(r.sessionInventoryPending, id) }

func (r *Runtime) replicatePendingInventories(tick uint64, report *StepReport) {
	// Resource feedback is retried once at the beginning of Runtime.Step, before current commands
	// can append newer states. Do not retry it again here in the same tick.
	r.pruneItemUseCooldowns(tick)
	r.retryPendingItemUseResults(tick, report)
	if len(r.sessionInventoryPending) == 0 { return }
	for _, s := range r.sessions.List() {
		if _, pending := r.sessionInventoryPending[s.ID]; !pending { continue }
		inv := r.inventories[s.CharacterIdentity.ID]
		if inv == nil { delete(r.sessionInventoryPending, s.ID); continue }
		stacks := inv.Snapshot()
		items := make([]protocol.InventoryItemStack, 0, len(stacks))
		for _, stack := range stacks { items = append(items, protocol.InventoryItemStack{ArchetypeID: stack.ArchetypeID, Quantity: stack.Quantity}) }
		inventoryMessage := protocol.InventorySnapshot{Revision: inv.Revision(), CurrentCarryWeight: inv.CurrentWeight(), MaxCarryWeight: inv.MaxWeight(), Items: items}
		inventoryEnvelope := protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: s.NextOutboundSequence(protocol.DeliveryReliableOrdered), ServerTick: tick, Message: inventoryMessage}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(inventoryEnvelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: inventoryEnvelope.Delivery, MessageType: inventoryMessage.Type(), Err: err}) }
			continue
		}

		slots := make([]protocol.EquipmentSlotState, 0, 2)
		if mainHand := inv.MainHand(); mainHand != "" { slots = append(slots, protocol.EquipmentSlotState{Slot: protocol.EquipmentSlotMainHand, ItemArchetypeID: mainHand}) }
		if offHand := inv.OffHand(); offHand != "" { slots = append(slots, protocol.EquipmentSlotState{Slot: protocol.EquipmentSlotOffHand, ItemArchetypeID: offHand}) }
		equipmentMessage := protocol.EquipmentSnapshot{Revision: inv.EquipmentRevision(), Slots: slots}
		equipmentEnvelope := protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: s.NextOutboundSequence(protocol.DeliveryReliableOrdered), ServerTick: tick, Message: equipmentMessage}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(equipmentEnvelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: equipmentEnvelope.Delivery, MessageType: equipmentMessage.Type(), Err: err}) }
			continue
		}

		appearanceMessage := r.appearanceSnapshotForSession(s)
		appearanceEnvelope := protocol.Envelope{Delivery: protocol.DeliveryReliableOrdered, Sequence: s.NextOutboundSequence(protocol.DeliveryReliableOrdered), ServerTick: tick, Message: appearanceMessage}
		report.Metrics.OutboundMessages++
		if err := s.Connection().TrySend(appearanceEnvelope); err != nil {
			if !errors.Is(err, session.ErrBackpressure) { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID: s.ID, Delivery: appearanceEnvelope.Delivery, MessageType: appearanceMessage.Type(), Err: err}) }
			continue
		}
		delete(r.sessionInventoryPending, s.ID)
	}
	for id := range r.sessionInventoryPending { if _, ok := r.sessions.Get(id); !ok { delete(r.sessionInventoryPending, id) } }
}