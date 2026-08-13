package siege

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

type fakeWorld struct {
	blocker gameplayworld.Blocker
	enabled bool
	los     bool
}

func (f *fakeWorld) BlockerDefinition(id string) (gameplayworld.Blocker, error) {
	if id != f.blocker.ID { return gameplayworld.Blocker{}, errors.New("missing blocker") }
	return f.blocker, nil
}
func (f *fakeWorld) BlockerEnabled(id string) (bool, error) {
	if id != f.blocker.ID { return false, errors.New("missing blocker") }
	return f.enabled, nil
}
func (f *fakeWorld) SetBlockerEnabled(id string, enabled bool) error {
	if id != f.blocker.ID { return errors.New("missing blocker") }
	f.enabled = enabled
	return nil
}
func (f *fakeWorld) HasLineOfSightIgnoringBlocker(from, to world.Position, ignoreBlockerID string) bool {
	return f.los && ignoreBlockerID == f.blocker.ID
}

func testService() (*Service, *fakeWorld) {
	definition := gameplayworld.Gate{
		ID: "main-gate", BlockerID: "main-gate", MaxHP: 200,
		Attack: gameplayworld.GateAttackProfile{Range: 4.5, Damage: 100, CooldownSeconds: 0.5},
	}
	scene := &fakeWorld{
		blocker: gameplayworld.Blocker{
			ID: "main-gate", Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: -3, MaxX: 3, MinZ: 9.5, MaxZ: 10.5},
			MinY: 0, MaxY: 6, BlocksMovement: true, BlocksLOS: true, Enabled: true,
		},
		enabled: true,
		los: true,
	}
	return NewService([]gameplayworld.Gate{definition}), scene
}

func TestAttackDamagesAndDestroysGateAtomically(t *testing.T) {
	svc, scene := testService()
	position := world.Position{X: 0, Y: 0, Z: 6, Layer: 0}

	first, err := svc.Attack(1, position, "main-gate", 10, 50*time.Millisecond, scene)
	if err != nil { t.Fatal(err) }
	if first.HP != 100 || first.Destroyed || !scene.enabled {
		t.Fatalf("first attack state=%+v blocker=%v", first, scene.enabled)
	}

	if _, err := svc.Attack(1, position, "main-gate", 15, 50*time.Millisecond, scene); !errors.Is(err, ErrGateAttackCooldown) {
		t.Fatalf("cooldown error=%v", err)
	}

	second, err := svc.Attack(1, position, "main-gate", 20, 50*time.Millisecond, scene)
	if err != nil { t.Fatal(err) }
	if second.HP != 0 || !second.Destroyed || scene.enabled {
		t.Fatalf("destroy state=%+v blocker=%v", second, scene.enabled)
	}
}

func TestAttackValidatesLayerRangeAndLOS(t *testing.T) {
	t.Run("layer", func(t *testing.T) {
		svc, scene := testService()
		_, err := svc.Attack(1, world.Position{X: 0, Z: 6, Layer: 2}, "main-gate", 1, 50*time.Millisecond, scene)
		if !errors.Is(err, ErrGateWrongLayer) { t.Fatalf("error=%v", err) }
	})
	t.Run("range", func(t *testing.T) {
		svc, scene := testService()
		_, err := svc.Attack(1, world.Position{X: 0, Z: 0, Layer: 0}, "main-gate", 1, 50*time.Millisecond, scene)
		if !errors.Is(err, ErrGateOutOfRange) { t.Fatalf("error=%v", err) }
	})
	t.Run("los", func(t *testing.T) {
		svc, scene := testService()
		scene.los = false
		_, err := svc.Attack(1, world.Position{X: 0, Z: 6, Layer: 0}, "main-gate", 1, 50*time.Millisecond, scene)
		if !errors.Is(err, ErrGateNoLineOfSight) { t.Fatalf("error=%v", err) }
	})
}
