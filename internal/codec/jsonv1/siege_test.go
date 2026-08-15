package jsonv1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestSiegeMatchStateRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.SiegeMatchState{
		Revision:          2,
		Round:             4,
		MatchID:           "castle-sandbox-siege",
		AttackerID:        "attackers",
		DefenderID:        "defenders",
		YourTeam:          protocol.SiegeTeamAttacker,
		Phase:             protocol.SiegePhaseThrone,
		BreachGateID:      "main-gate",
		ThroneObjectiveID: "throne",
		GateBreached:      true,
		WinnerTeam:        protocol.SiegeTeamUnknown,
		WinnerID:          "",
		CastleOwnerID:     "defenders",
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageSiegeMatchState, data)
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.SiegeMatchState)
	if !ok || got != want { t.Fatalf("got=%#v want=%#v", decoded, want) }
}

func TestSiegeMatchCompletedResultRoundTrip(t *testing.T) {
	codec := Codec{}
	want := protocol.SiegeMatchState{
		Revision:          3,
		Round:             4,
		MatchID:           "castle-sandbox-siege",
		AttackerID:        "attackers",
		DefenderID:        "defenders",
		YourTeam:          protocol.SiegeTeamDefender,
		Phase:             protocol.SiegePhaseCompleted,
		BreachGateID:      "main-gate",
		ThroneObjectiveID: "throne",
		GateBreached:      true,
		WinnerTeam:        protocol.SiegeTeamAttacker,
		WinnerID:          "attackers",
		CastleOwnerID:     "attackers",
	}
	data, err := codec.Marshal(want)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageSiegeMatchState, data)
	if err != nil { t.Fatal(err) }
	got, ok := decoded.(protocol.SiegeMatchState)
	if !ok || got != want { t.Fatalf("got=%#v want=%#v", decoded, want) }
}

func TestSiegeMatchStateRejectsUnknownField(t *testing.T) {
	codec := Codec{}
	_, err := codec.Unmarshal(protocol.MessageSiegeMatchState, []byte(`{"revision":1,"round":1,"match_id":"m1","attacker_id":"a","defender_id":"d","your_team":"attacker","phase":"gate","breach_gate_id":"main-gate","throne_objective_id":"throne","gate_breached":false,"winner_team":"unknown","winner_id":"","castle_owner_id":"d","client_progress":100}`))
	if err == nil { t.Fatal("expected strict JSON rejection") }
}
