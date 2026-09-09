package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCharacterStateSaveCapturesInitializedInventoryAndMainHand(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeCharacterStateRuntime(t, outbox)
	identity, err := characteridentity.NewTrusted("character:inventory-capture")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}

	inv := rt.inventories[identity.ID]
	if inv == nil {
		t.Fatal("authoritative inventory missing after register")
	}
	if err := inv.EquipMainHand(trainingBladeArchetypeID); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueLeave(s.ID); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 || report.Metrics.CharacterStateSaveIntentFailures != 0 {
		t.Fatalf("leave report=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}

	persisted := pending[0].Snapshot.Inventory
	if !persisted.Initialized {
		t.Fatalf("persisted inventory must be initialized: %#v", persisted)
	}
	if persisted.MainHand != trainingBladeArchetypeID {
		t.Fatalf("main hand=%q, want %q", persisted.MainHand, trainingBladeArchetypeID)
	}
	stacks, err := persisted.Stacks()
	if err != nil {
		t.Fatal(err)
	}
	quantities := make(map[string]uint32, len(stacks))
	for _, stack := range stacks {
		quantities[stack.ItemArchetypeID] = stack.Quantity
	}
	if quantities["item_minor_healing_potion"] != 5 || quantities["item_minor_mana_potion"] != 3 {
		t.Fatalf("persisted stacks=%#v", stacks)
	}
	if quantities[trainingBladeArchetypeID] != 0 {
		t.Fatalf("equipped main hand was duplicated in unequipped stacks: %#v", stacks)
	}
}

func TestCharacterStateSaveMissingInventoryFailsClosed(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeCharacterStateRuntime(t, outbox)
	identity, err := characteridentity.NewTrusted("character:inventory-missing")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}
	delete(rt.inventories, identity.ID)

	if err := rt.EnqueueLeave(s.ID); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || report.CommandErrors[0].Command != "enqueue_character_state_save" {
		t.Fatalf("save errors=%#v", report.CommandErrors)
	}
	if report.Metrics.CharacterStateSaveIntentsEnqueued != 0 || report.Metrics.CharacterStateSaveIntentFailures != 1 || outbox.Depth() != 0 {
		t.Fatalf("report=%#v outbox_depth=%d", report, outbox.Depth())
	}
	if _, ok := rt.sessions.Get(s.ID); ok {
		t.Fatal("save failure incorrectly kept the session registered")
	}
	if _, ok := rt.world.Entity(s.EntityID); ok {
		t.Fatal("save failure incorrectly kept the world entity alive")
	}
}

func TestJoinLegacyUninitializedInventoryBootstrapsStarterItems(t *testing.T) {
	rt := makeRestoreRuntime(t)
	rt.config.StarterInventory = []inventory.Stack{
		{ArchetypeID: "item_minor_healing_potion", Quantity: 9},
		{ArchetypeID: trainingBladeArchetypeID, Quantity: 1},
	}
	identity, err := characteridentity.NewTrusted("character:legacy-inventory-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	restore := CharacterRestore{
		SchemaVersion: characterstate.ResourceSchemaVersion,
		CharacterID:   identity.ID,
		Revision:      1,
		World:         characterRestoreWorld,
		HP:            1000,
		MaxHP:         1000,
		MP:            100,
		MaxMP:         100,
		Transform:     world.Transform{Position: world.Position{Layer: 4}},
		Inventory:     characterstate.InventoryState{},
	}
	if err := rt.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
		Restore: &restore,
	}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	inv := rt.inventories[identity.ID]
	if inv == nil {
		t.Fatal("starter inventory missing after legacy restore")
	}
	if inv.Quantity("item_minor_healing_potion") != 9 || inv.Quantity(trainingBladeArchetypeID) != 1 || inv.MainHand() != "" {
		t.Fatalf("legacy bootstrap inventory=%#v main_hand=%q", inv.Snapshot(), inv.MainHand())
	}
}

func TestHealingPotionUsePersistsAcrossSaveAndRestore(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeCharacterStateRuntime(t, outbox)
	identity, err := characteridentity.NewTrusted("character:item-use-continuity")
	if err != nil {
		t.Fatal(err)
	}
	conn := session.NewQueueConnection(32, 32)
	s, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(s); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}
	if _, err := rt.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("use-item errors=%#v", report.CommandErrors)
	}
	state, ok := rt.characters.State(s.EntityID)
	if !ok || state.HP != 850 {
		t.Fatalf("post-use state=%#v ok=%v", state, ok)
	}
	if got := rt.inventories[identity.ID].Quantity("item_minor_healing_potion"); got != 4 {
		t.Fatalf("post-use healing potion quantity=%d, want 4", got)
	}

	if err := rt.EnqueueLeave(s.ID); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
		t.Fatalf("leave report=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	intent := pending[0]
	if intent.Snapshot.HP != 850 || !intent.Snapshot.Inventory.Initialized {
		t.Fatalf("saved snapshot=%#v", intent.Snapshot)
	}
	stacks, err := intent.Snapshot.Inventory.Stacks()
	if err != nil {
		t.Fatal(err)
	}
	persistedHealing := uint32(0)
	for _, stack := range stacks {
		if stack.ItemArchetypeID == "item_minor_healing_potion" {
			persistedHealing = stack.Quantity
			break
		}
	}
	if persistedHealing != 4 {
		t.Fatalf("saved healing potion quantity=%d, want 4", persistedHealing)
	}

	restore := CharacterRestoreFromRecord(characterstate.Record{
		SchemaVersion: characterstate.InventorySchemaVersion,
		CharacterID:   identity.ID,
		Revision:      1,
		Snapshot:      intent.Snapshot,
	})
	rt2 := makeRestoreRuntime(t)
	conn2 := session.NewQueueConnection(32, 32)
	s2, err := session.NewWithCharacterIdentity(2, 2, identity, 64, conn2)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt2.EnqueueJoin(JoinRequest{
		Session: s2,
		Entity: world.EntityState{ID: 2, Kind: world.EntityPlayer},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
		Restore: &restore,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt2.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("restore join errors=%#v", report.CommandErrors)
	}
	restoredState, ok := rt2.characters.State(s2.EntityID)
	if !ok || restoredState.HP != 850 {
		t.Fatalf("restored state=%#v ok=%v", restoredState, ok)
	}
	restoredInventory := rt2.inventories[identity.ID]
	if restoredInventory == nil || restoredInventory.Quantity("item_minor_healing_potion") != 4 {
		t.Fatalf("restored inventory=%#v", restoredInventory)
	}
}
