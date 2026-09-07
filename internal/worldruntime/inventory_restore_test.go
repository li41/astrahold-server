package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestJoinRestoresInitializedInventoryAndMainHandBeforeFirstSnapshot(t *testing.T) {
	rt := makeRestoreRuntime(t)
	rt.config.StarterInventory = []inventory.Stack{{ArchetypeID: "item_minor_healing_potion", Quantity: 9}}
	identity, _ := characteridentity.NewTrusted("character:restore-inventory")
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil { t.Fatal(err) }
	persisted, err := characterstate.NewInventoryState([]characterstate.InventoryStack{
		{ItemArchetypeID: "item_minor_healing_potion", Quantity: 2},
		{ItemArchetypeID: "item_minor_mana_potion", Quantity: 3},
	}, "item_training_blade")
	if err != nil { t.Fatal(err) }
	restore := CharacterRestore{
		SchemaVersion: characterstate.InventorySchemaVersion,
		CharacterID: identity.ID, Revision: 4, World: characterRestoreWorld,
		HP: 700, MaxHP: 1000, MP: 60, MaxMP: 100,
		Transform: world.Transform{Position: world.Position{Layer: 4}}, Inventory: persisted,
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID:1,Kind:world.EntityPlayer}, Speed:6,Radius:0.35,MaxStepHeight:0.5,Restore:&restore}); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("errors=%#v", report.CommandErrors) }
	inv := rt.inventories[identity.ID]
	if inv == nil { t.Fatal("restored inventory missing") }
	if inv.MainHand() != "item_training_blade" || inv.Quantity("item_minor_healing_potion") != 2 || inv.Quantity("item_minor_mana_potion") != 3 {
		t.Fatalf("mainhand=%q inventory=%#v", inv.MainHand(), inv.Snapshot())
	}
	if inv.Quantity("item_minor_healing_potion") == 9 { t.Fatal("starter inventory replaced durable truth") }
}

func TestJoinInitializedEmptyInventoryDoesNotBootstrapStarterItems(t *testing.T) {
	rt := makeRestoreRuntime(t)
	rt.config.StarterInventory = []inventory.Stack{{ArchetypeID:"item_minor_healing_potion",Quantity:9}}
	identity, _ := characteridentity.NewTrusted("character:restore-empty-inventory")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	empty, err := characterstate.NewInventoryState(nil, "")
	if err != nil { t.Fatal(err) }
	restore := CharacterRestore{SchemaVersion:characterstate.InventorySchemaVersion,CharacterID:identity.ID,Revision:1,World:characterRestoreWorld,HP:1000,MaxHP:1000,MP:100,MaxMP:100,Transform:world.Transform{Position:world.Position{Layer:4}},Inventory:empty}
	if err := rt.EnqueueJoin(JoinRequest{Session:sess,Entity:world.EntityState{ID:1,Kind:world.EntityPlayer},Speed:6,Radius:0.35,MaxStepHeight:0.5,Restore:&restore});err!=nil{t.Fatal(err)}
	report:=rt.Step(1,50*time.Millisecond);if len(report.CommandErrors)!=0{t.Fatalf("errors=%#v",report.CommandErrors)}
	inv:=rt.inventories[identity.ID];if inv==nil{t.Fatal("empty durable inventory missing")};if len(inv.Snapshot())!=0||inv.MainHand()!=""{t.Fatalf("inventory=%#v mainhand=%q",inv.Snapshot(),inv.MainHand())}
}

func TestJoinRejectsIllegalDurableMainHandBeforeWorldMutation(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-illegal-mainhand")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	persisted, err := characterstate.NewInventoryState(nil, "item_forbidden_sword")
	if err != nil { t.Fatal(err) }
	restore := CharacterRestore{SchemaVersion:characterstate.InventorySchemaVersion,CharacterID:identity.ID,Revision:1,World:characterRestoreWorld,HP:1000,MaxHP:1000,MP:100,MaxMP:100,Transform:world.Transform{Position:world.Position{Layer:4}},Inventory:persisted}
	if err := rt.EnqueueJoin(JoinRequest{Session:sess,Entity:world.EntityState{ID:1,Kind:world.EntityPlayer},Speed:6,Radius:0.35,MaxStepHeight:0.5,Restore:&restore});err!=nil{t.Fatal(err)}
	report:=rt.Step(1,50*time.Millisecond)
	if len(report.CommandErrors)!=1||!errors.Is(report.CommandErrors[0].Err,ErrEquipmentItemNotAllowed){t.Fatalf("errors=%#v",report.CommandErrors)}
	assertRestoreJoinDidNotPartiallySpawn(t,rt)
	if _,ok:=rt.inventories[identity.ID];ok{t.Fatal("invalid restore left inventory residue")}
}
