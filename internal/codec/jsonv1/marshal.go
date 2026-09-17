package jsonv1

import (
	"encoding/json"

	"github.com/li41/astrahold-server/internal/protocol"
)

func toItemAffixState(in protocol.ItemAffixState) itemAffixState {
	return itemAffixState{AffixID: in.AffixID, Strength: in.Strength, Value: in.Value}
}

func toItemInstanceState(in protocol.ItemInstanceState) itemInstanceState {
	out := itemInstanceState{ItemInstanceID: in.ItemInstanceID, ItemArchetypeID: in.ItemArchetypeID, EnhancementLevel: in.EnhancementLevel, Affixes: make([]itemAffixState, len(in.Affixes))}
	for i := range in.Affixes { out.Affixes[i] = toItemAffixState(in.Affixes[i]) }
	return out
}

func (Codec) Marshal(message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ClientMoveInput:
		return json.Marshal(clientMoveInput{DX: m.DirectionX, DZ: m.DirectionZ})
	case *protocol.ClientMoveInput:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientMoveInput{DX: m.DirectionX, DZ: m.DirectionZ})
	case protocol.ClientUseAction:
		return json.Marshal(clientUseAction{ActionID: m.ActionID, TargetKind: string(m.TargetKind), TargetID: m.TargetID, TargetX: m.TargetX, TargetZ: m.TargetZ})
	case *protocol.ClientUseAction:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientUseAction{ActionID: m.ActionID, TargetKind: string(m.TargetKind), TargetID: m.TargetID, TargetX: m.TargetX, TargetZ: m.TargetZ})
	case protocol.ClientEquipmentCommand:
		return json.Marshal(clientEquipmentCommand{Operation: string(m.Operation), Slot: string(m.Slot), ItemArchetypeID: m.ItemArchetypeID})
	case *protocol.ClientEquipmentCommand:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientEquipmentCommand{Operation: string(m.Operation), Slot: string(m.Slot), ItemArchetypeID: m.ItemArchetypeID})
	case protocol.ClientEquipmentInstanceCommand:
		return json.Marshal(clientEquipmentInstanceCommand{Operation: string(m.Operation), Slot: string(m.Slot), ItemInstanceID: m.ItemInstanceID})
	case *protocol.ClientEquipmentInstanceCommand:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientEquipmentInstanceCommand{Operation: string(m.Operation), Slot: string(m.Slot), ItemInstanceID: m.ItemInstanceID})
	case protocol.ClientEnhanceEquipment:
		return json.Marshal(clientEnhanceEquipment{ScrollItemArchetypeID: m.ScrollItemArchetypeID, ItemInstanceID: m.ItemInstanceID})
	case *protocol.ClientEnhanceEquipment:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientEnhanceEquipment{ScrollItemArchetypeID: m.ScrollItemArchetypeID, ItemInstanceID: m.ItemInstanceID})
	case protocol.ClientWarehouseCommand:
		return json.Marshal(clientWarehouseCommand{Operation: string(m.Operation), ItemArchetypeID: m.ItemArchetypeID, Quantity: m.Quantity})
	case *protocol.ClientWarehouseCommand:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientWarehouseCommand{Operation: string(m.Operation), ItemArchetypeID: m.ItemArchetypeID, Quantity: m.Quantity})
	case protocol.ClientPickupItem:
		return json.Marshal(clientPickupItem{DropEntityID: uint64(m.DropEntityID)})
	case *protocol.ClientPickupItem:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientPickupItem{DropEntityID: uint64(m.DropEntityID)})
	case protocol.ClientUseItem:
		return json.Marshal(clientUseItem{ItemArchetypeID: m.ItemArchetypeID})
	case *protocol.ClientUseItem:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientUseItem{ItemArchetypeID: m.ItemArchetypeID})
	case protocol.ClientInitialClassSelection:
		return json.Marshal(clientInitialClassSelection{ClassID: m.ClassID})
	case *protocol.ClientInitialClassSelection:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientInitialClassSelection{ClassID: m.ClassID})
	case protocol.ClientInteractNPC:
		return json.Marshal(clientInteractNPC{NPCEntityID: uint64(m.NPCEntityID)})
	case *protocol.ClientInteractNPC:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientInteractNPC{NPCEntityID: uint64(m.NPCEntityID)})
	case protocol.ClientShopCommand:
		return json.Marshal(clientShopCommand{Operation: string(m.Operation), NPCEntityID: uint64(m.NPCEntityID), OfferID: m.OfferID})
	case *protocol.ClientShopCommand:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientShopCommand{Operation: string(m.Operation), NPCEntityID: uint64(m.NPCEntityID), OfferID: m.OfferID})
	case protocol.ClientRespawnRequest:
		return json.Marshal(clientRespawnRequest{})
	case *protocol.ClientRespawnRequest:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientRespawnRequest{})
	case protocol.ActionStarted:
		return json.Marshal(actionStarted{ActionInstanceID: m.ActionInstanceID, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, TargetKind: string(m.TargetKind), TargetID: m.TargetID, TargetX: m.TargetX, TargetZ: m.TargetZ})
	case protocol.ActionRejected:
		return json.Marshal(actionRejected{ClientActionSequence: m.ClientActionSequence, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, TargetKind: string(m.TargetKind), Reason: string(m.Reason), CooldownReadyTick: m.CooldownReadyTick})
	case protocol.ItemUseResult:
		return json.Marshal(itemUseResult{ClientActionSequence: m.ClientActionSequence, ItemArchetypeID: m.ItemArchetypeID, Outcome: string(m.Outcome), Reason: string(m.Reason), AppliedAmount: m.AppliedAmount, CooldownReadyTick: m.CooldownReadyTick})
	case protocol.EquipmentEnhancementResult:
		return json.Marshal(equipmentEnhancementResult{ClientActionSequence: m.ClientActionSequence, ScrollItemArchetypeID: m.ScrollItemArchetypeID, ItemInstanceID: m.ItemInstanceID, Outcome: string(m.Outcome), Reason: string(m.Reason), PreviousLevel: m.PreviousLevel, CurrentLevel: m.CurrentLevel, ScrollConsumed: m.ScrollConsumed})
	case protocol.WarehouseResult:
		return json.Marshal(warehouseResult{ClientActionSequence: m.ClientActionSequence, Operation: string(m.Operation), Outcome: string(m.Outcome), Reason: string(m.Reason), ItemArchetypeID: m.ItemArchetypeID, Quantity: m.Quantity})
	case protocol.WarehouseSnapshot:
		out := warehouseSnapshot{Items: make([]warehouseItemStack, len(m.Items))}
		for i, item := range m.Items { out.Items[i] = warehouseItemStack{ItemArchetypeID: item.ItemArchetypeID, Quantity: item.Quantity} }
		return json.Marshal(out)
	case protocol.CharacterClassState:
		return json.Marshal(characterClassState{ClassID: m.ClassID})
	case protocol.CharacterClassResourceState:
		return json.Marshal(characterClassResourceState{EntityID: uint64(m.EntityID), ResourceID: m.ResourceID, Current: m.Current, Max: m.Max})
	case protocol.CharacterTargetResourceState:
		return json.Marshal(characterTargetResourceState{SourceEntityID: uint64(m.SourceEntityID), TargetEntityID: uint64(m.TargetEntityID), ResourceID: m.ResourceID, Current: m.Current, Max: m.Max})
	case protocol.InitialClassSelectionResult:
		return json.Marshal(initialClassSelectionResult{ClientActionSequence: m.ClientActionSequence, ClassID: m.ClassID, Outcome: string(m.Outcome), Reason: string(m.Reason)})
	case protocol.SessionWelcome:
		return json.Marshal(sessionWelcome{SessionID: m.SessionID, EntityID: uint64(m.EntityID), RealtimePort: m.RealtimePort, RealtimeToken: m.RealtimeToken, TickRateHz: m.TickRateHz, SnapshotRateHz: m.SnapshotRateHz, WorldID: m.World.WorldID, WorldRevision: m.World.Revision, GameplaySHA256: m.World.GameplaySHA256})
	case protocol.EntitySpawn:
		return json.Marshal(toEntitySpawn(m))
	case protocol.EntityDespawn:
		return json.Marshal(entityDespawn{EntityID: uint64(m.EntityID)})
	case protocol.WorldSnapshot:
		out := worldSnapshot{Tick: m.Tick, Entities: make([]entityTransform, len(m.Entities))}
		for i := range m.Entities { out.Entities[i] = toEntityTransform(m.Entities[i]) }
		return json.Marshal(out)
	case protocol.PositionCorrection:
		return json.Marshal(positionCorrection{Tick: m.Tick, EntityID: uint64(m.EntityID), Position: toPosition(m.Position), Yaw: m.Yaw, LastProcessedInputSequence: m.LastProcessedInputSequence})
	case protocol.WorldDynamicState:
		out := worldDynamicState{Revision: m.Revision, Blockers: make([]worldBlockerState, len(m.Blockers)), Gates: make([]worldGateState, len(m.Gates))}
		for i, b := range m.Blockers { out.Blockers[i] = worldBlockerState{ID: b.ID, Enabled: b.Enabled} }
		for i, g := range m.Gates { out.Gates[i] = worldGateState{ID: g.ID, HP: g.HP, MaxHP: g.MaxHP, Destroyed: g.Destroyed} }
		return json.Marshal(out)
	case protocol.EntityVitalsState:
		return json.Marshal(entityVitalsState{EntityID: uint64(m.EntityID), HP: m.HP, MaxHP: m.MaxHP, MP: m.MP, MaxMP: m.MaxMP, Defeated: m.Defeated, ReviveProtectionUntilTick: m.ReviveProtectionUntilTick})
	case protocol.InventorySnapshot:
		out := inventorySnapshot{Revision: m.Revision, CurrentCarryWeight: m.CurrentCarryWeight, MaxCarryWeight: m.MaxCarryWeight, Items: make([]inventoryItemStack, len(m.Items))}
		for i, item := range m.Items { out.Items[i] = inventoryItemStack{ArchetypeID: item.ArchetypeID, Quantity: item.Quantity} }
		return json.Marshal(out)
	case protocol.InventoryInstanceSnapshot:
		out := inventoryInstanceSnapshot{Revision: m.Revision, Items: make([]itemInstanceState, len(m.Items))}
		for i := range m.Items { out.Items[i] = toItemInstanceState(m.Items[i]) }
		return json.Marshal(out)
	case protocol.EquipmentSnapshot:
		out := equipmentSnapshot{Revision: m.Revision, Slots: make([]equipmentSlotState, len(m.Slots))}
		for i, slot := range m.Slots { out.Slots[i] = equipmentSlotState{Slot: string(slot.Slot), ItemArchetypeID: slot.ItemArchetypeID} }
		return json.Marshal(out)
	case protocol.AppearanceSnapshot:
		return json.Marshal(toAppearanceSnapshot(m))
	case protocol.EquipmentInstanceSnapshot:
		out := equipmentInstanceSnapshot{Revision: m.Revision, Slots: make([]equipmentInstanceSlotState, len(m.Slots))}
		for i := range m.Slots { out.Slots[i] = equipmentInstanceSlotState{Slot: string(m.Slots[i].Slot), Item: toItemInstanceState(m.Slots[i].Item)} }
		return json.Marshal(out)
	case protocol.NPCInteraction:
		return json.Marshal(npcInteraction{NPCEntityID: uint64(m.NPCEntityID), NPCArchetypeID: m.NPCArchetypeID, DisplayName: m.DisplayName, Text: m.Text})
	case protocol.ShopSnapshot:
		out := shopSnapshot{Revision: m.Revision, NPCEntityID: uint64(m.NPCEntityID), ShopID: m.ShopID, Offers: make([]shopOffer, len(m.Offers))}
		for i, offer := range m.Offers { out.Offers[i] = shopOffer{OfferID: offer.OfferID, ItemArchetypeID: offer.ItemArchetypeID, Quantity: offer.Quantity, CostArchetypeID: offer.CostArchetypeID, CostQuantity: offer.CostQuantity} }
		return json.Marshal(out)
	case protocol.SiegeMatchState:
		return json.Marshal(siegeMatchState{Revision: m.Revision, Round: m.Round, MatchID: m.MatchID, AttackerID: m.AttackerID, DefenderID: m.DefenderID, YourTeam: string(m.YourTeam), Phase: string(m.Phase), BreachGateID: m.BreachGateID, ThroneObjectiveID: m.ThroneObjectiveID, GateBreached: m.GateBreached, WinnerTeam: string(m.WinnerTeam), WinnerID: m.WinnerID, CastleOwnerID: m.CastleOwnerID})
	case protocol.CombatEvent:
		return json.Marshal(combatEvent{ActionInstanceID: m.ActionInstanceID, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, Result: string(m.Result), TargetEntityID: uint64(m.TargetEntityID), ImpactX: m.ImpactX, ImpactZ: m.ImpactZ, Damage: m.Damage, Blocked: m.Blocked, CooldownReadyTick: m.CooldownReadyTick})
	default:
		return nil, ErrUnsupportedMessage
	}
}
