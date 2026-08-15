package siege

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/gameplayworld"
)

func TestLoadMatchStrictConfigRosterAndGateValidation(t *testing.T) {
	loaded, err := LoadMatch(strings.NewReader(`{
		"schema_version":3,
		"revision":"s4d2b-001",
		"match_id":"castle-sandbox-siege",
		"attacker_id":"attackers",
		"defender_id":"defenders",
		"breach_gate_id":"main-gate",
		"throne_objective_id":"throne",
		"throne_zone":{"layer":0,"bounds":{"min_x":-4,"max_x":4,"min_z":27,"max_z":33}},
		"throne_capture_seconds":10,
		"participants":[
			{"character_id":"character.attacker","team":"attacker"},
			{"character_id":"character.defender","team":"defender"}
		]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != "s4d2b-001" || loaded.Definition.ID != "castle-sandbox-siege" || loaded.Definition.BreachGateID != "main-gate" {
		t.Fatalf("loaded=%+v", loaded)
	}
	if loaded.Definition.Throne == nil || loaded.Definition.Throne.ID != "throne" || loaded.Definition.Throne.Zone.Layer != 0 || !loaded.Definition.Throne.Zone.Bounds.Contains(0, 30) {
		t.Fatalf("throne=%+v", loaded.Definition.Throne)
	}
	if loaded.Definition.Throne.CaptureDuration.Seconds() != 10 || loaded.ParticipantCount() != 2 {
		t.Fatalf("capture=%v participants=%d", loaded.Definition.Throne.CaptureDuration, loaded.ParticipantCount())
	}

	attacker, _ := characteridentity.NewTrusted("character.attacker")
	defender, _ := characteridentity.NewTrusted("character.defender")
	unlisted, _ := characteridentity.NewTrusted("character.other")
	ephemeral, _ := characteridentity.NewEphemeral()
	if team, ok := loaded.ResolveParticipant(attacker); !ok || team != TeamAttacker {
		t.Fatalf("attacker team=%v ok=%v", team, ok)
	}
	if team, ok := loaded.ResolveParticipant(defender); !ok || team != TeamDefender {
		t.Fatalf("defender team=%v ok=%v", team, ok)
	}
	if _, ok := loaded.ResolveParticipant(unlisted); ok {
		t.Fatal("unlisted trusted character must remain unknown")
	}
	if _, ok := loaded.ResolveParticipant(ephemeral); ok {
		t.Fatal("ephemeral character must never resolve from trusted roster")
	}
	if err := ValidateMatchAgainstGates(loaded.Definition, []gameplayworld.Gate{{ID: "main-gate"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMatchAgainstGates(loaded.Definition, []gameplayworld.Gate{{ID: "other"}}); !errors.Is(err, ErrInvalidMatchDefinition) {
		t.Fatalf("missing gate error=%v", err)
	}
}

func TestLoadMatchRejectsInvalidSchemaFieldsCaptureAndRoster(t *testing.T) {
	base := `"revision":"r1","match_id":"m","attacker_id":"a","defender_id":"d","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":0,"max_x":1,"min_z":0,"max_z":1}},"throne_capture_seconds":10`
	tests := []struct {
		name string
		json string
		want error
	}{
		{"schema", `{"schema_version":4,` + base + `,"participants":[]}`, ErrUnsupportedMatchConfigSchema},
		{"unknown", `{"schema_version":3,` + base + `,"participants":[],"client_truth":true}`, ErrInvalidMatchConfig},
		{"same sides", `{"schema_version":3,"revision":"r1","match_id":"m","attacker_id":"same","defender_id":"same","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":0,"max_x":1,"min_z":0,"max_z":1}},"throne_capture_seconds":10,"participants":[]}`, ErrInvalidMatchConfig},
		{"invalid zone", `{"schema_version":3,"revision":"r1","match_id":"m","attacker_id":"a","defender_id":"d","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":1,"max_x":1,"min_z":0,"max_z":1}},"throne_capture_seconds":10,"participants":[]}`, ErrInvalidMatchConfig},
		{"zero capture", `{"schema_version":3,` + strings.Replace(base, `"throne_capture_seconds":10`, `"throne_capture_seconds":0`, 1) + `,"participants":[]}`, ErrInvalidMatchConfig},
		{"capture too long", `{"schema_version":3,` + strings.Replace(base, `"throne_capture_seconds":10`, `"throne_capture_seconds":3601`, 1) + `,"participants":[]}`, ErrInvalidMatchConfig},
		{"invalid team", `{"schema_version":3,` + base + `,"participants":[{"character_id":"character.one","team":"spectator"}]}`, ErrInvalidMatchConfig},
		{"invalid character", `{"schema_version":3,` + base + `,"participants":[{"character_id":"bad character","team":"attacker"}]}`, ErrInvalidMatchConfig},
		{"duplicate character", `{"schema_version":3,` + base + `,"participants":[{"character_id":"character.one","team":"attacker"},{"character_id":"character.one","team":"defender"}]}`, ErrInvalidMatchConfig},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadMatch(strings.NewReader(test.json))
			if !errors.Is(err, test.want) {
				t.Fatalf("error=%v want=%v", err, test.want)
			}
		})
	}
}
