package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLegacyRuntimeClassDoesNotEnterDurableLeaveSnapshot(t *testing.T) {
	outbox, err := characterstate.NewOutbox(8)
	if err != nil { t.Fatal(err) }
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 4}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig(); cfg.SnapshotEveryTicks = 1000
	worldRef := characterstate.WorldRef{WorldID: characterRestoreWorld.WorldID, Revision: characterRestoreWorld.Revision, GameplaySHA256: characterRestoreWorld.GameplaySHA256}
	rt := New(sim, cfg, WithCharacterStateOutbox(outbox, worldRef))
	identity, err := characteridentity.NewTrusted("character:class-roundtrip"); if err != nil { t.Fatal(err) }
	connection := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, connection); if err != nil { t.Fatal(err) }
	restore := CharacterRestore{
		SchemaVersion: characterstate.LearnedSkillsSchemaVersion, CharacterID: identity.ID, Revision: 1,
		World: characterRestoreWorld, LegacyRuntimeClassID: classid.Oathguard,
		HP: 900, MaxHP: 1000, MP: 80, MaxMP: 100,
		Transform: world.Transform{Position: world.Position{Layer: 4}},
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors=%#v", report.CommandErrors) }
	state, ok := rt.characters.State(1)
	if !ok || state.ClassID != classid.Oathguard { t.Fatalf("runtime state=%#v ok=%v", state, ok) }
	if err := rt.EnqueueLeave(sess.ID); err != nil { t.Fatal(err) }
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("leave errors=%#v", report.CommandErrors) }
	pending := outbox.Pending(0)
	if len(pending) != 1 { t.Fatalf("save intents=%#v", pending) }
}

func TestValidateCharacterRestoreLegacyRuntimeClassIsSchemaBound(t *testing.T) {
	identity, err := characteridentity.NewTrusted("character:class-validation"); if err != nil { t.Fatal(err) }
	base := CharacterRestore{
		SchemaVersion: characterstate.ClassSchemaVersion, CharacterID: identity.ID, Revision: 1,
		World: characterRestoreWorld, LegacyRuntimeClassID: classid.Oathguard,
		HP: 1, MaxHP: 1, MP: 1, MaxMP: 1,
	}
	if err := ValidateCharacterRestore(identity, base, characterRestoreWorld); err != nil { t.Fatalf("canonical legacy restore rejected: %v", err) }
	unknown := base; unknown.LegacyRuntimeClassID = classid.ID("class_guard")
	if err := ValidateCharacterRestore(identity, unknown, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("unknown class err=%v", err) }
	preClass := base; preClass.SchemaVersion = characterstate.EquipmentSchemaVersion
	if err := ValidateCharacterRestore(identity, preClass, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("pre-class class err=%v", err) }
	classless := base; classless.SchemaVersion = characterstate.ClasslessSchemaVersion
	if err := ValidateCharacterRestore(identity, classless, characterRestoreWorld); !errors.Is(err, ErrCharacterRestoreInvalid) { t.Fatalf("classless class smuggle err=%v", err) }
	classless.LegacyRuntimeClassID = ""
	if err := ValidateCharacterRestore(identity, classless, characterRestoreWorld); err != nil { t.Fatalf("classless restore rejected: %v", err) }
}
