package respawnpolicy

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLoadIsStrictAndRequiresAllDeathContexts(t *testing.T) {
	valid := `{
		"schema_version":2,
		"revision":"test-002",
		"spawn_points":[
			{"id":"safe","class":"safe","x":0,"y":0,"z":0,"layer":0,"checkpoint_activation_radius":0},
			{"id":"checkpoint","class":"checkpoint","x":10,"y":0,"z":0,"layer":0,"checkpoint_activation_radius":4},
			{"id":"siege","class":"siege","x":-10,"y":0,"z":0,"layer":0,"checkpoint_activation_radius":0}
		],
		"contexts":[
			{"context":"pve","respawn_delay_seconds":1.25,"default_spawn_point":"safe","allowed_spawn_classes":["safe","checkpoint"]},
			{"context":"pvp","respawn_delay_seconds":2,"default_spawn_point":"safe","allowed_spawn_classes":["safe"]},
			{"context":"siege","respawn_delay_seconds":3,"default_spawn_point":"siege","allowed_spawn_classes":["safe","siege"]}
		]
	}`
	loaded, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Revision != "test-002" {
		t.Fatalf("revision=%q", loaded.Definition.Revision)
	}

	unknown := strings.Replace(valid, `"revision":"test-002"`, `"revision":"test-002","extra":true`, 1)
	if _, err := Load(strings.NewReader(unknown)); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown field err=%v", err)
	}

	missingSiege := loaded.Definition
	missingSiege.Contexts = append([]ContextRule(nil), loaded.Definition.Contexts[:2]...)
	if err := Validate(missingSiege); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("missing siege context err=%v", err)
	}
}

func TestValidateRejectsContextDefaultOutsideAllowedClass(t *testing.T) {
	definition := testDefinition()
	definition.Contexts[1].DefaultSpawnPoint = "checkpoint"
	if err := Validate(definition); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("default class err=%v", err)
	}
}

func TestValidateAgainstWorldRejectsOffSurfaceAndBlocker(t *testing.T) {
	gameplay := testGameplayDefinition()
	valid := testDefinition()
	if err := ValidateAgainstWorld(valid, gameplay); err != nil {
		t.Fatal(err)
	}

	offSurface := valid
	offSurface.SpawnPoints = append([]SpawnPoint(nil), valid.SpawnPoints...)
	offSurface.SpawnPoints[0].X = 100
	if err := ValidateAgainstWorld(offSurface, gameplay); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("off surface err=%v", err)
	}

	blocked := valid
	blocked.SpawnPoints = append([]SpawnPoint(nil), valid.SpawnPoints...)
	blocked.SpawnPoints[0].X = 5
	blocked.SpawnPoints[0].Z = 5
	if err := ValidateAgainstWorld(blocked, gameplay); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("blocked err=%v", err)
	}
}

func TestAcquireCheckpointRequiresClassLayerAndRadius(t *testing.T) {
	service, err := NewService(testDefinition(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AcquireCheckpoint(7, world.Position{X: 8, Layer: 0}, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if checkpoint, ok := service.Checkpoint(7); !ok || checkpoint != "checkpoint" {
		t.Fatalf("checkpoint=%q ok=%v", checkpoint, ok)
	}
	if err := service.AcquireCheckpoint(8, world.Position{X: 8, Layer: 1}, "checkpoint"); !errors.Is(err, ErrCheckpointWrongLayer) {
		t.Fatalf("wrong layer err=%v", err)
	}
	if err := service.AcquireCheckpoint(8, world.Position{X: 0, Layer: 0}, "checkpoint"); !errors.Is(err, ErrCheckpointOutOfRange) {
		t.Fatalf("out of range err=%v", err)
	}
	if err := service.AcquireCheckpoint(8, world.Position{X: 0, Layer: 0}, "safe"); !errors.Is(err, ErrCheckpointNotAcquirable) {
		t.Fatalf("safe point err=%v", err)
	}
}

func TestScheduleBindsContextDelayAndAllowedSpawnClass(t *testing.T) {
	service, err := NewService(testDefinition(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if delay, ok := service.DelayTicks(DeathContextPvE); !ok || delay != 25 {
		t.Fatalf("pve delay=%d ok=%v", delay, ok)
	}
	if delay, ok := service.DelayTicks(DeathContextPvP); !ok || delay != 40 {
		t.Fatalf("pvp delay=%d ok=%v", delay, ok)
	}
	if delay, ok := service.DelayTicks(DeathContextSiege); !ok || delay != 60 {
		t.Fatalf("siege delay=%d ok=%v", delay, ok)
	}

	for _, entityID := range []world.EntityID{3, 7, 9} {
		if err := service.AcquireCheckpoint(entityID, world.Position{X: 10, Layer: 0}, "checkpoint"); err != nil {
			t.Fatal(err)
		}
	}
	pve, err := service.Schedule(7, 100, DeathContextPvE)
	if err != nil {
		t.Fatal(err)
	}
	if pve.Context != DeathContextPvE || pve.SpawnPointID != "checkpoint" || pve.SpawnClass != SpawnClassCheckpoint || pve.DueTick != 125 {
		t.Fatalf("pve=%#v", pve)
	}
	pvp, err := service.Schedule(3, 100, DeathContextPvP)
	if err != nil {
		t.Fatal(err)
	}
	if pvp.Context != DeathContextPvP || pvp.SpawnPointID != "safe" || pvp.SpawnClass != SpawnClassSafe || pvp.DueTick != 140 {
		t.Fatalf("pvp=%#v", pvp)
	}
	siege, err := service.Schedule(9, 100, DeathContextSiege)
	if err != nil {
		t.Fatal(err)
	}
	if siege.Context != DeathContextSiege || siege.SpawnPointID != "siege" || siege.SpawnClass != SpawnClassSiege || siege.DueTick != 160 {
		t.Fatalf("siege=%#v", siege)
	}

	// Death-time binding：清除 checkpoint 不會改寫已排定的 PvE結果。
	service.ClearCheckpoint(7)
	pending, ok := service.Pending(7)
	if !ok || pending.SpawnPointID != "checkpoint" {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}
	if due := service.Due(124); len(due) != 0 {
		t.Fatalf("early due=%#v", due)
	}
	due := service.Due(160)
	if len(due) != 3 || due[0].EntityID != 7 || due[1].EntityID != 3 || due[2].EntityID != 9 {
		t.Fatalf("due order=%#v", due)
	}
	if _, ok := service.Pending(7); !ok {
		t.Fatal("Due selection removed pending truth")
	}
}

func TestRemoveClearsCheckpointAndPending(t *testing.T) {
	service, err := NewService(testDefinition(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AcquireCheckpoint(9, world.Position{X: 10, Layer: 0}, "checkpoint"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Schedule(9, 1, DeathContextPvE); err != nil {
		t.Fatal(err)
	}
	service.Remove(9)
	if _, ok := service.Checkpoint(9); ok {
		t.Fatal("checkpoint still present")
	}
	if _, ok := service.Pending(9); ok {
		t.Fatal("pending still present")
	}
}

func testDefinition() Definition {
	return Definition{
		SchemaVersion: SchemaVersion,
		Revision:      "test-002",
		SpawnPoints: []SpawnPoint{
			{ID: "safe", Class: SpawnClassSafe, X: 0, Y: 0, Z: 0, Layer: 0},
			{ID: "checkpoint", Class: SpawnClassCheckpoint, X: 10, Y: 0, Z: 0, Layer: 0, CheckpointActivationRadius: 4},
			{ID: "siege", Class: SpawnClassSiege, X: -10, Y: 0, Z: 0, Layer: 0},
		},
		Contexts: []ContextRule{
			{Context: DeathContextPvE, RespawnDelaySeconds: 1.25, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []SpawnClass{SpawnClassSafe, SpawnClassCheckpoint}},
			{Context: DeathContextPvP, RespawnDelaySeconds: 2, DefaultSpawnPoint: "safe", AllowedSpawnClasses: []SpawnClass{SpawnClassSafe}},
			{Context: DeathContextSiege, RespawnDelaySeconds: 3, DefaultSpawnPoint: "siege", AllowedSpawnClasses: []SpawnClass{SpawnClassSafe, SpawnClassSiege}},
		},
	}
}

func testGameplayDefinition() gameplayworld.Definition {
	return gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID:       "test-world",
		Revision:      "world-001",
		Units:         "meters",
		Agent:         gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{
			ID:     "ground",
			Layer:  0,
			Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
			Plane:  gameplayworld.SurfacePlane{BaseY: 0},
		}},
		Blockers: []gameplayworld.Blocker{{
			ID:             "blocked",
			Layer:          0,
			Bounds:         gameplayworld.BoundsXZ{MinX: 4, MaxX: 6, MinZ: 4, MaxZ: 6},
			MinY:           0,
			MaxY:           2,
			BlocksMovement: true,
			Enabled:        true,
		}},
	}
}
