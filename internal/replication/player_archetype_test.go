package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/playerarchetype"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildReplicatesCanonicalPlayerArchetype(t *testing.T) {
	svc := NewService()
	sid := session.ID(41)
	svc.Register(sid)

	batch := svc.Build(sid, 1, 0, 1, []world.EntityState{{
		ID:          1,
		Kind:        world.EntityPlayer,
		ArchetypeID: playerarchetype.DefaultID,
	}})

	var spawn *protocol.EntitySpawn
	for _, outbound := range batch.Messages {
		message, ok := outbound.Message.(protocol.EntitySpawn)
		if !ok || message.EntityID != 1 {
			continue
		}
		copy := message
		spawn = &copy
		break
	}
	if spawn == nil {
		t.Fatal("player EntitySpawn not produced")
	}
	if spawn.ArchetypeID != playerarchetype.DefaultID {
		t.Fatalf("spawn archetype=%q want=%q", spawn.ArchetypeID, playerarchetype.DefaultID)
	}
}
