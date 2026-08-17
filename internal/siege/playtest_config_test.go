package siege

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
)

func TestPlayableSiegeMVPProfile(t *testing.T) {
	loaded, err := LoadMatchFile("../../config/siege-match-playtest.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != "s5a1-playtest-001" {
		t.Fatalf("revision=%q", loaded.Revision)
	}
	if loaded.Definition.ID != "castle-sandbox-siege-playtest" {
		t.Fatalf("match id=%q", loaded.Definition.ID)
	}
	if loaded.Definition.AttackerID != "attackers" || loaded.Definition.DefenderID != "defenders" {
		t.Fatalf("unexpected teams: attacker=%q defender=%q", loaded.Definition.AttackerID, loaded.Definition.DefenderID)
	}
	if loaded.Definition.BreachGateID != "main-gate" || loaded.Definition.ThroneObjectiveID != "throne" {
		t.Fatalf("unexpected objectives: gate=%q throne=%q", loaded.Definition.BreachGateID, loaded.Definition.ThroneObjectiveID)
	}
	if loaded.Definition.Throne == nil || loaded.Definition.Throne.CaptureDuration != 10*time.Second {
		t.Fatalf("unexpected throne config: %+v", loaded.Definition.Throne)
	}
	if loaded.ParticipantCount() != 2 {
		t.Fatalf("participant count=%d", loaded.ParticipantCount())
	}

	attacker, err := characteridentity.NewTrusted("playtest-attacker")
	if err != nil {
		t.Fatal(err)
	}
	if team, ok := loaded.ResolveParticipant(attacker); !ok || team != TeamAttacker {
		t.Fatalf("attacker resolved team=%v ok=%t", team, ok)
	}
	defender, err := characteridentity.NewTrusted("playtest-defender")
	if err != nil {
		t.Fatal(err)
	}
	if team, ok := loaded.ResolveParticipant(defender); !ok || team != TeamDefender {
		t.Fatalf("defender resolved team=%v ok=%t", team, ok)
	}
	unknown, err := characteridentity.NewTrusted("playtest-unknown")
	if err != nil {
		t.Fatal(err)
	}
	if team, ok := loaded.ResolveParticipant(unknown); ok || team != TeamUnknown {
		t.Fatalf("unknown participant unexpectedly resolved team=%v ok=%t", team, ok)
	}
}
