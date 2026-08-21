package jsonv1

import (
	"bytes"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestPointActionRoundTripAndLegacyActionShape(t *testing.T) {
	codec := Codec{}
	x, z := float32(3.25), float32(-7.5)
	payload, err := codec.Marshal(protocol.ClientUseAction{
		ActionID: "fireball", TargetKind: protocol.ActionTargetPoint, TargetX: &x, TargetZ: &z,
	})
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(payload, []byte(`"target_x":3.25`)) || !bytes.Contains(payload, []byte(`"target_z":-7.5`)) {
		t.Fatalf("point payload=%s", payload)
	}
	decodedMessage, err := codec.Unmarshal(protocol.MessageClientUseAction, payload)
	if err != nil { t.Fatal(err) }
	decoded := decodedMessage.(protocol.ClientUseAction)
	if decoded.TargetKind != protocol.ActionTargetPoint || decoded.TargetX == nil || decoded.TargetZ == nil || *decoded.TargetX != x || *decoded.TargetZ != z {
		t.Fatalf("decoded=%#v", decoded)
	}

	legacyPayload, err := codec.Marshal(protocol.ClientUseAction{
		ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "2",
	})
	if err != nil { t.Fatal(err) }
	if bytes.Contains(legacyPayload, []byte("target_x")) || bytes.Contains(legacyPayload, []byte("target_z")) {
		t.Fatalf("legacy payload gained point fields: %s", legacyPayload)
	}
}
