// Package gamev1 提供 Protocol v6 使用的混合 Payload Codec。
//
// Reliable control message 仍委派給 jsonv1，方便 Godot Thin Client 開發；
// Realtime movement / snapshot / correction 則使用固定欄位 binary，降低 payload 與 allocation。
package gamev1

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/codec/jsonv1"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	clientMovePayloadSize         = 8
	worldSnapshotHeaderSize       = 14
	worldSnapshotTransformSize    = 26
	positionCorrectionPayloadSize = 38
)

var (
	ErrInvalidPayload       = errors.New("gamev1: invalid payload")
	ErrInvalidSnapshotChunk = errors.New("gamev1: invalid snapshot chunk")
)

type Codec struct {
	json jsonv1.Codec
}

func (c Codec) Marshal(message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ClientMoveInput:
		return marshalMove(m), nil
	case *protocol.ClientMoveInput:
		if m == nil {
			return nil, ErrInvalidPayload
		}
		return marshalMove(*m), nil
	case protocol.WorldSnapshot:
		return marshalSnapshot(m)
	case *protocol.WorldSnapshot:
		if m == nil {
			return nil, ErrInvalidPayload
		}
		return marshalSnapshot(*m)
	case protocol.PositionCorrection:
		return marshalCorrection(m), nil
	case *protocol.PositionCorrection:
		if m == nil {
			return nil, ErrInvalidPayload
		}
		return marshalCorrection(*m), nil
	default:
		return c.json.Marshal(message)
	}
}

// AppendMarshal 讓 transport 可以直接把 realtime payload 寫進 reusable frame/datagram buffer。
// wire layout 與 Marshal 完全一致；Reliable JSON fallback 仍保留既有 codec contract。
func (c Codec) AppendMarshal(dst []byte, message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ClientMoveInput:
		return appendMove(dst, m), nil
	case *protocol.ClientMoveInput:
		if m == nil {
			return dst, ErrInvalidPayload
		}
		return appendMove(dst, *m), nil
	case protocol.WorldSnapshot:
		return appendSnapshot(dst, m)
	case *protocol.WorldSnapshot:
		if m == nil {
			return dst, ErrInvalidPayload
		}
		return appendSnapshot(dst, *m)
	case protocol.PositionCorrection:
		return appendCorrection(dst, m), nil
	case *protocol.PositionCorrection:
		if m == nil {
			return dst, ErrInvalidPayload
		}
		return appendCorrection(dst, *m), nil
	default:
		payload, err := c.json.Marshal(message)
		if err != nil {
			return dst, err
		}
		return append(dst, payload...), nil
	}
}

func (c Codec) Unmarshal(messageType protocol.MessageType, data []byte) (protocol.Message, error) {
	switch messageType {
	case protocol.MessageClientMoveInput:
		if len(data) != clientMovePayloadSize {
			return nil, ErrInvalidPayload
		}
		return protocol.ClientMoveInput{
			DirectionX: readFloat32(data[0:4]),
			DirectionZ: readFloat32(data[4:8]),
		}, nil
	case protocol.MessageWorldSnapshot:
		return unmarshalSnapshot(data)
	case protocol.MessagePositionCorrection:
		return unmarshalCorrection(data)
	default:
		return c.json.Unmarshal(messageType, data)
	}
}

func marshalMove(message protocol.ClientMoveInput) []byte {
	out := make([]byte, clientMovePayloadSize)
	writeFloat32(out[0:4], message.DirectionX)
	writeFloat32(out[4:8], message.DirectionZ)
	return out
}

func appendMove(dst []byte, message protocol.ClientMoveInput) []byte {
	start := len(dst)
	dst = growPayload(dst, clientMovePayloadSize)
	out := dst[start:]
	writeFloat32(out[0:4], message.DirectionX)
	writeFloat32(out[4:8], message.DirectionZ)
	return dst
}

func marshalSnapshot(message protocol.WorldSnapshot) ([]byte, error) {
	if !message.ValidChunk() {
		return nil, ErrInvalidSnapshotChunk
	}
	out := make([]byte, worldSnapshotHeaderSize+len(message.Entities)*worldSnapshotTransformSize)
	writeSnapshot(out, message)
	return out, nil
}

func appendSnapshot(dst []byte, message protocol.WorldSnapshot) ([]byte, error) {
	if !message.ValidChunk() {
		return dst, ErrInvalidSnapshotChunk
	}
	start := len(dst)
	dst = growPayload(dst, worldSnapshotHeaderSize+len(message.Entities)*worldSnapshotTransformSize)
	writeSnapshot(dst[start:], message)
	return dst, nil
}

func writeSnapshot(out []byte, message protocol.WorldSnapshot) {
	binary.BigEndian.PutUint64(out[0:8], message.Tick)
	binary.BigEndian.PutUint16(out[8:10], message.ChunkIndex)
	binary.BigEndian.PutUint16(out[10:12], message.ChunkCount)
	binary.BigEndian.PutUint16(out[12:14], uint16(len(message.Entities)))

	offset := worldSnapshotHeaderSize
	for _, transform := range message.Entities {
		binary.BigEndian.PutUint64(out[offset:offset+8], uint64(transform.EntityID))
		writeFloat32(out[offset+8:offset+12], transform.Position.X)
		writeFloat32(out[offset+12:offset+16], transform.Position.Y)
		writeFloat32(out[offset+16:offset+20], transform.Position.Z)
		writeFloat32(out[offset+20:offset+24], transform.Yaw)
		binary.BigEndian.PutUint16(out[offset+24:offset+26], uint16(transform.Position.Layer))
		offset += worldSnapshotTransformSize
	}
}

func unmarshalSnapshot(data []byte) (protocol.Message, error) {
	if len(data) < worldSnapshotHeaderSize {
		return nil, ErrInvalidPayload
	}
	tick := binary.BigEndian.Uint64(data[0:8])
	chunkIndex := binary.BigEndian.Uint16(data[8:10])
	chunkCount := binary.BigEndian.Uint16(data[10:12])
	entityCount := int(binary.BigEndian.Uint16(data[12:14]))
	if entityCount > protocol.MaxSnapshotEntitiesPerChunk || len(data) != worldSnapshotHeaderSize+entityCount*worldSnapshotTransformSize {
		return nil, ErrInvalidPayload
	}

	entities := make([]protocol.EntityTransform, entityCount)
	offset := worldSnapshotHeaderSize
	for i := range entities {
		entities[i] = protocol.EntityTransform{
			EntityID: world.EntityID(binary.BigEndian.Uint64(data[offset : offset+8])),
			Tick:     tick,
			Position: world.Position{
				X:     readFloat32(data[offset+8 : offset+12]),
				Y:     readFloat32(data[offset+12 : offset+16]),
				Z:     readFloat32(data[offset+16 : offset+20]),
				Layer: world.LayerID(binary.BigEndian.Uint16(data[offset+24 : offset+26])),
			},
			Yaw: readFloat32(data[offset+20 : offset+24]),
		}
		offset += worldSnapshotTransformSize
	}
	message := protocol.WorldSnapshot{Tick: tick, ChunkIndex: chunkIndex, ChunkCount: chunkCount, Entities: entities}
	if !message.ValidChunk() {
		return nil, ErrInvalidSnapshotChunk
	}
	return message, nil
}

func marshalCorrection(message protocol.PositionCorrection) []byte {
	out := make([]byte, positionCorrectionPayloadSize)
	writeCorrection(out, message)
	return out
}

func appendCorrection(dst []byte, message protocol.PositionCorrection) []byte {
	start := len(dst)
	dst = growPayload(dst, positionCorrectionPayloadSize)
	writeCorrection(dst[start:], message)
	return dst
}

func writeCorrection(out []byte, message protocol.PositionCorrection) {
	binary.BigEndian.PutUint64(out[0:8], message.Tick)
	binary.BigEndian.PutUint64(out[8:16], uint64(message.EntityID))
	writeFloat32(out[16:20], message.Position.X)
	writeFloat32(out[20:24], message.Position.Y)
	writeFloat32(out[24:28], message.Position.Z)
	writeFloat32(out[28:32], message.Yaw)
	binary.BigEndian.PutUint16(out[32:34], uint16(message.Position.Layer))
	binary.BigEndian.PutUint32(out[34:38], message.LastProcessedInputSequence)
}

func unmarshalCorrection(data []byte) (protocol.Message, error) {
	if len(data) != positionCorrectionPayloadSize {
		return nil, ErrInvalidPayload
	}
	return protocol.PositionCorrection{
		Tick:     binary.BigEndian.Uint64(data[0:8]),
		EntityID: world.EntityID(binary.BigEndian.Uint64(data[8:16])),
		Position: world.Position{
			X:     readFloat32(data[16:20]),
			Y:     readFloat32(data[20:24]),
			Z:     readFloat32(data[24:28]),
			Layer: world.LayerID(binary.BigEndian.Uint16(data[32:34])),
		},
		Yaw:                        readFloat32(data[28:32]),
		LastProcessedInputSequence: binary.BigEndian.Uint32(data[34:38]),
	}, nil
}

func growPayload(dst []byte, count int) []byte {
	if count <= 0 {
		return dst
	}
	start := len(dst)
	if count <= cap(dst)-start {
		dst = dst[:start+count]
		clear(dst[start:])
		return dst
	}
	return append(dst, make([]byte, count)...)
}

func writeFloat32(target []byte, value float32) {
	binary.BigEndian.PutUint32(target, math.Float32bits(value))
}

func readFloat32(source []byte) float32 {
	return math.Float32frombits(binary.BigEndian.Uint32(source))
}
