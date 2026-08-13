package navigation

import (
	"errors"
	"math"
	"sync"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrUnknownBlocker = errors.New("navigation: unknown blocker")

// GameplayNavigator 是 S3 起由版本化 Gameplay Proxy 驅動的權威導航實作。
// 它刻意先使用簡單 Surface/Portal/Blocker 幾何，讓 World Compiler schema、Layer transition、
// dynamic gate 與 LOS contract 先穩定；日後可以在不改 movement API 的前提下替換為 Recast/Detour。
type GameplayNavigator struct {
	surfaces map[world.LayerID][]gameplayworld.Surface
	portals  []gameplayworld.Portal
	blockers map[string]gameplayworld.Blocker

	mu      sync.RWMutex
	enabled map[string]bool
}

func NewGameplayNavigator(definition gameplayworld.Definition) (*GameplayNavigator, error) {
	if err := gameplayworld.Validate(definition); err != nil {
		return nil, err
	}
	n := &GameplayNavigator{
		surfaces: make(map[world.LayerID][]gameplayworld.Surface),
		portals:  append([]gameplayworld.Portal(nil), definition.Portals...),
		blockers: make(map[string]gameplayworld.Blocker, len(definition.Blockers)),
		enabled:  make(map[string]bool, len(definition.Blockers)),
	}
	for _, surface := range definition.Surfaces {
		n.surfaces[surface.Layer] = append(n.surfaces[surface.Layer], surface)
	}
	for _, blocker := range definition.Blockers {
		n.blockers[blocker.ID] = blocker
		n.enabled[blocker.ID] = blocker.Enabled
	}
	return n, nil
}

func (n *GameplayNavigator) ResolveMove(from world.Position, displacement world.Vec3, agent Agent) (world.Position, error) {
	if _, ok := n.surfaceAt(from.Layer, from.X, from.Z); !ok {
		if _, exists := n.surfaces[from.Layer]; !exists {
			return from, ErrUnsupportedLayer
		}
		return from, ErrBlocked
	}

	toX := from.X + displacement.X
	toZ := from.Z + displacement.Z
	if n.movementBlocked(from.Layer, from.X, from.Z, toX, toZ, agent.Radius) {
		return from, ErrBlocked
	}

	// Portal 優先於同 layer surface。只有從 portal 外部進入／穿越 trigger 才轉層，
	// 避免 bidirectional portal 在 trigger 內每 tick 來回切 layer。
	if targetLayer, ok := n.portalTarget(from.Layer, from.X, from.Z, toX, toZ); ok {
		targetSurface, found := n.surfaceAt(targetLayer, toX, toZ)
		if !found {
			return from, ErrBlocked
		}
		if n.movementBlocked(targetLayer, from.X, from.Z, toX, toZ, agent.Radius) {
			return from, ErrBlocked
		}
		next := world.Position{X: toX, Y: targetSurface.Plane.HeightAt(toX, toZ), Z: toZ, Layer: targetLayer}
		if !stepAllowed(from.Y, next.Y, agent.MaxStepHeight) {
			return from, ErrBlocked
		}
		return next, nil
	}

	targetSurface, ok := n.surfaceAt(from.Layer, toX, toZ)
	if !ok {
		return from, ErrBlocked
	}
	next := world.Position{X: toX, Y: targetSurface.Plane.HeightAt(toX, toZ), Z: toZ, Layer: from.Layer}
	if !stepAllowed(from.Y, next.Y, agent.MaxStepHeight) {
		return from, ErrBlocked
	}
	return next, nil
}

func (n *GameplayNavigator) HasLineOfSight(from, to world.Position) bool {
	if _, ok := n.surfaceAt(from.Layer, from.X, from.Z); !ok {
		return false
	}
	if _, ok := n.surfaceAt(to.Layer, to.X, to.Z); !ok {
		return false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	for id, blocker := range n.blockers {
		if !n.enabled[id] || !blocker.BlocksLOS {
			continue
		}
		if segmentIntersectsAABB(from, to, blocker) {
			return false
		}
	}
	return true
}

// SetBlockerEnabled 是 Siege/Gate runtime 切換 Gameplay blocker 的唯一入口。
// 它不修改原始 world definition，也不需要重建整張導航資料。
func (n *GameplayNavigator) SetBlockerEnabled(id string, enabled bool) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.blockers[id]; !ok {
		return ErrUnknownBlocker
	}
	n.enabled[id] = enabled
	return nil
}

func (n *GameplayNavigator) BlockerEnabled(id string) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()
	if _, ok := n.blockers[id]; !ok {
		return false, ErrUnknownBlocker
	}
	return n.enabled[id], nil
}

func (n *GameplayNavigator) surfaceAt(layer world.LayerID, x, z float32) (gameplayworld.Surface, bool) {
	for _, surface := range n.surfaces[layer] {
		if surface.Bounds.Contains(x, z) {
			return surface, true
		}
	}
	return gameplayworld.Surface{}, false
}

func (n *GameplayNavigator) portalTarget(layer world.LayerID, fromX, fromZ, toX, toZ float32) (world.LayerID, bool) {
	for _, portal := range n.portals {
		var target world.LayerID
		matches := false
		switch {
		case portal.FromLayer == layer:
			target = portal.ToLayer
			matches = true
		case portal.Bidirectional && portal.ToLayer == layer:
			target = portal.FromLayer
			matches = true
		}
		if !matches || portal.Bounds.Contains(fromX, fromZ) {
			continue
		}
		if segmentIntersectsRect(fromX, fromZ, toX, toZ, portal.Bounds) {
			return target, true
		}
	}
	return 0, false
}

func (n *GameplayNavigator) movementBlocked(layer world.LayerID, fromX, fromZ, toX, toZ, radius float32) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for id, blocker := range n.blockers {
		if !n.enabled[id] || !blocker.BlocksMovement || blocker.Layer != layer {
			continue
		}
		if segmentIntersectsRect(fromX, fromZ, toX, toZ, blocker.Bounds.Expanded(radius)) {
			return true
		}
	}
	return false
}

func stepAllowed(fromY, toY, maxStepHeight float32) bool {
	if maxStepHeight <= 0 {
		return fromY == toY
	}
	return float32(math.Abs(float64(toY-fromY))) <= maxStepHeight+0.0001
}

func segmentIntersectsRect(x0, z0, x1, z1 float32, b gameplayworld.BoundsXZ) bool {
	tMin, tMax := float32(0), float32(1)
	if !clipAxis(x0, x1-x0, b.MinX, b.MaxX, &tMin, &tMax) {
		return false
	}
	return clipAxis(z0, z1-z0, b.MinZ, b.MaxZ, &tMin, &tMax)
}

func segmentIntersectsAABB(from, to world.Position, b gameplayworld.Blocker) bool {
	tMin, tMax := float32(0), float32(1)
	if !clipAxis(from.X, to.X-from.X, b.Bounds.MinX, b.Bounds.MaxX, &tMin, &tMax) {
		return false
	}
	if !clipAxis(from.Y, to.Y-from.Y, b.MinY, b.MaxY, &tMin, &tMax) {
		return false
	}
	return clipAxis(from.Z, to.Z-from.Z, b.Bounds.MinZ, b.Bounds.MaxZ, &tMin, &tMax)
}

func clipAxis(origin, delta, minV, maxV float32, tMin, tMax *float32) bool {
	const epsilon = 1e-7
	if float32(math.Abs(float64(delta))) < epsilon {
		return origin >= minV && origin <= maxV
	}
	inv := 1 / delta
	t1 := (minV - origin) * inv
	t2 := (maxV - origin) * inv
	if t1 > t2 {
		t1, t2 = t2, t1
	}
	if t1 > *tMin {
		*tMin = t1
	}
	if t2 < *tMax {
		*tMax = t2
	}
	return *tMin <= *tMax
}
