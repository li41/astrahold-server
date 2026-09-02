package jsonv1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestEntityVitalsRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.EntityVitalsState{
		EntityID: world.EntityID(42),
		HP: 725,
		MaxHP: 1000,
		MP: 40,
		MaxMP: 100,
		Defeated: false,
		ReviveProtectionUntilTick: 900,
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageEntityVitalsState, data)
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.EntityVitalsState)
	if !ok || got != want { t.Fatalf("got=%#v want=%#v", decoded, want) }
}

func TestEntityVitalsV12PayloadDefaultsMPToZero(t *testing.T) {
	codec := Codec{}
	decoded, err := codec.Unmarshal(protocol.MessageEntityVitalsState, []byte(`{"entity_id":42,"hp":725,"max_hp":1000,"defeated":false}`))
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.EntityVitalsState)
	if !ok { t.Fatalf("got=%#v", decoded) }
	if got.MP != 0 || got.MaxMP != 0 { t.Fatalf("legacy mp=%d/%d want=0/0", got.MP, got.MaxMP) }
	if got.ReviveProtectionUntilTick != 0 { t.Fatalf("revive protection=%d want=0", got.ReviveProtectionUntilTick) }
}

func TestEntityVitalsRejectsUnknownField(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageEntityVitalsState, []byte(`{"entity_id":42,"hp":10,"max_hp":1000,"mp":40,"max_mp":100,"defeated":false,"client_damage":999}`))
	if err == nil { t.Fatal("expected strict JSON rejection") }
}
