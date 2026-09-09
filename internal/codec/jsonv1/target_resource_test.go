package jsonv1

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestMarshalCharacterTargetResourceState(t *testing.T) {
	payload, err := (Codec{}).Marshal(protocol.CharacterTargetResourceState{SourceEntityID: world.EntityID(10), TargetEntityID: world.EntityID(9403), ResourceID: "flaw", Current: 2, Max: 3})
	if err != nil { t.Fatal(err) }
	const want = `{"source_entity_id":10,"target_entity_id":9403,"resource_id":"flaw","current":2,"max":3}`
	if string(payload) != want { t.Fatalf("payload=%s want=%s", payload, want) }
	if protocol.Version != 27 { t.Fatalf("protocol version=%d want 27", protocol.Version) }
	if protocol.MessageCharacterTargetResourceState != 118 { t.Fatalf("type=%d want 118", protocol.MessageCharacterTargetResourceState) }
}
