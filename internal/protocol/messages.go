// Package protocol defines Astrahold protocol semantics independently from wire codec and transport.
package protocol

import (
	"encoding/hex"

	"github.com/li41/astrahold-server/internal/world"
)

// Version increments for wire-incompatible contracts or gameplay protocol semantics that would
// make old Client/Server pairs ambiguous.
// v27: Shadowblade adds authoritative source-target resource state for per-target Flaw.
// v26: Oathguard class action legality added wrong_class rejection plus authoritative class-resource state.
// v25: Reliable initial class selection intent/result plus authoritative CharacterClassState.
// v24: CombatEvent adds Server-authoritative shield block outcome; damage now reflects final mitigated damage.
// v23: Equipment semantics add authoritative off_hand so shields are distinct from MainHand weapons.
// v22: Reliable ItemUseResult returns authoritative consumable outcome/cooldown feedback.
// v21: Reliable ClientUseItem intent lets the Server authoritatively consume inventory items and restore HP/MP.
// v20: InventorySnapshot adds Server-authoritative current/max carry weight.
// v19: Reliable ClientRespawnRequest lets a defeated player request restart without reconnecting.
// v18: Server-authoritative NPC shop open/buy barter vertical slice.
// v17: Reliable ClientInteractNPC plus source-session authoritative NPCInteraction.
// v16: Server-owned item-drop lifecycle plus Reliable ClientPickupItem.
// v15: Reliable authoritative MainHand equipment intent/snapshot vertical slice.
// v14: Server emits Reliable InventorySnapshot.
// v13: EntityVitalsState adds authoritative MP/MaxMP and insufficient_resource rejection.
// v12: valid point-target ClientUseAction ingress semantics are compatibility-fenced.
// v11: Reliable ActionRejected returns authoritative action rejection reason.
const Version uint16 = 27

const MaxSnapshotEntitiesPerChunk = 43

type MessageType uint16

const (
	MessageUnknown            MessageType = 0
	MessageClientMoveInput    MessageType = 1
	MessageClientUseAction    MessageType = 2
	MessageSessionWelcome     MessageType = 10
	MessageEntitySpawn        MessageType = 100
	MessageEntityDespawn      MessageType = 101
	MessageWorldSnapshot      MessageType = 102
	MessagePositionCorrection MessageType = 103
	MessageWorldDynamicState  MessageType = 104
	MessageEntityVitalsState  MessageType = 105
	MessageSiegeMatchState    MessageType = 106
	MessageCombatEvent        MessageType = 107
	MessageActionStarted      MessageType = 108
	MessageActionRejected     MessageType = 109
)

type Delivery uint8

const (
	DeliveryUnknown Delivery = iota
	DeliveryReliableOrdered
	DeliveryRealtimeSequenced
)

type Message interface{ Type() MessageType }

type Envelope struct {
	Delivery   Delivery
	Sequence   uint32
	ServerTick uint64
	Message    Message
}

func (e Envelope) MessageType() MessageType {
	if e.Message == nil {
		return MessageUnknown
	}
	return e.Message.Type()
}

type WorldIdentity struct {
	WorldID        string
	Revision       string
	GameplaySHA256 string
}

func (w WorldIdentity) Valid() bool {
	if w.WorldID == "" || w.Revision == "" || len(w.GameplaySHA256) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(w.GameplaySHA256)
	return err == nil && len(decoded) == 32
}

type ClientMoveInput struct {
	DirectionX float32
	DirectionZ float32
}

func (ClientMoveInput) Type() MessageType { return MessageClientMoveInput }

type ActionTargetKind string

const (
	ActionTargetGate   ActionTargetKind = "gate"
	ActionTargetEntity ActionTargetKind = "entity"
	ActionTargetPoint  ActionTargetKind = "point"
)

type ClientUseAction struct {
	ActionID   string
	TargetKind ActionTargetKind
	TargetID   string
	TargetX    *float32
	TargetZ    *float32
}

func (ClientUseAction) Type() MessageType { return MessageClientUseAction }

// ActionStarted means the Server accepted the action far enough to consume gameplay
// execution/cooldown/resource cost. CombatEvent and EntityVitalsState remain outcome/vitals truth.
type ActionStarted struct {
	ActionInstanceID uint64
	ActorEntityID    world.EntityID
	ActionID         string
	TargetKind       ActionTargetKind
	TargetID         string
	TargetX          *float32
	TargetZ          *float32
}

func (ActionStarted) Type() MessageType { return MessageActionStarted }

type ActionRejectionReason string

const (
	ActionRejectionCooldown             ActionRejectionReason = "cooldown"
	ActionRejectionInsufficientResource ActionRejectionReason = "insufficient_resource"
	ActionRejectionInvalidTarget        ActionRejectionReason = "invalid_target"
	ActionRejectionOutOfRange           ActionRejectionReason = "out_of_range"
	ActionRejectionWrongLayer           ActionRejectionReason = "wrong_layer"
	ActionRejectionLineOfSight          ActionRejectionReason = "line_of_sight"
	ActionRejectionDefeated             ActionRejectionReason = "defeated"
	ActionRejectionReviveProtected      ActionRejectionReason = "revive_protected"
	ActionRejectionUnknownAction        ActionRejectionReason = "unknown_action"
	ActionRejectionServerRejected       ActionRejectionReason = "server_rejected"
)

type ActionRejected struct {
	ClientActionSequence uint32
	ActorEntityID        world.EntityID
	ActionID             string
	TargetKind           ActionTargetKind
	Reason               ActionRejectionReason
	CooldownReadyTick    uint64
}

func (ActionRejected) Type() MessageType { return MessageActionRejected }

type SessionWelcome struct {
	SessionID      uint64
	EntityID       world.EntityID
	RealtimePort   uint16
	RealtimeToken  string
	TickRateHz     uint16
	SnapshotRateHz uint16
	World          WorldIdentity
}

func (SessionWelcome) Type() MessageType { return MessageSessionWelcome }

type EntityTransform struct {
	EntityID world.EntityID
	Tick     uint64
	Position world.Position
	Yaw      float32
}

type EntitySpawn struct {
	EntityID    world.EntityID
	Kind        world.EntityKind
	Transform   EntityTransform
	ArchetypeID string
}

func (EntitySpawn) Type() MessageType { return MessageEntitySpawn }

type EntityDespawn struct{ EntityID world.EntityID }

func (EntityDespawn) Type() MessageType { return MessageEntityDespawn }

type WorldSnapshot struct {
	Tick       uint64
	ChunkIndex uint16
	ChunkCount uint16
	Entities   []EntityTransform
}

func (WorldSnapshot) Type() MessageType { return MessageWorldSnapshot }

func (s WorldSnapshot) ValidChunk() bool {
	return s.ChunkCount > 0 && s.ChunkIndex < s.ChunkCount && len(s.Entities) <= MaxSnapshotEntitiesPerChunk
}

type PositionCorrection struct {
	Tick                       uint64
	EntityID                   world.EntityID
	Position                   world.Position
	Yaw                        float32
	LastProcessedInputSequence uint32
}

func (PositionCorrection) Type() MessageType { return MessagePositionCorrection }

type WorldBlockerState struct {
	ID      string
	Enabled bool
}

type WorldGateState struct {
	ID        string
	HP        uint32
	MaxHP     uint32
	Destroyed bool
}

type WorldDynamicState struct {
	Revision uint64
	Blockers []WorldBlockerState
	Gates    []WorldGateState
}

func (WorldDynamicState) Type() MessageType { return MessageWorldDynamicState }

// EntityVitalsState is complete resendable combatant vitals truth.
type EntityVitalsState struct {
	EntityID                  world.EntityID
	HP                        uint32
	MaxHP                     uint32
	MP                        uint32
	MaxMP                     uint32
	Defeated                  bool
	ReviveProtectionUntilTick uint64
}

func (EntityVitalsState) Type() MessageType { return MessageEntityVitalsState }

type CombatEventResult string

const (
	CombatEventHit       CombatEventResult = "hit"
	CombatEventMiss      CombatEventResult = "miss"
	CombatEventResurrect CombatEventResult = "resurrect"
)

// CombatEvent is the Server-resolved presentation outcome. EntityVitalsState remains HP/MP truth.
// Damage is the final mitigated damage for this instance. Blocked is true only when the Server
// performed one eligible shield block roll and it succeeded. Clients must not reroll or recompute it.
type CombatEvent struct {
	ActionInstanceID  uint64
	ActorEntityID     world.EntityID
	ActionID          string
	Result            CombatEventResult
	TargetEntityID    world.EntityID
	ImpactX           *float32
	ImpactZ           *float32
	Damage            uint32
	Blocked           bool
	CooldownReadyTick uint64
}

func (CombatEvent) Type() MessageType { return MessageCombatEvent }

type SiegeTeam string

const (
	SiegeTeamUnknown  SiegeTeam = "unknown"
	SiegeTeamAttacker SiegeTeam = "attacker"
	SiegeTeamDefender SiegeTeam = "defender"
)

type SiegePhase string

const (
	SiegePhaseUnknown   SiegePhase = "unknown"
	SiegePhaseGate      SiegePhase = "gate"
	SiegePhaseThrone    SiegePhase = "throne"
	SiegePhaseCompleted SiegePhase = "completed"
)

type SiegeMatchState struct {
	Revision          uint64
	Round             uint64
	MatchID           string
	AttackerID        string
	DefenderID        string
	YourTeam          SiegeTeam
	Phase             SiegePhase
	BreachGateID      string
	ThroneObjectiveID string
	GateBreached      bool
	WinnerTeam        SiegeTeam
	WinnerID          string
	CastleOwnerID     string
}

func (SiegeMatchState) Type() MessageType { return MessageSiegeMatchState }
