package jsonv1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestActionRejectedRoundTripPreservesAuthorityContract(t *testing.T) {
	codec := Codec{}
	want := protocol.ActionRejected{
		ClientActionSequence: 42,
		ActorEntityID: world.EntityID(7),
		ActionID: "fireball",
		TargetKind: protocol.ActionTargetPoint,
		Reason: protocol.ActionRejectionCooldown,
		CooldownReadyTick: 1234,
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageActionRejected, data)
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.ActionRejected)
	if !ok { t.Fatalf("got type %T", decoded) }
	if got != want { t.Fatalf("got=%#v want=%#v", got, want) }
}
