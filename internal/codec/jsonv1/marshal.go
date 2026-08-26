package jsonv1

import (
	"encoding/json"

	"github.com/li41/astrahold-server/internal/protocol"
)

func (Codec) Marshal(message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ClientMoveInput:
		return json.Marshal(clientMoveInput{DX:m.DirectionX,DZ:m.DirectionZ})
	case *protocol.ClientMoveInput:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientMoveInput{DX:m.DirectionX,DZ:m.DirectionZ})
	case protocol.ClientUseAction:
		return json.Marshal(clientUseAction{ActionID:m.ActionID,TargetKind:string(m.TargetKind),TargetID:m.TargetID,TargetX:m.TargetX,TargetZ:m.TargetZ})
	case *protocol.ClientUseAction:
		if m == nil { return nil, ErrUnsupportedMessage }
		return json.Marshal(clientUseAction{ActionID:m.ActionID,TargetKind:string(m.TargetKind),TargetID:m.TargetID,TargetX:m.TargetX,TargetZ:m.TargetZ})
	case protocol.ActionStarted:
		return json.Marshal(actionStarted{ActionInstanceID:m.ActionInstanceID,ActorEntityID:uint64(m.ActorEntityID),ActionID:m.ActionID,TargetKind:string(m.TargetKind),TargetID:m.TargetID,TargetX:m.TargetX,TargetZ:m.TargetZ})
	case protocol.SessionWelcome:
		return json.Marshal(sessionWelcome{SessionID:m.SessionID,EntityID:uint64(m.EntityID),RealtimePort:m.RealtimePort,RealtimeToken:m.RealtimeToken,TickRateHz:m.TickRateHz,SnapshotRateHz:m.SnapshotRateHz,WorldID:m.World.WorldID,WorldRevision:m.World.Revision,GameplaySHA256:m.World.GameplaySHA256})
	case protocol.EntitySpawn:
		return json.Marshal(toEntitySpawn(m))
	case protocol.EntityDespawn:
		return json.Marshal(entityDespawn{EntityID:uint64(m.EntityID)})
	case protocol.WorldSnapshot:
		out:=worldSnapshot{Tick:m.Tick,Entities:make([]entityTransform,len(m.Entities))};for i:=range m.Entities{out.Entities[i]=toEntityTransform(m.Entities[i])};return json.Marshal(out)
	case protocol.PositionCorrection:
		return json.Marshal(positionCorrection{Tick:m.Tick,EntityID:uint64(m.EntityID),Position:toPosition(m.Position),Yaw:m.Yaw,LastProcessedInputSequence:m.LastProcessedInputSequence})
	case protocol.WorldDynamicState:
		out:=worldDynamicState{Revision:m.Revision,Blockers:make([]worldBlockerState,len(m.Blockers)),Gates:make([]worldGateState,len(m.Gates))};for i,b:=range m.Blockers{out.Blockers[i]=worldBlockerState{ID:b.ID,Enabled:b.Enabled}};for i,g:=range m.Gates{out.Gates[i]=worldGateState{ID:g.ID,HP:g.HP,MaxHP:g.MaxHP,Destroyed:g.Destroyed}};return json.Marshal(out)
	case protocol.EntityVitalsState:
		return json.Marshal(entityVitalsState{EntityID:uint64(m.EntityID),HP:m.HP,MaxHP:m.MaxHP,Defeated:m.Defeated,ReviveProtectionUntilTick:m.ReviveProtectionUntilTick})
	case protocol.SiegeMatchState:
		return json.Marshal(siegeMatchState{Revision:m.Revision,Round:m.Round,MatchID:m.MatchID,AttackerID:m.AttackerID,DefenderID:m.DefenderID,YourTeam:string(m.YourTeam),Phase:string(m.Phase),BreachGateID:m.BreachGateID,ThroneObjectiveID:m.ThroneObjectiveID,GateBreached:m.GateBreached,WinnerTeam:string(m.WinnerTeam),WinnerID:m.WinnerID,CastleOwnerID:m.CastleOwnerID})
	case protocol.CombatEvent:
		return json.Marshal(combatEvent{ActionInstanceID:m.ActionInstanceID,ActorEntityID:uint64(m.ActorEntityID),ActionID:m.ActionID,Result:string(m.Result),TargetEntityID:uint64(m.TargetEntityID),ImpactX:m.ImpactX,ImpactZ:m.ImpactZ,Damage:m.Damage,CooldownReadyTick:m.CooldownReadyTick})
	default:
		return nil, ErrUnsupportedMessage
	}
}
