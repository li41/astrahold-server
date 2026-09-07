// Package protocol 定義 Astrahold 自有協定的語意層；不綁定特定 wire codec 或 transport。
package protocol

import (
	"encoding/hex"

	"github.com/li41/astrahold-server/internal/world"
)

// Version 在 wire-incompatible contract 或會造成舊 Client/Server 行為歧義的 gameplay protocol 語意變更時必須遞增。
// v23: Equipment semantics add authoritative off_hand so shields are distinct from MainHand weapons.
// v22: Reliable ItemUseResult returns authoritative consumable outcome/cooldown feedback; Server owns shared potion cooldown legality.
// v21: Reliable ClientUseItem intent lets the Server authoritatively consume inventory items and restore HP/MP.
// v20: InventorySnapshot adds Server-authoritative current/max carry weight so clients present capacity without reimplementing item-weight gameplay rules.
// v19: Reliable ClientRespawnRequest lets a defeated player request restart without reconnecting; Server retains respawn destination/timing authority.
// v18: Server-authoritative NPC shop open/buy barter vertical slice.
// v17: Reliable ClientInteractNPC intent plus source-session authoritative NPCInteraction dialogue response.
// v16: Server-owned item-drop entity lifecycle plus Reliable ClientPickupItem intent.
// v15: Reliable authoritative MainHand equipment intent/snapshot vertical slice.
// v14: Server production emits Reliable InventorySnapshot and the Unreal client decodes message 110 as authoritative inventory truth.
// v13: EntityVitalsState 新增 MP/MaxMP authoritative resource truth，並新增 insufficient_resource action rejection。
// v12: valid point-target ClientUseAction ingress semantics 納入 compatibility fence；舊版會把合法 point intent
// 當 malformed transport message關閉連線，不能再與新 Client 成功握手後延遲到第一次施法才失敗。
// v11: 新增 Reliable ActionRejected，讓 Server 對已處理的 action intent 明確回覆 authoritative rejection reason。
// ActionStarted 仍只代表 Server accepted；CombatEvent / EntityVitalsState 仍分別是 resolved outcome / vitals truth。
const Version uint16 = 23

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
