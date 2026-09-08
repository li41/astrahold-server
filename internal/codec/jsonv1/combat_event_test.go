package jsonv1

import (
	"bytes"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCombatEventRoundTripIncludesAuthoritativeBlockResult(t *testing.T) {
	codec := Codec{}
	x := float32(4.25)
	z := float32(-3.5)
	want := protocol.CombatEvent{
		ActionInstanceID:  77,
		ActorEntityID:     world.EntityID(10),
		ActionID:          "basic-attack",
		Result:            protocol.CombatEventHit,
		TargetEntityID:    world.EntityID(11),
		ImpactX:           &x,
		ImpactZ:           &z,
		Damage:            58,
		Blocked:           true,
		CooldownReadyTick: 240,
	}
	data, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"blocked":true`)) {
		t.Fatalf("wire missing blocked result: %s", data)
	}
	decoded, err := codec.Unmarshal(protocol.MessageCombatEvent, data)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(protocol.CombatEvent)
	if !ok {
		t.Fatalf("got type %T", decoded)
	}
	if got.ActionInstanceID != want.ActionInstanceID || got.ActorEntityID != want.ActorEntityID || got.ActionID != want.ActionID || got.Result != want.Result || got.TargetEntityID != want.TargetEntityID || got.Damage != want.Damage || got.Blocked != want.Blocked || got.CooldownReadyTick != want.CooldownReadyTick || got.ImpactX == nil || got.ImpactZ == nil || *got.ImpactX != x || *got.ImpactZ != z {
		t.Fatalf("got=%#v want=%#v", got, want)
	}
}

func TestCombatEventFalseBlockAndCooldownDefaults(t *testing.T) {
	codec := Codec{}
	decoded, err := codec.Unmarshal(protocol.MessageCombatEvent, []byte(`{"action_instance_id":1,"actor_entity_id":10,"action_id":"fireball","result":"miss","target_entity_id":0,"damage":0,"blocked":false}`))
	if err != nil {
		t.Fatal(err)
	}
	got := decoded.(protocol.CombatEvent)
	if got.CooldownReadyTick != 0 || got.Blocked {
		t.Fatalf("got=%#v", got)
	}
}

func TestEntitySpawnArchetypeRoundTripAndOmission(t *testing.T) {
	codec := Codec{}
	transform := protocol.EntityTransform{EntityID: 51, Tick: 8, Position: world.Position{X: 1, Z: 2}, Yaw: 0.5}
	want := protocol.EntitySpawn{EntityID: 51, Kind: world.EntityMonster, Transform: transform, ArchetypeID: "wolf-gray-01"}
	data, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := codec.Unmarshal(protocol.MessageEntitySpawn, data)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.(protocol.EntitySpawn)
	if !ok || got != want {
		t.Fatalf("got=%#v want=%#v", decoded, want)
	}

	decoded, err = codec.Unmarshal(protocol.MessageEntitySpawn, []byte(`{"entity_id":52,"kind":3,"transform":{"entity_id":52,"tick":9,"position":{"x":0,"y":0,"z":0,"layer":0},"yaw":0}}`))
	if err != nil {
		t.Fatal(err)
	}
	omitted := decoded.(protocol.EntitySpawn)
	if omitted.ArchetypeID != "" {
		t.Fatalf("expected empty archetype, got %q", omitted.ArchetypeID)
	}
}
