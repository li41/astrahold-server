package gamev1

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestClientRespawnRequestUsesEmptyPayload(t *testing.T) {
	codec := Codec{}
	payload, err := codec.Marshal(protocol.ClientRespawnRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) != 0 {
		t.Fatalf("payload length=%d want=0", len(payload))
	}
	message, err := codec.Unmarshal(protocol.MessageClientRespawnRequest, payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := message.(protocol.ClientRespawnRequest); !ok {
		t.Fatalf("message=%T want protocol.ClientRespawnRequest", message)
	}
}

func TestClientRespawnRequestRejectsUnexpectedPayload(t *testing.T) {
	codec := Codec{}
	if _, err := codec.Unmarshal(protocol.MessageClientRespawnRequest, []byte{1}); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("err=%v want ErrInvalidPayload", err)
	}
	var request *protocol.ClientRespawnRequest
	if _, err := codec.Marshal(request); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("nil pointer err=%v want ErrInvalidPayload", err)
	}
}
