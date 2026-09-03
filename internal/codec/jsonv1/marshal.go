package jsonv1

import (
	"encoding/json"

	"github.com/li41/astrahold-server/internal/protocol"
)

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
	case protocol.ClientPickupItem:
		return json.Marshal(clientPickupItem{DropEntityID: uint64(m.DropEntityID)})
	case *protocol.ClientPickupItem:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientPickupItem{DropEntityID: uint64(m.DropEntityID)})
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
	case protocol.ActionStarted:
		return json.Marshal(actionStarted{ActionInstanceID: m.ActionInstanceID, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, TargetKind: string(m.TargetKind), TargetID: m.TargetID, TargetX: m.TargetX, TargetZ: m.TargetZ})
	case protocol.ActionRejected:
		return json.Marshal(actionRejected{ClientActionSequence: m.ClientActionSequence, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, TargetKind: string(m.TargetKind), Reason: string(m.Reason), CooldownReadyTick: m.CooldownReadyTick})
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
		out := inventorySnapshot{Revision: m.Revision, Items: make([]inventoryItemStack, len(m.Items))}
		for i, item := range m.Items { out.Items[i] = inventoryItemStack{ArchetypeID: item.ArchetypeID, Quantity: item.Quantity} }
		return json.Marshal(out)
	case protocol.EquipmentSnapshot:
		out := equipmentSnapshot{Revision: m.Revision, Slots: make([]equipmentSlotState, len(m.Slots))}
		for i, slot := range m.Slots { out.Slots[i] = equipmentSlotState{Slot: string(slot.Slot), ItemArchetypeID: slot.ItemArchetypeID} }
		return json.Marshal(out)
	case protocol.NPCInteraction:
		return json.Marshal(npcInteraction{NPCEntityID: uint64(m.NPCEntityID), NPCArchetypeID: m.NPCArchetypeID, DisplayName: m.DisplayName, Text: m.Text})
	case protocol.ShopSnapshot:
		out := shopSnapshot{Revision: m.Revision, NPCEntityID: uint64(m.NPCEntityID), ShopID: m.ShopID, Offers: make([]shopOffer, len(m.Offers))}
		for i, offer := range m.Offers {
			out.Offers[i] = shopOffer{OfferID: offer.OfferID, ItemArchetypeID: offer.ItemArchetypeID, Quantity: offer.Quantity, CostArchetypeID: offer.CostArchetypeID, CostQuantity: offer.CostQuantity}
		}
		return json.Marshal(out)
	case protocol.SiegeMatchState:
		return json.Marshal(siegeMatchState{Revision: m.Revision, Round: m.Round, MatchID: m.MatchID, AttackerID: m.AttackerID, DefenderID: m.DefenderID, YourTeam: string(m.YourTeam), Phase: string(m.Phase), BreachGateID: m.BreachGateID, ThroneObjectiveID: m.ThroneObjectiveID, GateBreached: m.GateBreached, WinnerTeam: string(m.WinnerTeam), WinnerID: m.WinnerID, CastleOwnerID: m.CastleOwnerID})
	case protocol.CombatEvent:
		return json.Marshal(combatEvent{ActionInstanceID: m.ActionInstanceID, ActorEntityID: uint64(m.ActorEntityID), ActionID: m.ActionID, Result: string(m.Result), TargetEntityID: uint64(m.TargetEntityID), ImpactX: m.ImpactX, ImpactZ: m.ImpactZ, Damage: m.Damage, CooldownReadyTick: m.CooldownReadyTick})
	default:
		return nil, ErrUnsupportedMessage
	}
}
