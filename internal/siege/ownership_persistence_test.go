package siege

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDurableThroneResolutionRetriesWithoutPublishingMatch(t *testing.T) {
	throne := ThroneObjectiveDefinition{
		ID: "throne",
		Zone: ObjectiveZone{
			Layer:  0,
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
	restored := CastleOwnershipState{Revision: 7, OwnerID: "defenders", PreviousOwnerID: "old-owner", LastTransferMatchID: "m0"}
	commitErr := errors.New("durable store unavailable")
	attempts := 0
	committer := func(transfer CastleOwnershipTransfer) (CastleOwnershipState, error) {
		attempts++
		if transfer.ExpectedRevision != 7 || transfer.PreviousOwnerID != "defenders" || transfer.OwnerID != "attackers" || transfer.MatchID != "m1" {
			t.Fatalf("transfer=%+v", transfer)
		}
		if attempts == 1 {
			return CastleOwnershipState{}, commitErr
		}
		return CastleOwnershipState{Revision: 8, OwnerID: "attackers", PreviousOwnerID: "defenders", LastTransferMatchID: "m1"}, nil
	}
	if err := svc.ConfigureCastleOwnershipPersistence(restored, committer); err != nil {
		t.Fatal(err)
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
	if !svc.AdvanceThroneCapture(100 * time.Millisecond) {
		t.Fatal("expected capture threshold")
	}

	resolved, err := svc.ResolveThroneCaptureWithError()
	if resolved || !errors.Is(err, commitErr) {
		t.Fatalf("first resolved=%v err=%v", resolved, err)
	}
	match, _ := svc.MatchState()
	ownership, _ := svc.CastleOwnershipState()
	capture, _ := svc.ThroneCaptureState()
	if match.Phase != MatchPhaseThrone || match.Revision != 2 || match.WinnerTeam != TeamUnknown || match.WinnerID != "" {
		t.Fatalf("failed commit published match=%+v", match)
	}
	if ownership != restored {
		t.Fatalf("failed commit changed ownership=%+v", ownership)
	}
	if !capture.ReadyForResolution || capture.Progress != capture.Required || !capture.Active {
		t.Fatalf("failed commit lost ready latch=%+v", capture)
	}

	resolved, err = svc.ResolveThroneCaptureWithError()
	if err != nil || !resolved {
		t.Fatalf("retry resolved=%v err=%v", resolved, err)
	}
	match, _ = svc.MatchState()
	ownership, _ = svc.CastleOwnershipState()
	if match.Phase != MatchPhaseCompleted || match.Revision != 3 || match.WinnerTeam != TeamAttacker || match.WinnerID != "attackers" {
		t.Fatalf("resolved match=%+v", match)
	}
	if ownership.Revision != 8 || ownership.OwnerID != "attackers" || ownership.PreviousOwnerID != "defenders" || ownership.LastTransferMatchID != "m1" {
		t.Fatalf("resolved ownership=%+v", ownership)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d", attempts)
	}
}

func TestRestoredAttackerOwnershipCompletesWithoutRedundantTransfer(t *testing.T) {
	throne := ThroneObjectiveDefinition{
		ID:              "throne",
		Zone:            ObjectiveZone{Layer: 0, Bounds: gameplayworld.BoundsXZ{MinX: -2, MaxX: 2, MinZ: 4, MaxZ: 8}},
		CaptureDuration: time.Millisecond,
	}
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{ID: "m2", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne", Throne: &throne}); err != nil {
		t.Fatal(err)
	}
	restored := CastleOwnershipState{Revision: 9, OwnerID: "attackers", PreviousOwnerID: "defenders", LastTransferMatchID: "m1"}
	calls := 0
	if err := svc.ConfigureCastleOwnershipPersistence(restored, func(CastleOwnershipTransfer) (CastleOwnershipState, error) {
		calls++
		return CastleOwnershipState{}, errors.New("must not commit no-op ownership")
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignParticipant(1, TeamAttacker); err != nil {
		t.Fatal(err)
	}
	svc.ObserveGateState(GateState{ID: "main-gate", Destroyed: true})
	svc.ObserveThronePresence([]ParticipantPresence{{EntityID: 1, Position: world.Position{Z: 6, Layer: 0}}})
	svc.AdvanceThroneCapture(time.Millisecond)
	resolved, err := svc.ResolveThroneCaptureWithError()
	if err != nil || !resolved {
		t.Fatalf("resolved=%v err=%v", resolved, err)
	}
	ownership, _ := svc.CastleOwnershipState()
	if ownership != restored || calls != 0 {
		t.Fatalf("ownership=%+v calls=%d", ownership, calls)
	}
}

func TestConfigureCastleOwnershipPersistenceRejectsOwnerOutsideMatch(t *testing.T) {
	svc := NewService([]gameplayworld.Gate{{ID: "main-gate", BlockerID: "main-gate", MaxHP: 1000}})
	if err := svc.ConfigureMatch(MatchDefinition{ID: "m3", AttackerID: "attackers", DefenderID: "defenders", BreachGateID: "main-gate", ThroneObjectiveID: "throne"}); err != nil {
		t.Fatal(err)
	}
	err := svc.ConfigureCastleOwnershipPersistence(
		CastleOwnershipState{Revision: 2, OwnerID: "unrelated-owner", PreviousOwnerID: "defenders", LastTransferMatchID: "older-match"},
		func(CastleOwnershipTransfer) (CastleOwnershipState, error) { return CastleOwnershipState{}, nil },
	)
	if !errors.Is(err, ErrInvalidCastleOwnership) {
		t.Fatalf("configure err=%v", err)
	}
}
