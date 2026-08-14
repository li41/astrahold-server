package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
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
	if err != nil {
		t.Fatal(err)
	}
	restore := CharacterRestore{
		CharacterID: identity.ID,
		Revision:    7,
		World:       characterRestoreWorld,
		HP:          640,
		MaxHP:       1200,
		Transform: world.Transform{
			Position: world.Position{X: 21, Y: 0, Z: -8, Layer: 4},
			Yaw:      1.5,
		},
	}
	bootstrap := world.EntityState{
		ID: 1, Kind: world.EntityPlayer,
		Transform: world.Transform{Position: world.Position{X: -50, Layer: 4}},
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: bootstrap, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	state, ok := rt.characters.State(1)
	if !ok || state.HP != 640 || state.MaxHP != 1200 || state.Defeated {
		t.Fatalf("character state=%#v ok=%v", state, ok)
	}
	entity, ok := rt.world.Entity(1)
	if !ok || entity.Transform != restore.Transform {
		t.Fatalf("entity=%#v ok=%v", entity, ok)
	}
	if got, ok := rt.sessions.Get(1); !ok || got != sess {
		t.Fatalf("session=%#v ok=%v", got, ok)
	}
}

func TestJoinRejectsRestoreWorldMismatchBeforeSpawn(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-world")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	restore := CharacterRestore{
		CharacterID: identity.ID,
		Revision:    1,
		World:       protocol.WorldIdentity{WorldID: "castle-sandbox", Revision: "other", GameplaySHA256: characterRestoreTestSHA},
		HP:          500,
		MaxHP:       1000,
		Transform:   world.Transform{Position: world.Position{Layer: 4}},
	}
	request := JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}
	if err := rt.EnqueueJoin(request); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreWorldMismatch) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
}

func TestJoinRejectsDefeatedRestoreBeforeSpawn(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:restore-defeated")
	conn := session.NewQueueConnection(32, 32)
	sess, _ := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	restore := CharacterRestore{
		CharacterID: identity.ID,
		Revision:    2,
		World:       characterRestoreWorld,
		HP:          0,
		MaxHP:       1000,
		Defeated:    true,
		Transform:   world.Transform{Position: world.Position{Layer: 4}},
	}
	request := JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}
	if err := rt.EnqueueJoin(request); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreDefeatedUnsupported) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
}

func TestValidateCharacterRestoreRequiresTrustedMatchingIdentity(t *testing.T) {
	trusted, _ := characteridentity.NewTrusted("character:trusted")
	restore := CharacterRestore{CharacterID: trusted.ID, Revision: 1, World: characterRestoreWorld, HP: 1, MaxHP: 1}
	if err := ValidateCharacterRestore(trusted, restore, characterRestoreWorld); err != nil {
		t.Fatal(err)
	}
	other, _ := characteridentity.NewTrusted("character:other")
	if err := ValidateCharacterRestore(other, restore, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreIdentityMismatch) {
		t.Fatalf("identity err=%v", err)
	}
	ephemeral, _ := characteridentity.NewEphemeral()
	if err := ValidateCharacterRestore(ephemeral, restore, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreRequiresTrustedIdentity) {
		t.Fatalf("ephemeral err=%v", err)
	}
}

func makeRestoreRuntime(t *testing.T) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	worldRef := characterstate.WorldRef{
		WorldID: characterRestoreWorld.WorldID, Revision: characterRestoreWorld.Revision, GameplaySHA256: characterRestoreWorld.GameplaySHA256,
	}
	return New(sim, cfg, WithCharacterStateOutbox(nil, worldRef))
}

func assertRestoreJoinDidNotPartiallySpawn(t *testing.T, rt *Runtime) {
	t.Helper()
	if _, ok := rt.world.Entity(1); ok {
		t.Fatal("invalid restore partially spawned world entity")
	}
	if _, ok := rt.characters.State(1); ok {
		t.Fatal("invalid restore partially registered character state")
	}
	if _, ok := rt.sessions.Get(1); ok {
		t.Fatal("invalid restore partially registered session")
	}
}
