package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestTrustedRestoreBootstrapReplicatesActiveSkinWeaponAffinity(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, err := characteridentity.NewTrusted("character:appearance-bootstrap")
	if err != nil { t.Fatal(err) }
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil { t.Fatal(err) }
	inventoryState, err := characterstate.NewInventoryStateWithEquipment(nil, "item_militia_iron_sword", "")
	if err != nil { t.Fatal(err) }
	restore := CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID: identity.ID,
		Revision: 8,
		World: characterRestoreWorld,
		HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Inventory: inventoryState,
		PrimaryStats: characterstats.DefaultPrimary(),
		SkinID: appearance.KnightDPelegrini,
		Transform: world.Transform{Position: world.Position{Layer: 4}},
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }

	batch := readOwnerSnapshotBatch(t, conn)
	if len(batch.InventoryInstance.Items) != 0 || len(batch.EquipmentInstance.Slots) != 0 { t.Fatalf("low-tier restore leaked unique state=%#v", batch) }
	if len(batch.Equipment.Slots) != 1 || batch.Equipment.Slots[0].Slot != protocol.EquipmentSlotMainHand || batch.Equipment.Slots[0].ItemArchetypeID != "item_militia_iron_sword" { t.Fatalf("restored equipment=%#v", batch.Equipment) }
	if batch.Appearance.SkinID != appearance.KnightDPelegrini || batch.Appearance.BasicAttackAffinityBonus != 1 { t.Fatalf("appearance bootstrap=%#v want skin=%q bonus=1", batch.Appearance, appearance.KnightDPelegrini) }
}
