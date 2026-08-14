// Package gamev2 在既有 gamev1 wire 上增加 Protocol v6 EntityVitalsState。
package gamev2

import (
	"github.com/li41/astrahold-server/internal/codec/gamev1"
	"github.com/li41/astrahold-server/internal/protocol"
)

type Codec struct {
	base gamev1.Codec
}

func (c Codec) Marshal(message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.EntityVitalsState:
		return marshalVitals(m), nil
	case *protocol.EntityVitalsState:
		if m == nil { return nil, ErrInvalidVitalsPayload }
		return marshalVitals(*m), nil
	default:
		return c.base.Marshal(message)
	}
}

func (c Codec) Unmarshal(messageType protocol.MessageType, data []byte) (protocol.Message, error) {
	if messageType == protocol.MessageEntityVitalsState {
		return unmarshalVitals(data)
	}
	return c.base.Unmarshal(messageType, data)
}
