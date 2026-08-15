package siege

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMatchStateAdvancesOnlyFromAuthoritativeBreachGate(t *testing.T) {
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	definition := MatchDefinition{
		ID:                "castle-sandbox-siege",
		AttackerID:        "attackers",
		DefenderID:        "defenders",
		BreachGateID:      "main-gate",
		ThroneObjectiveID: "throne",
	}
	if err := svc.ConfigureMatch(definition); err != nil {
		t.Fatal(err)
	}

	initial, ok := svc.MatchState()
	if !ok || initial.Revision != 1 || initial.Phase != MatchPhaseGate || initial.GateBreached {
		t.Fatalf("initial match state=%+v ok=%v", initial, ok)
	}
	if svc.ObserveGateState(GateState{ID: "side-gate", Destroyed: true}) {
		t.Fatal("non-breach gate advanced match")
	}
	if svc.ObserveGateState(GateState{ID: "main-gate", HP: 500, MaxHP: 1000, Destroyed: false}) {
		t.Fatal("live breach gate advanced match")
	}
	if !svc.ObserveGateState(GateState{ID: "main-gate", HP: 0, MaxHP: 1000, Destroyed: true}) {
		t.Fatal("destroyed breach gate did not advance match")
	}
	advanced, ok := svc.MatchState()
	if !ok || advanced.Revision != 2 || advanced.Phase != MatchPhaseThrone || !advanced.GateBreached {
		t.Fatalf("advanced match state=%+v ok=%v", advanced, ok)
	}
	if svc.ObserveGateState(GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("repeated destroyed observation advanced match twice")
	}
	afterRepeat, _ := svc.MatchState()
	if afterRepeat.Revision != 2 {
		t.Fatalf("repeat revision=%d", afterRepeat.Revision)
	}
}

func TestMatchParticipantTeamSeamIsExplicitServerState(t *testing.T) {
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne",
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(world.EntityID(101), TeamAttacker); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(world.EntityID(202), TeamDefender); err != nil {
		t.Fatal(err)
	}
	if team, ok := svc.ParticipantTeam(101); !ok || team != TeamAttacker {
		t.Fatalf("attacker team=%v ok=%v", team, ok)
	}
	if team, ok := svc.ParticipantTeam(202); !ok || team != TeamDefender {
		t.Fatalf("defender team=%v ok=%v", team, ok)
	}
	if err := svc.AssignParticipant(0, TeamAttacker); !errors.Is(err, ErrInvalidParticipant) {
		t.Fatalf("zero participant error=%v", err)
	}
	if err := svc.AssignParticipant(303, TeamUnknown); !errors.Is(err, ErrInvalidParticipant) {
		t.Fatalf("unknown team error=%v", err)
	}
}

func TestConfigureMatchRejectsInvalidDefinitionAndUnknownBreachGate(t *testing.T) {
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{}); !errors.Is(err, ErrInvalidMatchDefinition) {
		t.Fatalf("empty definition error=%v", err)
	}
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "same", DefenderID: "same", BreachGateID: "main-gate", ThroneObjectiveID: "throne",
	}); !errors.Is(err, ErrInvalidMatchDefinition) {
		t.Fatalf("same side identity error=%v", err)
	}
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "missing", ThroneObjectiveID: "throne",
	}); !errors.Is(err, ErrInvalidMatchDefinition) {
		t.Fatalf("unknown breach gate error=%v", err)
	}
}
