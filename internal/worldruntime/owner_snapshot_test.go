package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

type ownerSnapshotBatch struct {
	Inventory         protocol.InventorySnapshot
	InventoryInstance protocol.InventoryInstanceSnapshot
	Equipment         protocol.EquipmentSnapshot
	EquipmentInstance protocol.EquipmentInstanceSnapshot
	Appearance        protocol.AppearanceSnapshot
}

func readOwnerSnapshotBatch(t *testing.T, connection *session.QueueConnection) ownerSnapshotBatch {
	t.Helper()
	var batch ownerSnapshotBatch
	messages := []struct {
		name string
		set  func(protocol.Message) bool
	}{
		{name: "InventorySnapshot", set: func(message protocol.Message) bool { value, ok := message.(protocol.InventorySnapshot); if ok { batch.Inventory = value }; return ok }},
		{name: "InventoryInstanceSnapshot", set: func(message protocol.Message) bool { value, ok := message.(protocol.InventoryInstanceSnapshot); if ok { batch.InventoryInstance = value }; return ok }},
		{name: "EquipmentSnapshot", set: func(message protocol.Message) bool { value, ok := message.(protocol.EquipmentSnapshot); if ok { batch.Equipment = value }; return ok }},
		{name: "EquipmentInstanceSnapshot", set: func(message protocol.Message) bool { value, ok := message.(protocol.EquipmentInstanceSnapshot); if ok { batch.EquipmentInstance = value }; return ok }},
		{name: "AppearanceSnapshot", set: func(message protocol.Message) bool { value, ok := message.(protocol.AppearanceSnapshot); if ok { batch.Appearance = value }; return ok }},
	}
	for index, expected := range messages {
		envelope := <-connection.Reliable()
		if !expected.set(envelope.Message) {
			t.Fatalf("owner snapshot[%d]=%T %#v want %s", index, envelope.Message, envelope.Message, expected.name)
		}
	}
	return batch
}
