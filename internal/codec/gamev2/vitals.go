package gamev2

import (
	"encoding/binary"
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

const entityVitalsPayloadSize = 17

var ErrInvalidVitalsPayload = errors.New("gamev2: invalid entity vitals payload")

func marshalVitals(message protocol.EntityVitalsState) []byte {
	out := make([]byte, entityVitalsPayloadSize)
	binary.BigEndian.PutUint64(out[0:8], uint64(message.EntityID))
	binary.BigEndian.PutUint32(out[8:12], message.HP)
	binary.BigEndian.PutUint32(out[12:16], message.MaxHP)
	if message.Defeated { out[16] = 1 }
	return out
}

func unmarshalVitals(data []byte) (protocol.Message, error) {
	if len(data) != entityVitalsPayloadSize || data[16] > 1 {
		return nil, ErrInvalidVitalsPayload
	}
	return protocol.EntityVitalsState{
		EntityID: world.EntityID(binary.BigEndian.Uint64(data[0:8])),
		HP: binary.BigEndian.Uint32(data[8:12]),
		MaxHP: binary.BigEndian.Uint32(data[12:16]),
		Defeated: data[16] == 1,
	}, nil
}
