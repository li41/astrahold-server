package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/learnedskills"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/skillloadout"
	"github.com/li41/astrahold-server/internal/world"
)

func TestCharacterSkillStateRestoresSavesCurrentRuntimeAndClearsOnLeave(t *testing.T) {
	outbox, err := characterstate.NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	rt := makeRestoreRuntime(t)
	rt.characterStateOutbox = outbox
	identity, _ := characteridentity.NewTrusted("character:skill-lifecycle")
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	initialLearned := mustRuntimeLearnedSet(t, skillcatalog.HeavyStrike, skillcatalog.FireBolt, skillcatalog.Haste)
	initialLoadout := mustRuntimeCombatSlots(t, skillcatalog.HeavyStrike, skillcatalog.FireBolt)
	restore := CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID:   identity.ID,
		Revision:      4,
		World:         characterRestoreWorld,
		HP:            900,
		MaxHP:         1000,
		MP:            80,
		MaxMP:         100,
		Transform:     world.Transform{Position: world.Position{X: 5, Z: -2, Layer: 4}, Yaw: 0.75},
		CombatLoadout: initialLoadout,
		LearnedSkills: initialLearned,
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	gotLearned := mustRuntimeLearnedSet(t, rt.characterSkills.learned.Learned(1)...)
	if gotLearned != initialLearned || rt.characterSkills.loadout.Slots(1) != initialLoadout {
		t.Fatalf("restored learned=%v loadout=%v", gotLearned.IDs(), rt.characterSkills.loadout.Combat(1))
	}

	currentLearned := mustRuntimeLearnedSet(t, skillcatalog.HeavyStrike, skillcatalog.RapidShot, skillcatalog.FireBolt, skillcatalog.Haste)
	currentLoadout := mustRuntimeCombatSlots(t, skillcatalog.RapidShot, skillcatalog.FireBolt)
	if err := rt.characterSkills.learned.SetLearned(1, currentLearned.IDs()); err != nil {
		t.Fatal(err)
	}
	if err := rt.characterSkills.loadout.SetCombat(1, currentLoadout.IDs(), func(id skillcatalog.ID) bool {
		return rt.characterSkills.learned.Contains(1, id)
	}); err != nil {
		t.Fatal(err)
	}

	if err := rt.EnqueueLeave(1); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
		t.Fatalf("leave report=%#v", report)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	if pending[0].Snapshot.LearnedSkills != currentLearned || pending[0].Snapshot.CombatLoadout != currentLoadout {
		t.Fatalf("saved learned=%v loadout=%v", pending[0].Snapshot.LearnedSkills.IDs(), pending[0].Snapshot.CombatLoadout.IDs())
	}
	if rt.characterSkills.learned.Count(1) != 0 || rt.characterSkills.loadout.Slots(1) != (skillloadout.Slots{}) {
		t.Fatalf("teardown left learned=%v loadout=%v", rt.characterSkills.learned.Learned(1), rt.characterSkills.loadout.Combat(1))
	}
}

func TestJoinLateFailureRollsBackRestoredCharacterSkillState(t *testing.T) {
	rt := makeRestoreRuntime(t)
	existingConn := session.NewQueueConnection(8, 8)
	existing, err := session.New(1, 99, 16, existingConn)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.sessions.Add(existing); err != nil {
		t.Fatal(err)
	}

	identity, _ := characteridentity.NewTrusted("character:skill-rollback")
	conn := session.NewQueueConnection(32, 32)
	incoming, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	learned := mustRuntimeLearnedSet(t, skillcatalog.Cleave, skillcatalog.Meteor)
	loadout := mustRuntimeCombatSlots(t, skillcatalog.Cleave, skillcatalog.Meteor)
	restore := CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID: identity.ID, Revision: 2, World: characterRestoreWorld,
		HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Transform: world.Transform{Position: world.Position{Layer: 4}},
		CombatLoadout: loadout, LearnedSkills: learned,
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: incoming, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, session.ErrSessionExists) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	if _, ok := rt.world.Entity(1); ok {
		t.Fatal("late join failure left world entity")
	}
	if _, ok := rt.characters.State(1); ok {
		t.Fatal("late join failure left character state")
	}
	if rt.characterSkills.learned.Count(1) != 0 || rt.characterSkills.loadout.Slots(1) != (skillloadout.Slots{}) {
		t.Fatalf("late join failure left learned=%v loadout=%v", rt.characterSkills.learned.Learned(1), rt.characterSkills.loadout.Combat(1))
	}
	if got, ok := rt.sessions.Get(1); !ok || got != existing {
		t.Fatalf("existing session changed got=%#v ok=%v", got, ok)
	}
}

func TestJoinRejectsUnlearnedCombatLoadoutBeforeWorldMutation(t *testing.T) {
	rt := makeRestoreRuntime(t)
	identity, _ := characteridentity.NewTrusted("character:skill-invalid-restore")
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	restore := CharacterRestore{
		SchemaVersion: characterstate.SchemaVersion,
		CharacterID: identity.ID, Revision: 1, World: characterRestoreWorld,
		HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Transform: world.Transform{Position: world.Position{Layer: 4}},
		CombatLoadout: mustRuntimeCombatSlots(t, skillcatalog.FireBolt),
		LearnedSkills: mustRuntimeLearnedSet(t, skillcatalog.HeavyStrike),
	}
	if err := rt.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrCharacterRestoreInvalid) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	assertRestoreJoinDidNotPartiallySpawn(t, rt)
	if rt.characterSkills.learned.Count(1) != 0 || rt.characterSkills.loadout.Slots(1) != (skillloadout.Slots{}) {
		t.Fatalf("invalid restore left learned=%v loadout=%v", rt.characterSkills.learned.Learned(1), rt.characterSkills.loadout.Combat(1))
	}
}

func TestValidateCharacterRestoreAcceptsV7InferredLearnedSkills(t *testing.T) {
	identity, _ := characteridentity.NewTrusted("character:skill-v7")
	loadout := mustRuntimeCombatSlots(t, skillcatalog.HeavyStrike, skillcatalog.FireBolt)
	restore := CharacterRestore{
		SchemaVersion: characterstate.LoadoutSchemaVersion,
		CharacterID: identity.ID, Revision: 1, World: characterRestoreWorld,
		HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Transform: world.Transform{Position: world.Position{Layer: 4}},
		CombatLoadout: loadout,
		LearnedSkills: mustRuntimeLearnedSet(t, loadout.IDs()...),
	}
	if err := ValidateCharacterRestore(identity, restore, characterRestoreWorld); err != nil {
		t.Fatalf("v7 inferred restore err=%v", err)
	}
}

func mustRuntimeLearnedSet(t *testing.T, ids ...skillcatalog.ID) learnedskills.Set {
	t.Helper()
	set, err := learnedskills.NewSet(ids)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func mustRuntimeCombatSlots(t *testing.T, ids ...skillcatalog.ID) skillloadout.Slots {
	t.Helper()
	slots, err := skillloadout.NewSlots(ids)
	if err != nil {
		t.Fatal(err)
	}
	return slots
}
