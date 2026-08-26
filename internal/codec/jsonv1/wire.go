package jsonv1

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

type position struct { X float32 `json:"x"`; Y float32 `json:"y"`; Z float32 `json:"z"`; Layer uint16 `json:"layer"` }
type entityTransform struct { EntityID uint64 `json:"entity_id"`; Tick uint64 `json:"tick"`; Position position `json:"position"`; Yaw float32 `json:"yaw"` }
type clientMoveInput struct { DX float32 `json:"dx"`; DZ float32 `json:"dz"` }
type clientUseAction struct { ActionID string `json:"action_id"`; TargetKind string `json:"target_kind"`; TargetID string `json:"target_id"`; TargetX *float32 `json:"target_x,omitempty"`; TargetZ *float32 `json:"target_z,omitempty"` }
type actionStarted struct { ActionInstanceID uint64 `json:"action_instance_id"`; ActorEntityID uint64 `json:"actor_entity_id"`; ActionID string `json:"action_id"`; TargetKind string `json:"target_kind"`; TargetID string `json:"target_id"`; TargetX *float32 `json:"target_x,omitempty"`; TargetZ *float32 `json:"target_z,omitempty"` }
type sessionWelcome struct { SessionID uint64 `json:"session_id"`; EntityID uint64 `json:"entity_id"`; RealtimePort uint16 `json:"realtime_port"`; RealtimeToken string `json:"realtime_token"`; TickRateHz uint16 `json:"tick_rate_hz"`; SnapshotRateHz uint16 `json:"snapshot_rate_hz"`; WorldID string `json:"world_id"`; WorldRevision string `json:"world_revision"`; GameplaySHA256 string `json:"gameplay_sha256"` }
type entitySpawn struct { EntityID uint64 `json:"entity_id"`; Kind uint8 `json:"kind"`; Transform entityTransform `json:"transform"`; ArchetypeID string `json:"archetype_id,omitempty"` }
type entityDespawn struct { EntityID uint64 `json:"entity_id"` }
type worldSnapshot struct { Tick uint64 `json:"tick"`; Entities []entityTransform `json:"entities"` }
type positionCorrection struct { Tick uint64 `json:"tick"`; EntityID uint64 `json:"entity_id"`; Position position `json:"position"`; Yaw float32 `json:"yaw"`; LastProcessedInputSequence uint32 `json:"last_processed_input_sequence"` }
type worldBlockerState struct { ID string `json:"id"`; Enabled bool `json:"enabled"` }
type worldGateState struct { ID string `json:"id"`; HP uint32 `json:"hp"`; MaxHP uint32 `json:"max_hp"`; Destroyed bool `json:"destroyed"` }
type worldDynamicState struct { Revision uint64 `json:"revision"`; Blockers []worldBlockerState `json:"blockers"`; Gates []worldGateState `json:"gates"` }
type entityVitalsState struct { EntityID uint64 `json:"entity_id"`; HP uint32 `json:"hp"`; MaxHP uint32 `json:"max_hp"`; Defeated bool `json:"defeated"` }
type combatEvent struct {
	ActionInstanceID uint64   `json:"action_instance_id"`
	ActorEntityID    uint64   `json:"actor_entity_id"`
	ActionID         string   `json:"action_id"`
	Result           string   `json:"result"`
	TargetEntityID   uint64   `json:"target_entity_id"`
	ImpactX          *float32 `json:"impact_x,omitempty"`
	ImpactZ          *float32 `json:"impact_z,omitempty"`
	Damage           uint32   `json:"damage"`
	CooldownReadyTick uint64  `json:"cooldown_ready_tick,omitempty"`
}
type siegeMatchState struct {
	Revision          uint64 `json:"revision"`
	Round             uint64 `json:"round"`
	MatchID           string `json:"match_id"`
	AttackerID        string `json:"attacker_id"`
	DefenderID        string `json:"defender_id"`
	YourTeam          string `json:"your_team"`
	Phase             string `json:"phase"`
	BreachGateID      string `json:"breach_gate_id"`
	ThroneObjectiveID string `json:"throne_objective_id"`
	GateBreached      bool   `json:"gate_breached"`
	WinnerTeam        string `json:"winner_team"`
	WinnerID          string `json:"winner_id"`
	CastleOwnerID     string `json:"castle_owner_id"`
}

func toPosition(p world.Position) position { return position{X:p.X,Y:p.Y,Z:p.Z,Layer:uint16(p.Layer)} }
func fromPosition(p position) world.Position { return world.Position{X:p.X,Y:p.Y,Z:p.Z,Layer:world.LayerID(p.Layer)} }
func toEntityTransform(t protocol.EntityTransform) entityTransform { return entityTransform{EntityID:uint64(t.EntityID),Tick:t.Tick,Position:toPosition(t.Position),Yaw:t.Yaw} }
func fromEntityTransform(t entityTransform) protocol.EntityTransform { return protocol.EntityTransform{EntityID:world.EntityID(t.EntityID),Tick:t.Tick,Position:fromPosition(t.Position),Yaw:t.Yaw} }
func toEntitySpawn(s protocol.EntitySpawn) entitySpawn { return entitySpawn{EntityID:uint64(s.EntityID),Kind:uint8(s.Kind),Transform:toEntityTransform(s.Transform),ArchetypeID:s.ArchetypeID} }
