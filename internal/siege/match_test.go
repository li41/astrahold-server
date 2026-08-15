package siege

import (
	"errors"
	"testing"
	"time"

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
	if !ok || initial.Revision != 1 || initial.Phase != MatchPhaseGate || initial.GateBreached || initial.WinnerTeam != TeamUnknown || initial.WinnerID != "" {
		t.Fatalf("initial match state=%+v ok=%v", initial, ok)
	}
	ownership, ok := svc.CastleOwnershipState()
	if !ok || ownership.Revision != 1 || ownership.OwnerID != "defenders" || ownership.PreviousOwnerID != "" || ownership.LastTransferMatchID != "" {
		t.Fatalf("initial ownership=%+v ok=%v", ownership, ok)
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

func TestThroneCaptureResolutionCompletesAndTransfersOwnershipOnce(t *testing.T) {
	throne := ThroneObjectiveDefinition{
		ID: "throne",
		Zone: ObjectiveZone{
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8},
		},
		CaptureDuration: 100 * time.Millisecond,
	}
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne,
	}); err != nil {
		t.Fatal(err)
	}
	if svc.ResolveThroneCapture() {
		t.Fatal("gate phase must not resolve")
	}
	if err := svc.AssignParticipant(101, TeamAttacker); err != nil {
		t.Fatal(err)
	}
	if !svc.ObserveGateState(GateState{ID: "main-gate", Destroyed: true}) {
		t.Fatal("expected gate breach")
	}
	inside := world.Position{X: 0, Z: 6, Layer: 0}
	if !svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 101, Position: inside}}) {
		t.Fatal("expected attacker presence")
	}
	if svc.ResolveThroneCapture() {
		t.Fatal("capture below threshold must not resolve")
	}
	if !svc.AdvanceThroneCapture(100 * time.Millisecond) {
		t.Fatal("expected capture threshold")
	}
	ready, ok := svc.ThroneCaptureState()
	if !ok || !ready.ReadyForResolution || ready.Progress != ready.Required || !ready.Active {
		t.Fatalf("ready capture=%+v ok=%v", ready, ok)
	}
	if !svc.ResolveThroneCapture() {
		t.Fatal("ready throne capture did not resolve")
	}

	match, ok := svc.MatchState()
	if !ok || match.Revision != 3 || match.Phase != MatchPhaseCompleted || match.WinnerTeam != TeamAttacker || match.WinnerID != "attackers" || !match.GateBreached {
		t.Fatalf("resolved match=%+v ok=%v", match, ok)
	}
	ownership, ok := svc.CastleOwnershipState()
	if !ok || ownership.Revision != 2 || ownership.OwnerID != "attackers" || ownership.PreviousOwnerID != "defenders" || ownership.LastTransferMatchID != "m1" {
		t.Fatalf("resolved ownership=%+v ok=%v", ownership, ok)
	}
	presence, _ := svc.ThronePresenceState()
	capture, _ := svc.ThroneCaptureState()
	if presence.Active || presence.AttackerCount != 0 || presence.DefenderCount != 0 || presence.Contested || presence.CaptureEligible {
		t.Fatalf("settled presence=%+v", presence)
	}
	if capture.Active || !capture.ReadyForResolution || capture.Progress != capture.Required {
		t.Fatalf("settled capture=%+v", capture)
	}

	matchRevision := match.Revision
	ownershipRevision := ownership.Revision
	captureRevision := capture.Revision
	presenceRevision := presence.Revision
	if svc.ResolveThroneCapture() {
		t.Fatal("completed match resolved twice")
	}
	match, _ = svc.MatchState()
	ownership, _ = svc.CastleOwnershipState()
	capture, _ = svc.ThroneCaptureState()
	presence, _ = svc.ThronePresenceState()
	if match.Revision != matchRevision || ownership.Revision != ownershipRevision || capture.Revision != captureRevision || presence.Revision != presenceRevision {
		t.Fatalf("repeat changed revisions match=%d ownership=%d capture=%d presence=%d", match.Revision, ownership.Revision, capture.Revision, presence.Revision)
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
