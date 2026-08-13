package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestBuildProducesSpawnSnapshotAndCorrection(t *testing.T) {
	svc := NewService()
	sid := session.ID(7)
	svc.Register(sid)
	visible := []world.EntityState{{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1}}}, {ID: 2, Kind: world.EntityMonster, Transform: world.Transform{Position: world.Position{X: 2}}}}
	batch := svc.Build(sid, 1, 12, 20, visible)
	var spawn, snapshot, correction int
	for _, m := range batch.Messages {
		switch m.Message.Type() {
		case protocol.MessageEntitySpawn:
			spawn++
		case protocol.MessageWorldSnapshot:
			snapshot++
		case protocol.MessagePositionCorrection:
			correction++
		}
	}
	if spawn != 2 || snapshot != 1 || correction != 1 {
		t.Fatalf("unexpected counts spawn=%d snapshot=%d correction=%d", spawn, snapshot, correction)
	}
}
