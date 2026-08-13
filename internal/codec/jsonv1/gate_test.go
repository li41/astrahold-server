package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestActionControlMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	action := protocol.ClientUseAction{ActionID: "basic-attack", TargetKind: protocol.ActionTargetGate, TargetID: "main-gate"}
	data, err := codec.Marshal(action)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageClientUseAction, data)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(decoded, action) { t.Fatalf("action got=%#v want=%#v", decoded, action) }

	state := protocol.WorldDynamicState{
		Revision: 4,
		Blockers: []protocol.WorldBlockerState{{ID: "main-gate", Enabled: false}},
		Gates: []protocol.WorldGateState{{ID: "main-gate", HP: 0, MaxHP: 1000, Destroyed: true}},
	}
	data, err = codec.Marshal(state)
	if err != nil { t.Fatal(err) }
	decoded, err = codec.Unmarshal(protocol.MessageWorldDynamicState, data)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(decoded, state) { t.Fatalf("dynamic got=%#v want=%#v", decoded, state) }
}
