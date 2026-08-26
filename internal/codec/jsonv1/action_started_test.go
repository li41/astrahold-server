package jsonv1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestActionStartedRoundTripPreservesAcceptedPointTarget(t *testing.T) {
	codec := Codec{}
	x := float32(8.25)
	z := float32(-4.5)
	want := protocol.ActionStarted{
		ActionInstanceID: 88,
		ActorEntityID: world.EntityID(10),
		ActionID: "meteor-strike",
		TargetKind: protocol.ActionTargetPoint,
		TargetX: &x,
		TargetZ: &z,
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageActionStarted, data)
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.ActionStarted)
	if !ok { t.Fatalf("got type %T", decoded) }
	if got.ActionInstanceID != want.ActionInstanceID || got.ActorEntityID != want.ActorEntityID || got.ActionID != want.ActionID || got.TargetKind != want.TargetKind || got.TargetID != "" || got.TargetX == nil || got.TargetZ == nil || *got.TargetX != x || *got.TargetZ != z {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}

func TestActionStartedRoundTripPreservesEntityTargetID(t *testing.T) {
	codec := Codec{}
	want := protocol.ActionStarted{ActionInstanceID: 89, ActorEntityID: world.EntityID(10), ActionID: "basic-attack", TargetKind: protocol.ActionTargetEntity, TargetID: "11"}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageActionStarted, data)
	if err != nil { t.Fatal(err) }
	got := decoded.(protocol.ActionStarted)
	if got != want { t.Fatalf("got=%#v want=%#v", got, want) }
}
