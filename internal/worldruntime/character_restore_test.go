package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const characterRestoreTestSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var characterRestoreWorld = protocol.WorldIdentity{
	WorldID: "castle-sandbox", Revision: "s3d-001", GameplaySHA256: characterRestoreTestSHA,
}

func TestJoinRestoresTrustedAliveCharacterAtomically(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-alpha")
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil { t.Fatal(err) }
	primary := characterstats.DefaultPrimary()
	primary.Strength = 18
	primary.Agility = 16
	restore := CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID: identity.ID,
		Revision: 7,
		World: characterRestoreWorld,
		MapID: "map1",
		HP: 640, MaxHP: 1200,
		MP: 45, MaxMP: 100,
		PrimaryStats: primary,
		SkinID: appearance.PeasantGirl,
		Transform: world.Transform{Position: world.Position{X: 21, Y: 0, Z: -8, Layer: 4}, Yaw: 1.5},
		Warehouse: characterstate.WarehouseState{Initialized: true, Items: []characterstate.WarehouseStack{{ItemArchetypeID: "item_minor_healing_potion", Quantity: 9}}},
	}
	bootstrap := world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: -50, Layer: 4}}}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: bootstrap, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }
	state, ok := rt.characters.State(1)
	if !ok || state.HP != 640 || state.MaxHP != 1200 || state.MP != 45 || state.MaxMP != 100 || state.Defeated || state.PrimaryStats != primary { t.Fatalf("character state=%#v ok=%v", state, ok) }
	entity, ok := rt.world.Entity(1)
	if !ok || entity.Transform != restore.Transform { t.Fatalf("entity=%#v ok=%v", entity, ok) }
	if got, ok := rt.sessions.Get(1); !ok || got != sess { t.Fatalf("session=%#v ok=%v", got, ok) }
	if got := rt.characterSkills.appearanceID(1); got != appearance.PeasantGirl { t.Fatalf("runtime skin=%q want=%q", got, appearance.PeasantGirl) }
	storage := rt.warehouses[identity.ID]
	if storage == nil || storage.Quantity("item_minor_healing_potion") != 9 { t.Fatalf("runtime warehouse=%#v", storage) }
	binding, snapshot, ok := rt.captureCharacterStateSnapshot(sess.ID, sess.EntityID, nil)
	if !ok { t.Fatal("capture restored character state failed") }
	if binding.ID != identity.ID || snapshot.SkinID != appearance.PeasantGirl || snapshot.World.MapID != "map1" { t.Fatalf("captured binding=%#v snapshot=%#v", binding, snapshot) }
	if len(snapshot.Warehouse.Items) != 1 || snapshot.Warehouse.Items[0].ItemArchetypeID != "item_minor_healing_potion" || snapshot.Warehouse.Items[0].Quantity != 9 { t.Fatalf("captured warehouse=%#v", snapshot.Warehouse) }
}

func TestJoinRejectsRestoreWorldMismatchBeforeSpawn(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-world")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	restore := CharacterRestore{SchemaVersion: characterstate.SchemaVersion, CharacterID: identity.ID, Revision: 1, World: protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "other", GameplaySHA256: characterRestoreTestSHA}, MapID: "map1", HP: 500, MaxHP: 1000, MP: 100, MaxMP: 100, PrimaryStats: characterstats.DefaultPrimary(), Warehouse: characterstate.EmptyWarehouseState(), Transform: world.Transform{Position: world.Position{Layer: 4}}}
	request := JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}
	if err := rt.EnqueueJoin(request); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreWorldMismatch) { t.Fatalf("errors=%#v", report.CommandErrors) }
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
}

func TestJoinRejectsRestoreMapMismatchBeforeSpawn(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-map")
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, session.NewQueueConnection(32, 32))
	restore := CharacterRestore{SchemaVersion: characterstate.SchemaVersion, CharacterID: identity.ID, Revision: 1, World: characterRestoreWorld, MapID: "map0", HP: 500, MaxHP: 1000, MP: 100, MaxMP: 100, PrimaryStats: characterstats.DefaultPrimary(), Warehouse: characterstate.EmptyWarehouseState(), Transform: world.Transform{Position: world.Position{Layer: 4}}}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreMapMismatch) { t.Fatalf("errors=%#v", report.CommandErrors) }
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
}

func TestJoinRejectsDefeatedRestoreBeforeSpawn(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-defeated")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	restore := CharacterRestore{CharacterID: identity.ID, Revision: 2, World: characterRestoreWorld, MapID: "map1", HP: 0, MaxHP: 1000, MP: 100, MaxMP: 100, Defeated: true, PrimaryStats: characterstats.DefaultPrimary(), Transform: world.Transform{Position: world.Position{Layer: 4}}}
	request := JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}
	if err := rt.EnqueueJoin(request); err != nil { t.Fatal(err) }
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreDefeatedUnsupported) { t.Fatalf("errors=%#v", report.CommandErrors) }
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
}

func TestValidateCharacterRestoreRequiresTrustedMatchingIdentity(t *testing.T) {
	trusted, _ := characteridentity.NewTrusted("character:trusted")
	restore := CharacterRestore{CharacterID: trusted.ID, Revision: 1, World: characterRestoreWorld, MapID: "map1", HP: 1, MaxHP: 1, MP: 1, MaxMP: 1, PrimaryStats: characterstats.DefaultPrimary()}
	if err := ValidateCharacterRestore(trusted, restore, characterRestoreWorld); err != nil { t.Fatal(err) }
	other, _ := characteridentity.NewTrusted("character:other")
	if err := ValidateCharacterRestore(other, restore, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreIdentityMismatch) { t.Fatalf("identity err=%v", err) }
	ephemeral, _ := characteridentity.NewEphemeral()
	if err := ValidateCharacterRestore(ephemeral, restore, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreRequiresTrustedIdentity) { t.Fatalf("ephemeral err=%v", err) }
	missingMap := restore; missingMap.SchemaVersion = characterstate.SchemaVersion; missingMap.MapID = ""; missingMap.Warehouse = characterstate.EmptyWarehouseState()
	if err := ValidateCharacterRestore(trusted, missingMap, characterRestoreWorld); err != nil { t.Fatalf("missing in-memory map should default to map1: %v", err) }
	mapID, ok := resolvedRestoreMapID(missingMap, characterRestoreWorld)
	if !ok || mapID != gameplayworld.MapIDStarterVillage { t.Fatalf("resolved map=%q ok=%v", mapID, ok) }
}

func TestValidateCharacterRestoreRejectsWarehouseAcrossSchemaBoundary(t *testing.T) {
	trusted, _ := characteridentity.NewTrusted("character:restore-warehouse-boundary")
	base := CharacterRestore{CharacterID: trusted.ID, Revision: 1, World: characterRestoreWorld, MapID: "map1", HP: 1, MaxHP: 1, MP: 1, MaxMP: 1, PrimaryStats: characterstats.DefaultPrimary()}
	legacy := base; legacy.SchemaVersion = characterstate.MapSchemaVersion; legacy.Warehouse = characterstate.EmptyWarehouseState()
	if err := ValidateCharacterRestore(trusted, legacy, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("legacy warehouse err=%v", err) }
	current := base; current.SchemaVersion = characterstate.SchemaVersion
	if err := ValidateCharacterRestore(trusted, current, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("uninitialized v15 warehouse err=%v", err) }
	current.Warehouse = characterstate.WarehouseState{Initialized: true, Items: []characterstate.WarehouseStack{{ItemArchetypeID: "z_item", Quantity: 1}, {ItemArchetypeID: "a_item", Quantity: 2}}}
	if err := ValidateCharacterRestore(trusted, current, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("noncanonical v15 warehouse err=%v", err) }
	current.Warehouse = characterstate.WarehouseState{Initialized: true, Items: []characterstate.WarehouseStack{{ItemArchetypeID: "a_item", Quantity: 2}, {ItemArchetypeID: "z_item", Quantity: 1}}}
	if err := ValidateCharacterRestore(trusted, current, characterRestoreWorld); err != nil { t.Fatalf("canonical v15 warehouse err=%v", err) }
}

func TestValidateCharacterRestoreRejectsStatSmugglingAcrossSchemaBoundary(t *testing.T) {
	trusted, _ := characteridentity.NewTrusted("character:restore-stat-boundary")
	base := CharacterRestore{CharacterID: trusted.ID, Revision: 1, World: characterRestoreWorld, MapID: "map1", HP: 1, MaxHP: 1, MP: 1, MaxMP: 1, PrimaryStats: characterstats.DefaultPrimary()}
	legacy := base; legacy.SchemaVersion = characterstate.ItemInstanceSchemaVersion; legacy.PrimaryStats.Strength = 11
	if err := ValidateCharacterRestore(trusted, legacy, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("legacy err=%v", err) }
	interim := base; interim.SchemaVersion = characterstate.PrimaryStatsSchemaVersion; interim.PrimaryStats.Strength = 17; interim.PrimaryStats.Agility = 18
	if err := ValidateCharacterRestore(trusted, interim, characterRestoreWorld); err != nil { t.Fatalf("v11 err=%v", err) }
	interim.PrimaryStats.Charisma = 11
	if err := ValidateCharacterRestore(trusted, interim, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("v11 smuggle err=%v", err) }
	current := base; current.SchemaVersion = characterstate.SixPrimaryStatsSchemaVersion; current.PrimaryStats.Charisma = 11
	if err := ValidateCharacterRestore(trusted, current, characterRestoreWorld); err != nil { t.Fatalf("v12 err=%v", err) }
}

func makeRestoreRuntime(t *testing.T) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 1
	worldRef := characterstate.WorldRef{MapID: "map1", WorldID: characterRestoreWorld.WorldID, Revision: characterRestoreWorld.Revision, GameplaySHA256: characterRestoreTestSHA}
	return New(sim, cfg, WithCharacterStateOutbox(nil, worldRef))
}

func assertRestoreJoinDidNotPartiallySpawn(t *testing.T, rt *Runtime) {
	t.Helper()
	if _, ok := rt.world.Entity(1); ok { t.Fatal("invalid restore partially spawned world entity") }
	if _, ok := rt.characters.State(1); ok { t.Fatal("invalid restore partially registered character state") }
	if _, ok := rt.sessions.Get(1); ok { t.Fatal("invalid restore partially registered session") }
}
