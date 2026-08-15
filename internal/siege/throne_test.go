package siege

import (
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestThronePresenceIsPhaseGatedServerOwnedAndContestAware(t *testing.T) {
	throne := ThroneObjectiveDefinition{
		ID: "throne",
		Zone: ObjectiveZone{
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8},
		},
	}
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(101, TeamAttacker); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(202, TeamDefender); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(303, TeamAttacker); err != nil {
		t.Fatal(err)
	}

	inside := world.Position{X: 0, Z: 6, Layer: 0}
	if svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 101, Position: inside}}) {
		t.Fatal("gate phase must keep throne presence locked")
	}
	locked, ok := svc.ThronePresenceState()
	if !ok || locked.Revision != 1 || locked.Active || locked.AttackerCount != 0 || locked.CaptureEligible {
		t.Fatalf("locked state=%+v ok=%v", locked, ok)
	}

	if !svc.ObserveGateState(GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("expected authoritative gate breach")
	}
	if !svc.ObserveThronePresence([]ParticipantPresence{
		{EntityID: 101, Position: inside},
		{EntityID: 101, Position: inside},
		{EntityID: 303, Position: world.Position{X: 0, Z: 6, Layer: 1}},
		{EntityID: 404, Position: inside},
	}) {
		t.Fatal("throne activation/presence should change state")
	}
	attacking, _ := svc.ThronePresenceState()
	if attacking.Revision != 2 || !attacking.Active || attacking.AttackerCount != 1 || attacking.DefenderCount != 0 || attacking.Contested || !attacking.CaptureEligible {
		t.Fatalf("attacking state=%+v", attacking)
	}
	if svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 101, Position: inside}}) {
		t.Fatal("identical presence must be idempotent")
	}

	if !svc.ObserveThronePresence([]ParticipantPresence{
		{EntityID: 101, Position: inside},
		{EntityID: 202, Position: inside},
	}) {
		t.Fatal("defender arrival should change presence")
	}
	contested, _ := svc.ThronePresenceState()
	if contested.Revision != 3 || contested.AttackerCount != 1 || contested.DefenderCount != 1 || !contested.Contested || contested.CaptureEligible {
		t.Fatalf("contested state=%+v", contested)
	}

	if !svc.ObserveThronePresence([]ParticipantPresence{
		{EntityID: 101, Position: inside},
		{EntityID: 202, Position: inside, Defeated: true},
	}) {
		t.Fatal("defeated defender must stop contesting")
	}
	afterDefeat, _ := svc.ThronePresenceState()
	if afterDefeat.Revision != 4 || afterDefeat.DefenderCount != 0 || afterDefeat.Contested || !afterDefeat.CaptureEligible {
		t.Fatalf("after defeat state=%+v", afterDefeat)
	}
}

func TestThroneObjectiveRejectsInvalidZone(t *testing.T) {
	bad := ThroneObjectiveDefinition{
		ID: "throne",
		Zone: ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: 2, MaxX: 2, MinZ: 0, MaxZ: 1}},
	}
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &bad,
	}); err == nil {
		t.Fatal("expected invalid throne zone rejection")
	}
}
