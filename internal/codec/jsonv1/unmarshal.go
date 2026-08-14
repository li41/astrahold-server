package jsonv1

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func (Codec) Unmarshal(messageType protocol.MessageType, data []byte) (protocol.Message, error) {
	switch messageType {
	case protocol.MessageClientMoveInput:
		var in clientMoveInput
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.ClientMoveInput{DirectionX:in.DX,DirectionZ:in.DZ},nil
	case protocol.MessageClientUseAction:
		var in clientUseAction
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.ClientUseAction{ActionID:in.ActionID,TargetKind:protocol.ActionTargetKind(in.TargetKind),TargetID:in.TargetID},nil
	case protocol.MessageSessionWelcome:
		var in sessionWelcome
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.SessionWelcome{SessionID:in.SessionID,EntityID:world.EntityID(in.EntityID),RealtimePort:in.RealtimePort,RealtimeToken:in.RealtimeToken,TickRateHz:in.TickRateHz,SnapshotRateHz:in.SnapshotRateHz,World:protocol.WorldIdentity{WorldID:in.WorldID,Revision:in.WorldRevision,GameplaySHA256:in.GameplaySHA256}},nil
	case protocol.MessageEntitySpawn:
		var in entitySpawn
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.EntitySpawn{EntityID:world.EntityID(in.EntityID),Kind:world.EntityKind(in.Kind),Transform:fromEntityTransform(in.Transform)},nil
	case protocol.MessageEntityDespawn:
		var in entityDespawn
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.EntityDespawn{EntityID:world.EntityID(in.EntityID)},nil
	case protocol.MessageWorldSnapshot:
		var in worldSnapshot
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		entities:=make([]protocol.EntityTransform,len(in.Entities));for i:=range in.Entities{entities[i]=fromEntityTransform(in.Entities[i])}
		return protocol.WorldSnapshot{Tick:in.Tick,ChunkIndex:0,ChunkCount:1,Entities:entities},nil
	case protocol.MessagePositionCorrection:
		var in positionCorrection
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.PositionCorrection{Tick:in.Tick,EntityID:world.EntityID(in.EntityID),Position:fromPosition(in.Position),Yaw:in.Yaw,LastProcessedInputSequence:in.LastProcessedInputSequence},nil
	case protocol.MessageWorldDynamicState:
		var in worldDynamicState
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		blockers:=make([]protocol.WorldBlockerState,len(in.Blockers));for i,b:=range in.Blockers{blockers[i]=protocol.WorldBlockerState{ID:b.ID,Enabled:b.Enabled}}
		gates:=make([]protocol.WorldGateState,len(in.Gates));for i,g:=range in.Gates{gates[i]=protocol.WorldGateState{ID:g.ID,HP:g.HP,MaxHP:g.MaxHP,Destroyed:g.Destroyed}}
		return protocol.WorldDynamicState{Revision:in.Revision,Blockers:blockers,Gates:gates},nil
	case protocol.MessageEntityVitalsState:
		var in entityVitalsState
		if err:=decodeStrict(data,&in);err!=nil{return nil,err}
		return protocol.EntityVitalsState{EntityID:world.EntityID(in.EntityID),HP:in.HP,MaxHP:in.MaxHP,Defeated:in.Defeated},nil
	default:
		return nil, ErrUnsupportedMessage
	}
}
