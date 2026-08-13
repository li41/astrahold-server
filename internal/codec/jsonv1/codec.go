// Package jsonv1 提供 S2 Godot Thin Client 用的開發橋接 Payload Codec。
// 它是可替換 adapter，不是 Simulation/Runtime 的依賴，也不是最終商用 wire format 承諾。
package jsonv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrUnsupportedMessage = errors.New("jsonv1: unsupported message")
	ErrTrailingData       = errors.New("jsonv1: trailing data")
)

type Codec struct{}

type position struct {
	X     float32 `json:"x"`
	Y     float32 `json:"y"`
	Z     float32 `json:"z"`
	Layer uint16  `json:"layer"`
}

type entityTransform struct {
	EntityID uint64   `json:"entity_id"`
	Tick     uint64   `json:"tick"`
	Position position `json:"position"`
	Yaw      float32  `json:"yaw"`
}

type clientMoveInput struct {
	DX float32 `json:"dx"`
	DZ float32 `json:"dz"`
}

type sessionWelcome struct {
	SessionID      uint64 `json:"session_id"`
	EntityID       uint64 `json:"entity_id"`
	RealtimePort   uint16 `json:"realtime_port"`
	RealtimeToken  string `json:"realtime_token"`
	TickRateHz     uint16 `json:"tick_rate_hz"`
	SnapshotRateHz uint16 `json:"snapshot_rate_hz"`
}

type entitySpawn struct {
	EntityID  uint64          `json:"entity_id"`
	Kind      uint8           `json:"kind"`
	Transform entityTransform `json:"transform"`
}

type entityDespawn struct {
	EntityID uint64 `json:"entity_id"`
}

type worldSnapshot struct {
	Tick     uint64            `json:"tick"`
	Entities []entityTransform `json:"entities"`
}

type positionCorrection struct {
	Tick                       uint64   `json:"tick"`
	EntityID                   uint64   `json:"entity_id"`
	Position                   position `json:"position"`
	Yaw                        float32  `json:"yaw"`
	LastProcessedInputSequence uint32   `json:"last_processed_input_sequence"`
}

func (Codec) Marshal(message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ClientMoveInput:
		return json.Marshal(clientMoveInput{DX: m.DirectionX, DZ: m.DirectionZ})
	case *protocol.ClientMoveInput:
		if m == nil {
			return nil, ErrUnsupportedMessage
		}
		return json.Marshal(clientMoveInput{DX: m.DirectionX, DZ: m.DirectionZ})
	case protocol.SessionWelcome:
		return json.Marshal(sessionWelcome{
			SessionID:      m.SessionID,
			EntityID:       uint64(m.EntityID),
			RealtimePort:   m.RealtimePort,
			RealtimeToken:  m.RealtimeToken,
			TickRateHz:     m.TickRateHz,
			SnapshotRateHz: m.SnapshotRateHz,
		})
	case protocol.EntitySpawn:
		return json.Marshal(toEntitySpawn(m))
	case protocol.EntityDespawn:
		return json.Marshal(entityDespawn{EntityID: uint64(m.EntityID)})
	case protocol.WorldSnapshot:
		out := worldSnapshot{Tick: m.Tick, Entities: make([]entityTransform, len(m.Entities))}
		for i := range m.Entities {
			out.Entities[i] = toEntityTransform(m.Entities[i])
		}
		return json.Marshal(out)
	case protocol.PositionCorrection:
		return json.Marshal(positionCorrection{
			Tick:                       m.Tick,
			EntityID:                   uint64(m.EntityID),
			Position:                   toPosition(m.Position),
			Yaw:                        m.Yaw,
			LastProcessedInputSequence: m.LastProcessedInputSequence,
		})
	default:
		return nil, ErrUnsupportedMessage
	}
}

func (Codec) Unmarshal(messageType protocol.MessageType, data []byte) (protocol.Message, error) {
	switch messageType {
	case protocol.MessageClientMoveInput:
		var in clientMoveInput
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		return protocol.ClientMoveInput{DirectionX: in.DX, DirectionZ: in.DZ}, nil
	case protocol.MessageSessionWelcome:
		var in sessionWelcome
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		return protocol.SessionWelcome{
			SessionID:      in.SessionID,
			EntityID:       world.EntityID(in.EntityID),
			RealtimePort:   in.RealtimePort,
			RealtimeToken:  in.RealtimeToken,
			TickRateHz:     in.TickRateHz,
			SnapshotRateHz: in.SnapshotRateHz,
		}, nil
	case protocol.MessageEntitySpawn:
		var in entitySpawn
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		return protocol.EntitySpawn{
			EntityID:  world.EntityID(in.EntityID),
			Kind:      world.EntityKind(in.Kind),
			Transform: fromEntityTransform(in.Transform),
		}, nil
	case protocol.MessageEntityDespawn:
		var in entityDespawn
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		return protocol.EntityDespawn{EntityID: world.EntityID(in.EntityID)}, nil
	case protocol.MessageWorldSnapshot:
		var in worldSnapshot
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		entities := make([]protocol.EntityTransform, len(in.Entities))
		for i := range in.Entities {
			entities[i] = fromEntityTransform(in.Entities[i])
		}
		return protocol.WorldSnapshot{Tick: in.Tick, Entities: entities}, nil
	case protocol.MessagePositionCorrection:
		var in positionCorrection
		if err := decodeStrict(data, &in); err != nil {
			return nil, err
		}
		return protocol.PositionCorrection{
			Tick:                       in.Tick,
			EntityID:                   world.EntityID(in.EntityID),
			Position:                   fromPosition(in.Position),
			Yaw:                        in.Yaw,
			LastProcessedInputSequence: in.LastProcessedInputSequence,
		}, nil
	default:
		return nil, ErrUnsupportedMessage
	}
}

func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return ErrTrailingData
		}
		return err
	}
	return nil
}

func toPosition(p world.Position) position {
	return position{X: p.X, Y: p.Y, Z: p.Z, Layer: uint16(p.Layer)}
}

func fromPosition(p position) world.Position {
	return world.Position{X: p.X, Y: p.Y, Z: p.Z, Layer: world.LayerID(p.Layer)}
}

func toEntityTransform(t protocol.EntityTransform) entityTransform {
	return entityTransform{EntityID: uint64(t.EntityID), Tick: t.Tick, Position: toPosition(t.Position), Yaw: t.Yaw}
}

func fromEntityTransform(t entityTransform) protocol.EntityTransform {
	return protocol.EntityTransform{EntityID: world.EntityID(t.EntityID), Tick: t.Tick, Position: fromPosition(t.Position), Yaw: t.Yaw}
}

func toEntitySpawn(s protocol.EntitySpawn) entitySpawn {
	return entitySpawn{EntityID: uint64(s.EntityID), Kind: uint8(s.Kind), Transform: toEntityTransform(s.Transform)}
}
