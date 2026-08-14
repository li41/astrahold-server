package respawnpolicy

import (
	"errors"
	"strings"
	"testing"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

func TestLoadIsStrictAndValidatesDefaultSpawnPoint(t *testing.T) {
	valid := `{
		"schema_version":1,
		"revision":"test-001",
		"respawn_delay_seconds":1.25,
		"default_spawn_point":"field",
		"spawn_points":[{"id":"field","x":0,"y":0,"z":0,"layer":0}]
	}`
	loaded, err := Load(strings.NewReader(valid))
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Definition.Revision != "test-001" {
		t.Fatalf("revision=%q", loaded.Definition.Revision)
	}

	unknown := strings.Replace(valid, `"revision":"test-001"`, `"revision":"test-001","extra":true`, 1)
	if _, err := Load(strings.NewReader(unknown)); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("unknown field err=%v", err)
	}

	missingDefault := strings.Replace(valid, `"default_spawn_point":"field"`, `"default_spawn_point":"missing"`, 1)
	if _, err := Load(strings.NewReader(missingDefault)); !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("missing default err=%v", err)
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

func TestScheduleBindsCheckpointAtDefeatAndDueIsDeterministic(t *testing.T) {
	definition := testDefinition()
	service, err := NewService(definition, 20)
	if err != nil {
		t.Fatal(err)
	}
	if service.DelayTicks() != 25 {
		t.Fatalf("delay ticks=%d", service.DelayTicks())
	}
	if err := service.SetCheckpoint(7, "courtyard"); err != nil {
		t.Fatal(err)
	}
	first, err := service.Schedule(7, 100)
	if err != nil {
		t.Fatal(err)
	}
	if first.SpawnPointID != "courtyard" || first.DueTick != 125 {
		t.Fatalf("scheduled=%#v", first)
	}

	// 已排定的死亡結果不因倒地後 checkpoint 改動而變更。
	service.ClearCheckpoint(7)
	pending, ok := service.Pending(7)
	if !ok || pending.SpawnPointID != "courtyard" {
		t.Fatalf("pending=%#v ok=%v", pending, ok)
	}
	if due := service.Due(124); len(due) != 0 {
		t.Fatalf("early due=%#v", due)
	}

	if _, err := service.Schedule(3, 100); err != nil {
		t.Fatal(err)
	}
	due := service.Due(125)
	if len(due) != 2 || due[0].EntityID != 3 || due[1].EntityID != 7 {
		t.Fatalf("due order=%#v", due)
	}
	// Due selection 不等於 transition 成功；pending 必須保留，直到 authoritative respawn確認。
	if _, ok := service.Pending(3); !ok {
		t.Fatal("due selection prematurely removed entity 3")
	}
	if _, ok := service.Pending(7); !ok {
		t.Fatal("due selection prematurely removed entity 7")
	}
	service.Cancel(3)
	if _, ok := service.Pending(3); ok {
		t.Fatal("cancel did not confirm pending removal")
	}
}

func TestCheckpointRejectsUnknownPointAndRemoveClearsState(t *testing.T) {
	service, err := NewService(testDefinition(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetCheckpoint(9, "missing"); !errors.Is(err, ErrUnknownSpawnPoint) {
		t.Fatalf("unknown checkpoint err=%v", err)
	}
	if err := service.SetCheckpoint(9, "courtyard"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Schedule(9, 1); err != nil {
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
		SchemaVersion:       SchemaVersion,
		Revision:            "test-001",
		RespawnDelaySeconds: 1.25,
		DefaultSpawnPoint:   "field",
		SpawnPoints: []SpawnPoint{
			{ID: "field", X: 0, Y: 0, Z: 0, Layer: 0},
			{ID: "courtyard", X: 10, Y: 0, Z: 10, Layer: 0},
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
			ID:    "ground",
			Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -20, MaxX: 20, MinZ: -20, MaxZ: 20},
			Plane: gameplayworld.SurfacePlane{BaseY: 0},
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
