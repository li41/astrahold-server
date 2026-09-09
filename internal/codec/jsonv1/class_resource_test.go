package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCharacterClassResourceStateRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.CharacterClassResourceState{
		EntityID:   world.EntityID(77),
		ResourceID: "resolve",
		Current:    24,
		Max:        100,
	}
	payload, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got, err := codec.Unmarshal(want.Type(), payload)
	if err != nil {
		t.Fatalf("unmarshal payload=%s: %v", payload, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}
