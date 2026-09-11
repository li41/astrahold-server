package jsonv1

import (
	"bytes"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAppendMarshalMatchesMarshalForHotReliableMessages(t *testing.T) {
	codec := Codec{}
	cases := []struct {
		name    string
		message protocol.Message
	}{
		{
			name: "action-started",
			message: protocol.ActionStarted{
				ActionInstanceID: 17,
				ActorEntityID:    world.EntityID(23),
				ActionID:         "soak-attack",
				TargetKind:       protocol.ActionTargetEntity,
				TargetID:         "entity-24",
			},
		},
		{
			name: "vitals",
			message: protocol.EntityVitalsState{
				EntityID:  world.EntityID(23),
				HP:        900,
				MaxHP:     1000,
				MP:        45,
				MaxMP:     50,
				Defeated:  false,
			},
		},
		{
			name: "vitals-with-revive-protection",
			message: protocol.EntityVitalsState{
				EntityID:                  world.EntityID(23),
				HP:                        1000,
				MaxHP:                     1000,
				MP:                        50,
				MaxMP:                     50,
				ReviveProtectionUntilTick: 777,
			},
		},
		{
			name: "combat-event",
			message: protocol.CombatEvent{
				ActionInstanceID: 17,
				ActorEntityID:    world.EntityID(23),
				ActionID:         "soak-attack",
				Result:           protocol.CombatEventHit,
				TargetEntityID:   world.EntityID(24),
				Damage:           100,
				Blocked:          true,
			},
		},
		{
			name: "combat-event-with-cooldown",
			message: protocol.CombatEvent{
				ActionInstanceID: 17,
				ActorEntityID:    world.EntityID(23),
				ActionID:         "soak-attack",
				Result:           protocol.CombatEventHit,
				TargetEntityID:   world.EntityID(24),
				Damage:           100,
				CooldownReadyTick: 99,
			},
		},
		{
			name: "escaping-falls-back-to-json",
			message: protocol.ActionStarted{
				ActionInstanceID: 17,
				ActorEntityID:    world.EntityID(23),
				ActionID:         "attack<quoted>",
				TargetKind:       protocol.ActionTargetEntity,
				TargetID:         "entity\\24",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := codec.Marshal(tc.message)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			prefix := []byte("prefix:")
			got, err := codec.AppendMarshal(append([]byte(nil), prefix...), tc.message)
			if err != nil {
				t.Fatalf("AppendMarshal() error = %v", err)
			}
			got = got[len(prefix):]
			if !bytes.Equal(got, want) {
				t.Fatalf("AppendMarshal() = %q, Marshal() = %q", got, want)
			}
		})
	}
}
