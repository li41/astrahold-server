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
	weights := map[string]uint32{"item_minor_healing_potion":1,"item_minor_mana_potion":1,"item_training_blade":8,"item_gray_wolf_pelt":2}
	for id, weight := range defaultEquipmentCatalog.UnitWeights() { if id == "" || weight == 0 { panic("worldruntime: invalid equipment catalog weight") }; if _, exists := weights[id]; exists { panic("worldruntime: equipment catalog collides with base inventory weight") }; weights[id] = weight }
	return weights
}
func newCharacterInventory(maxStacks int) *inventory.Inventory { return inventory.NewWithWeightPolicy(maxStacks, inventory.WeightPolicy{MaxWeight:defaultInventoryCarryCapacity, DefaultUnitWeight:1, UnitWeights:defaultInventoryUnitWeights}) }
func validateStarterInventory(maxStacks int, stacks []inventory.Stack) error { inv := newCharacterInventory(maxStacks); for _, stack := range stacks { if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil { return err } }; return nil }

func validateDurableEquipmentInstance(instance iteminstance.Instance, kind equipmentcatalog.Kind, slot equipmentcatalog.Slot) error {
	definition, ok := defaultEquipmentCatalog.Resolve(instance.ItemArchetypeID); if !ok { return ErrEquipmentItemNotAllowed }
	if kind != "" && !equipmentDefinitionAllowed(definition, kind, slot) { return ErrEquipmentItemNotAllowed }
	return iteminstance.Validate(instance, definition)
}
func legacyEquipmentArchetypeAllowed(itemArchetypeID string, kind equipmentcatalog.Kind, slot equipmentcatalog.Slot) bool {
	if kind == equipmentcatalog.KindWeapon && slot == equipmentcatalog.SlotMainHand && itemArchetypeID == trainingBladeArchetypeID { return true }
	definition, ok := defaultEquipmentCatalog.Resolve(itemArchetypeID); return ok && definition.Tier == equipmentcatalog.TierLow && equipmentDefinitionAllowed(definition, kind, slot)
}

func durableInventoryState(inv *inventory.Inventory) (characterstate.InventoryState, error) {
	if inv == nil { return characterstate.InventoryState{}, errors.New("worldruntime: inventory unavailable") }
	if err := validateInventoryHandCombination(inv); err != nil { return characterstate.InventoryState{}, err }
	stacks := inv.Snapshot(); durableStacks := make([]characterstate.InventoryStack, 0, len(stacks))
	for _, stack := range stacks { if definition, ok := defaultEquipmentCatalog.Resolve(stack.ArchetypeID); ok && definition.Tier != equipmentcatalog.TierLow { return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed }; durableStacks = append(durableStacks, characterstate.InventoryStack{ItemArchetypeID:stack.ArchetypeID, Quantity:stack.Quantity}) }
	instances := inv.InstanceSnapshot(); for _, instance := range instances { if err := validateDurableEquipmentInstance(instance, "", ""); err != nil { return characterstate.InventoryState{}, err } }
	equipment := make([]characterstate.EquipmentSlotState, 0, 7)
	for _, item := range inv.EquippedArchetypeSnapshot() {
		kind, ok := expectedEquipmentKind(item.Slot); if !ok { return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed }
		catalogSlot, ok := catalogEquipmentSlot(item.Slot); if !ok || !legacyEquipmentArchetypeAllowed(item.ArchetypeID, kind, catalogSlot) { return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed }
		equipment = append(equipment, characterstate.EquipmentSlotState{Slot:string(item.Slot), ItemArchetypeID:item.ArchetypeID})
	}
	equippedInstances := make([]characterstate.EquipmentInstanceSlotState, 0, 7)
	for _, equipped := range inv.EquippedInstanceSnapshot() {
		kind, ok := expectedEquipmentKind(equipped.Slot); if !ok { return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed }
		catalogSlot, ok := catalogEquipmentSlot(equipped.Slot); if !ok { return characterstate.InventoryState{}, ErrEquipmentItemNotAllowed }
		if err := validateDurableEquipmentInstance(equipped.Item, kind, catalogSlot); err != nil { return characterstate.InventoryState{}, err }
		data, err := iteminstance.CanonicalShapeJSON(equipped.Item); if err != nil { return characterstate.InventoryState{}, err }
		equippedInstances = append(equippedInstances, characterstate.EquipmentInstanceSlotState{Slot:string(equipped.Slot), ItemInstanceJSON:string(data)})
	}
	return characterstate.NewInventoryStateWithSlots(durableStacks, instances, equipment, equippedInstances)
}

func restoreCharacterInventory(maxStacks int, state characterstate.InventoryState) (*inventory.Inventory, error) {
	if !state.Initialized { return nil, nil }
	canonical, err := characterstate.CanonicalInventoryState(state); if err != nil { return nil, characterstate.ErrInvalidSnapshot }; state = canonical
	if err := validateInventoryStateHandCombination(state); err != nil { return nil, err }
	stacks, err := state.Stacks(); if err != nil { return nil, err }; instances, err := state.Instances(); if err != nil { return nil, err }; equipment, err := state.Equipment(); if err != nil { return nil, err }; equippedInstances, err := state.EquipmentInstances(); if err != nil { return nil, err }
	inv := newCharacterInventory(maxStacks)
	for _, item := range equipment {
		slot := inventory.EquipmentSlot(item.Slot); kind, ok := expectedEquipmentKind(slot); if !ok { return nil, ErrEquipmentItemNotAllowed }; catalogSlot, ok := catalogEquipmentSlot(slot); if !ok || !legacyEquipmentArchetypeAllowed(item.ItemArchetypeID, kind, catalogSlot) { return nil, ErrEquipmentItemNotAllowed }
		if err := inv.Add(item.ItemArchetypeID, 1); err != nil { return nil, err }; if err := inv.Equip(slot, item.ItemArchetypeID); err != nil { return nil, err }
	}
	for _, item := range equippedInstances {
		slot := inventory.EquipmentSlot(item.Slot); kind, ok := expectedEquipmentKind(slot); if !ok { return nil, ErrEquipmentItemNotAllowed }; catalogSlot, ok := catalogEquipmentSlot(slot); if !ok { return nil, ErrEquipmentItemNotAllowed }
		instance, err := iteminstance.DecodeCanonicalShapeJSON([]byte(item.ItemInstanceJSON)); if err != nil { return nil, err }; if err := validateDurableEquipmentInstance(instance, kind, catalogSlot); err != nil { return nil, err }; if err := inv.AddInstance(instance); err != nil { return nil, err }; if err := inv.EquipInstance(slot, instance.ID); err != nil { return nil, err }
	}
	for _, instance := range instances { if err := validateDurableEquipmentInstance(instance, "", ""); err != nil { return nil, err }; if err := inv.AddInstance(instance); err != nil { return nil, err } }
	for _, stack := range stacks { if definition, ok := defaultEquipmentCatalog.Resolve(stack.ItemArchetypeID); ok && definition.Tier != equipmentcatalog.TierLow { return nil, ErrEquipmentItemNotAllowed }; if err := inv.Add(stack.ItemArchetypeID, stack.Quantity); err != nil { return nil, err } }
	return inv, nil
}

func (r *Runtime) ensureSessionInventory(s *session.Session) {
	if s == nil { return }; identity := s.CharacterIdentity.ID
	if _, ok := r.inventories[identity]; !ok { inv := newCharacterInventory(r.config.InventoryMaxStacks); for _, stack := range r.config.StarterInventory { if err := inv.Add(stack.ArchetypeID, stack.Quantity); err != nil { panic(err) } }; r.inventories[identity] = inv }
	if s.CharacterIdentity.Assurance == characteridentity.AssuranceTrusted { r.queueCurrentActionResourceState(s) }
	r.sessionInventoryPending[s.ID] = struct{}{}
}
func (r *Runtime) removeSessionInventoryDelivery(id session.ID) { delete(r.sessionInventoryPending, id) }

func (r *Runtime) replicatePendingInventories(tick uint64, report *StepReport) {
	r.pruneItemUseCooldowns(tick); r.retryPendingItemUseResults(tick, report); if len(r.sessionInventoryPending) == 0 { return }
	r.sessions.RangeUnordered(func(s *session.Session) bool {
		if _, pending := r.sessionInventoryPending[s.ID]; !pending { return true }; inv := r.inventories[s.CharacterIdentity.ID]; if inv == nil { delete(r.sessionInventoryPending, s.ID); return true }
		inventoryInstanceMessage, err := buildInventoryInstanceSnapshot(inv); if err != nil { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID, Delivery:protocol.DeliveryReliableOrdered, MessageType:protocol.MessageInventoryInstanceSnapshot, Err:err}); return true }
		equipmentInstanceMessage, err := buildEquipmentInstanceSnapshot(inv); if err != nil { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID, Delivery:protocol.DeliveryReliableOrdered, MessageType:protocol.MessageEquipmentInstanceSnapshot, Err:err}); return true }
		stacks := inv.Snapshot(); items := make([]protocol.InventoryItemStack, 0, len(stacks)); for _, stack := range stacks { items = append(items, protocol.InventoryItemStack{ArchetypeID:stack.ArchetypeID, Quantity:stack.Quantity}) }
		messages := []protocol.Message{protocol.InventorySnapshot{Revision:inv.Revision(), CurrentCarryWeight:inv.CurrentWeight(), MaxCarryWeight:inv.MaxWeight(), Items:items}, inventoryInstanceMessage}
		slots := make([]protocol.EquipmentSlotState, 0, 7); for _, item := range inv.EquippedArchetypeSnapshot() { slot, ok := protocolEquipmentSlot(item.Slot); if !ok { continue }; slots = append(slots, protocol.EquipmentSlotState{Slot:slot, ItemArchetypeID:item.ArchetypeID}) }
		messages = append(messages, protocol.EquipmentSnapshot{Revision:inv.EquipmentRevision(), Slots:slots}, equipmentInstanceMessage, r.appearanceSnapshotForSession(s))
		for _, message := range messages { envelope := protocol.Envelope{Delivery:protocol.DeliveryReliableOrdered, Sequence:s.NextOutboundSequence(protocol.DeliveryReliableOrdered), ServerTick:tick, Message:message}; report.Metrics.OutboundMessages++; if err := s.Connection().TrySend(envelope); err != nil { if !errors.Is(err, session.ErrBackpressure) { report.DeliveryErrors = append(report.DeliveryErrors, DeliveryError{SessionID:s.ID, Delivery:envelope.Delivery, MessageType:message.Type(), Err:err}) }; return true } }
		delete(r.sessionInventoryPending, s.ID); return true
	})
	for id := range r.sessionInventoryPending { if _, ok := r.sessions.Get(id); !ok { delete(r.sessionInventoryPending, id) } }
}
