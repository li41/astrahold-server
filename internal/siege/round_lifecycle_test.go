package siege

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCompletedRoundResetRotatesRolesAndRestoresObjectives(t *testing.T) {
	gateDefinition := gameplayworld.Gate{
		ID: "main-gate", BlockerID: "main-gate", MaxHP: 100,
		Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 100, CooldownSeconds: 0.5},
	}
	scene := &fakeWorld{
		blocker: gameplayworld.Blocker{
			ID: "main-gate", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -1, MaxX: 1, MinZ: 1, MaxZ: 2},
			MinY:   0, MaxY: 3, BlocksMovement: true, BlocksLOS: true, Enabled: true,
		},
		enabled: true,
		los:     true,
	}
	attackerIdentity, _ := characteridentity.NewTrusted("character.alpha")
	defenderIdentity, _ := characteridentity.NewTrusted("character.beta")
	throne := ThroneObjectiveDefinition{
		ID: "throne",
		Zone: ObjectiveZone{
			Layer:  0,
			Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8},
		},
		CaptureDuration: 50 * time.Millisecond,
	}
	svc := NewService([]gameplayworld.Gate{gateDefinition})
	if err := svc.ConfigureMatch(MatchDefinition{
		ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne,
		ParticipantTeams: map[characteridentity.ID]Team{
			attackerIdentity.ID: TeamAttacker,
			defenderIdentity.ID: TeamDefender,
		},
	}); err != nil {
		t.Fatal(err)
	}
	if assigned, err := svc.AssignResolvedParticipant(1, attackerIdentity); err != nil || !assigned {
		t.Fatalf("assign attacker assigned=%v err=%v", assigned, err)
	}
	if assigned, err := svc.AssignResolvedParticipant(2, defenderIdentity); err != nil || !assigned {
		t.Fatalf("assign defender assigned=%v err=%v", assigned, err)
	}

	breachPosition := world.Position{X: 0, Z: 0, Layer: 0}
	gateState, err := svc.Attack(1, breachPosition, "main-gate", 1, 50*time.Millisecond, scene)
	if err != nil {
		t.Fatal(err)
	}
	if !gateState.Destroyed || scene.enabled {
		t.Fatalf("first breach gate=%+v blocker=%v", gateState, scene.enabled)
	}
	if !svc.ObserveGateState(gateState) {
		t.Fatal("first breach did not enter throne phase")
	}
	inside := world.Position{Z: 6, Layer: 0}
	if !svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 1, Position: inside}}) {
		t.Fatal("first attacker presence not observed")
	}
	svc.AdvanceThroneCapture(50 * time.Millisecond)
	if !svc.ResolveThroneCapture() {
		t.Fatal("first round did not resolve")
	}

	completed, _ := svc.MatchState()
	ownership, _ := svc.CastleOwnershipState()
	if completed.Round != 1 || completed.Phase != MatchPhaseCompleted || completed.WinnerID != "attackers" || ownership.Revision != 2 || ownership.OwnerID != "attackers" {
		t.Fatalf("first completion match=%+v ownership=%+v", completed, ownership)
	}

	changed, err := svc.StartNextRound(scene)
	if err != nil || !changed {
		t.Fatalf("start second round changed=%v err=%v", changed, err)
	}
	second, _ := svc.MatchState()
	if second.Round != 2 || second.Phase != MatchPhaseGate || second.AttackerID != "defenders" || second.DefenderID != "attackers" || second.GateBreached || second.WinnerTeam != TeamUnknown || second.WinnerID != "" {
		t.Fatalf("second round=%+v", second)
	}
	if team, _ := svc.ParticipantTeam(1); team != TeamDefender {
		t.Fatalf("former attacker role=%v", team)
	}
	if team, _ := svc.ParticipantTeam(2); team != TeamAttacker {
		t.Fatalf("former defender role=%v", team)
	}
	if assigned, err := svc.AssignResolvedParticipant(3, attackerIdentity); err != nil || !assigned {
		t.Fatalf("reconnect assign assigned=%v err=%v", assigned, err)
	}
	if team, _ := svc.ParticipantTeam(3); team != TeamDefender {
		t.Fatalf("trusted reconnect role=%v", team)
	}
	states := svc.States()
	if len(states) != 1 || states[0].HP != states[0].MaxHP || states[0].Destroyed || !scene.enabled {
		t.Fatalf("reset gate=%+v blocker=%v", states, scene.enabled)
	}
	presence, _ := svc.ThronePresenceState()
	capture, _ := svc.ThroneCaptureState()
	if presence.Active || presence.AttackerCount != 0 || presence.DefenderCount != 0 || presence.Contested || presence.CaptureEligible {
		t.Fatalf("reset presence=%+v", presence)
	}
	if capture.Active || capture.Progress != 0 || capture.ReadyForResolution || capture.Required != 50*time.Millisecond {
		t.Fatalf("reset capture=%+v", capture)
	}
	matchRevision := second.Revision
	if changed, err := svc.StartNextRound(scene); err != nil || changed {
		t.Fatalf("repeated reset changed=%v err=%v", changed, err)
	}
	afterRepeat, _ := svc.MatchState()
	if afterRepeat.Revision != matchRevision {
		t.Fatalf("repeated reset changed revision=%d want=%d", afterRepeat.Revision, matchRevision)
	}

	gateState, err = svc.Attack(2, breachPosition, "main-gate", 2, 50*time.Millisecond, scene)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.ObserveGateState(gateState) {
		t.Fatal("second breach did not enter throne phase")
	}
	svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 2, Position: inside}})
	svc.AdvanceThroneCapture(50 * time.Millisecond)
	if !svc.ResolveThroneCapture() {
		t.Fatal("second round did not resolve")
	}
	secondCompleted, _ := svc.MatchState()
	ownership, _ = svc.CastleOwnershipState()
	if secondCompleted.Round != 2 || secondCompleted.WinnerID != "defenders" || ownership.Revision != 3 || ownership.OwnerID != "defenders" {
		t.Fatalf("second completion match=%+v ownership=%+v", secondCompleted, ownership)
	}
}

func TestStartNextRoundRequiresExactNextOwnershipEpoch(t *testing.T) {
	throne := ThroneObjectiveDefinition{
		ID:              "throne",
		Zone:            ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8}},
		CaptureDuration: time.Millisecond,
	}
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 100}})
	if err := svc.ConfigureMatch(MatchDefinition{ID: "m1", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne}); err != nil {
		t.Fatal(err)
	}
	svc.match.state.Phase = MatchPhaseCompleted
	svc.match.state.WinnerTeam = TeamAttacker
	svc.match.state.WinnerID = "attackers"
	svc.match.ownership = CastleOwnershipState{Revision: 3, OwnerID: "attackers", PreviousOwnerID: "defenders", LastTransferMatchID: "m1"}
	scene := &fakeWorld{blocker: gameplayworld.Blocker{ID: "main-gate"}, enabled: false, los: true}
	changed, err := svc.StartNextRound(scene)
	if changed || !errors.Is(err, ErrRoundResetUnavailable) {
		t.Fatalf("skipped epoch changed=%v err=%v", changed, err)
	}
	if scene.enabled {
		t.Fatal("invalid epoch mutated blocker before validation")
	}
}
