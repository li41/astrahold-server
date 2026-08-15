package siege

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

func TestLoadMatchStrictConfigAndGateValidation(t *testing.T) {
	loaded, err := LoadMatch(strings.NewReader(`{
		"schema_version":2,
		"revision":"s4d2a-001",
		"match_id":"castle-sandbox-siege",
		"attacker_id":"attackers",
		"defender_id":"defenders",
		"breach_gate_id":"main-gate",
		"throne_objective_id":"throne",
		"throne_zone":{"layer":0,"bounds":{"min_x":-4,"max_x":4,"min_z":27,"max_z":33}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != "s4d2a-001" || loaded.Definition.ID != "castle-sandbox-siege" || loaded.Definition.BreachGateID != "main-gate" {
		t.Fatalf("loaded=%+v", loaded)
	}
	if loaded.Definition.Throne == nil || loaded.Definition.Throne.ID != "throne" || loaded.Definition.Throne.Zone.Layer != 0 || !loaded.Definition.Throne.Zone.Bounds.Contains(0, 30) {
		t.Fatalf("throne=%+v", loaded.Definition.Throne)
	}
	if err := ValidateMatchAgainstGates(loaded.Definition, []gameplayworld.Gate{{ID: "main-gate"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateMatchAgainstGates(loaded.Definition, []gameplayworld.Gate{{ID: "other"}}); !errors.Is(err, ErrInvalidMatchDefinition) {
		t.Fatalf("missing gate error=%v", err)
	}
}

func TestLoadMatchRejectsSchemaUnknownFieldsInvalidSidesAndZone(t *testing.T) {
	tests := []struct {
		name string
		json string
		want error
	}{
		{"schema", `{"schema_version":3,"revision":"r1","match_id":"m","attacker_id":"a","defender_id":"d","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":0,"max_x":1,"min_z":0,"max_z":1}}}`, ErrUnsupportedMatchConfigSchema},
		{"unknown", `{"schema_version":2,"revision":"r1","match_id":"m","attacker_id":"a","defender_id":"d","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":0,"max_x":1,"min_z":0,"max_z":1}},"client_truth":true}`, ErrInvalidMatchConfig},
		{"same sides", `{"schema_version":2,"revision":"r1","match_id":"m","attacker_id":"same","defender_id":"same","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":0,"max_x":1,"min_z":0,"max_z":1}}}`, ErrInvalidMatchConfig},
		{"invalid zone", `{"schema_version":2,"revision":"r1","match_id":"m","attacker_id":"a","defender_id":"d","breach_gate_id":"g","throne_objective_id":"t","throne_zone":{"layer":0,"bounds":{"min_x":1,"max_x":1,"min_z":0,"max_z":1}}}`, ErrInvalidMatchConfig},
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
