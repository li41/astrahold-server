// Package protocol 定義 Astrahold 自有協定的語意層；不綁定特定 wire codec 或 transport。
package protocol

import (
	"encoding/hex"

	"github.com/li41/astrahold-server/internal/world"
)

// Version 在 wire-incompatible contract 或會造成舊 Client/Server 行為歧義的 gameplay protocol 語意變更時必須遞增。
// v15: Reliable authoritative MainHand equipment intent/snapshot vertical slice.
// v14: Server production emits Reliable InventorySnapshot and the Unreal client decodes message 110 as authoritative inventory truth.
// v13: EntityVitalsState 新增 MP/MaxMP authoritative resource truth，並新增 insufficient_resource action rejection。
// v12: valid point-target ClientUseAction ingress semantics 納入 compatibility fence；舊版會把合法 point intent
// 當 malformed transport message關閉連線，不能再與新 Client 成功握手後延遲到第一次施法才失敗。
// v11: 新增 Reliable ActionRejected，讓 Server 對已處理的 action intent 明確回覆 authoritative rejection reason。
// ActionStarted 仍只代表 Server accepted；CombatEvent / EntityVitalsState 仍分別是 resolved outcome / vitals truth。
const Version uint16 = 15

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

// TargetX/TargetZ are present only for point-target actions. Entity/gate callers keep using TargetID.
type ClientUseAction struct {
	ActionID   string
	TargetKind ActionTargetKind
	TargetID   string
	TargetX    *float32
	TargetZ    *float32
}

func (ClientUseAction) Type() MessageType { return MessageClientUseAction }

// ActionStarted means the Server has accepted the action far enough that it will consume gameplay
// execution/cooldown/resource cost. Target fields preserve the accepted target spec, not the later resolved hit target.
// CombatEvent / EntityVitalsState remain the outcome and vitals truth respectively.
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

// ActionRejected is source-session-only authoritative legality feedback. ClientActionSequence is
// the Reliable ClientUseAction envelope sequence already consumed by the Server; clients must not
// invent Reason when this message is absent. CooldownReadyTick is supplied only when the Server
// has an authoritative ready tick for a cooldown rejection.
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

// ArchetypeID is stable content/presentation identity only; gameplay authority remains server-side.
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

// EntityVitalsState 是單一 combatant 完整、可重送的 Reliable vitals snapshot。
// HP/MP 均是 Server truth；Client 不得由 CombatEvent damage 或 local skill cost 自行推導。
// ReviveProtectionUntilTick=0 表示目前沒有 Server-authoritative revive protection；非 0 時
// Client 只能以 Server tick 顯示剩餘保護時間，不得自行延長、取消或決定 gameplay protection。
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

// CombatEvent 描述 Server 已 resolve 的 action outcome。EntityVitalsState 仍是 HP/MP truth；
// event 供 animation/VFX/audio 等 presentation 對齊 stable ActionInstanceID。
// CooldownReadyTick=0 表示沒有 additive cooldown metadata；非 0 時是 Server 會執行的 ready tick。
type CombatEvent struct {
	ActionInstanceID  uint64
	ActorEntityID     world.EntityID
	ActionID          string
	Result            CombatEventResult
	TargetEntityID    world.EntityID
	ImpactX           *float32
	ImpactZ           *float32
	Damage            uint32
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

// SiegeMatchState 是每個 Session 可重送的 Server-authoritative Siege view。
// YourTeam 只描述該 recipient 的 Server-owned team assignment；Client 不可自行推導或改寫 phase/winner/owner truth。
// Round 是 gameplay round identity；Revision 仍是 Reliable resend stamp，兩者不可互相替代。
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
