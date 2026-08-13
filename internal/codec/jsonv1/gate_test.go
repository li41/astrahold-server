package jsonv1

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestGateControlMessagesRoundTrip(t *testing.T) {
	codec := Codec{}
	attack := protocol.ClientAttackGate{GateID: "main-gate"}
	data, err := codec.Marshal(attack)
	if err != nil { t.Fatal(err) }
	decoded, err := codec.Unmarshal(protocol.MessageClientAttackGate, data)
	if err != nil { t.Fatal(err) }
	if !reflect.DeepEqual(decoded, attack) { t.Fatalf("attack got=%#v want=%#v", decoded, attack) }

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
