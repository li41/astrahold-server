package deathpenalty

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLoadIsStrictAndValidatesCheckpointForfeitContexts(t *testing.T) {
	valid := `{
		"schema_version":1,
		"revision":"test-001",
		"checkpoint_forfeit_contexts":["pve"]
	}`
	loaded, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Revision != "test-001" || len(loaded.Definition.CheckpointForfeitContexts) != 1 {
		t.Fatalf("loaded=%#v", loaded.Definition)
	}

	unknown := strings.Replace(valid, `"revision":"test-001"`, `"revision":"test-001","extra":true`, 1)
	if _, err := Load(strings.NewReader(unknown)); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown field err=%v", err)
	}

	duplicate := loaded.Definition
	duplicate.CheckpointForfeitContexts = []respawnpolicy.DeathContext{respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvE}
	if err := Validate(duplicate); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("duplicate context err=%v", err)
	}

	invalid := loaded.Definition
	invalid.CheckpointForfeitContexts = []respawnpolicy.DeathContext{"arena"}
	if err := Validate(invalid); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("invalid context err=%v", err)
	}
}

func TestApplyIsExactlyOncePerDefeatRevision(t *testing.T) {
	service, err := NewService(Definition{
		SchemaVersion:             SchemaVersion,
		Revision:                  "test-001",
		CheckpointForfeitContexts: []respawnpolicy.DeathContext{respawnpolicy.DeathContextPvE},
	})
	if err != nil {
		t.Fatal(err)
	}

	decision, applied, err := service.Apply(7, 1, respawnpolicy.DeathContextPvE)
	if err != nil || !applied || !decision.ForfeitCheckpoint {
		t.Fatalf("first apply decision=%#v applied=%v err=%v", decision, applied, err)
	}
	if revision, ok := service.AppliedRevision(7); !ok || revision != 1 {
		t.Fatalf("applied revision=%d ok=%v", revision, ok)
	}

	decision, applied, err = service.Apply(7, 1, respawnpolicy.DeathContextPvE)
	if err != nil || applied || decision.ForfeitCheckpoint {
		t.Fatalf("duplicate apply decision=%#v applied=%v err=%v", decision, applied, err)
	}

	if _, _, err := service.Apply(7, 0, respawnpolicy.DeathContextPvE); !errors.Is(err, ErrInvalidRevision) {
		t.Fatalf("zero revision err=%v", err)
	}

	decision, applied, err = service.Apply(7, 2, respawnpolicy.DeathContextPvP)
	if err != nil || !applied || decision.ForfeitCheckpoint {
		t.Fatalf("pvp apply decision=%#v applied=%v err=%v", decision, applied, err)
	}
	if _, _, err := service.Apply(7, 1, respawnpolicy.DeathContextPvE); !errors.Is(err, ErrRevisionRegression) {
		t.Fatalf("revision regression err=%v", err)
	}
}

func TestRemoveClearsAppliedRevision(t *testing.T) {
	service, err := NewService(Definition{SchemaVersion: SchemaVersion, Revision: "test-001"})
	if err != nil {
		t.Fatal(err)
	}
	if _, applied, err := service.Apply(world.EntityID(9), 1, respawnpolicy.DeathContextSiege); err != nil || !applied {
		t.Fatalf("apply applied=%v err=%v", applied, err)
	}
	service.Remove(9)
	if _, ok := service.AppliedRevision(9); ok {
		t.Fatal("applied revision survived remove")
	}
	if _, applied, err := service.Apply(9, 1, respawnpolicy.DeathContextSiege); err != nil || !applied {
		t.Fatalf("entity reuse apply applied=%v err=%v", applied, err)
	}
}
