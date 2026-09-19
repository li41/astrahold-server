package navigation

import (
	"errors"
	"math"
	"sync"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/terrainheight"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrUnknownBlocker          = errors.New("navigation: unknown blocker")
	ErrInvalidTerrainHeightfield = errors.New("navigation: invalid terrain heightfield")
)

// GameplayNavigator 是版本化 Gameplay Proxy 驅動的權威導航實作。
type GameplayNavigator struct {
	surfaces map[world.LayerID][]gameplayworld.Surface
	portals  []gameplayworld.Portal
	blockers     map[string]gameplayworld.Blocker
	heightfields map[string]*terrainheight.Field

	mu      sync.RWMutex
	enabled map[string]bool
}

func NewGameplayNavigator(definition gameplayworld.Definition) (*GameplayNavigator, error) {
	return NewGameplayNavigatorWithHeightfields(definition, nil)
}

// NewGameplayNavigatorWithHeightfields overlays immutable baked terrain onto named gameplay
// surfaces. A field may cover only a subset of its surface; outside that rectangle the authored
// plane remains the fallback. Loading and hash verification happen before this constructor, so
// ResolveMove remains allocation-free and performs no I/O on the world tick.
func NewGameplayNavigatorWithHeightfields(definition gameplayworld.Definition, heightfields map[string]*terrainheight.Field) (*GameplayNavigator, error) {
	if err := gameplayworld.Validate(definition); err != nil {
		return nil, err
	}
	n := &GameplayNavigator{
		surfaces:     make(map[world.LayerID][]gameplayworld.Surface),
		portals:      append([]gameplayworld.Portal(nil), definition.Portals...),
		blockers:     make(map[string]gameplayworld.Blocker, len(definition.Blockers)),
		heightfields: make(map[string]*terrainheight.Field, len(heightfields)),
		enabled:      make(map[string]bool, len(definition.Blockers)),
	}
	surfaceByID := make(map[string]gameplayworld.Surface, len(definition.Surfaces))
	for _, surface := range definition.Surfaces {
		n.surfaces[surface.Layer] = append(n.surfaces[surface.Layer], surface)
		surfaceByID[surface.ID] = surface
	}
	for surfaceID, field := range heightfields {
		surface, ok := surfaceByID[surfaceID]
		if !ok || field == nil || !terrainFieldWithinSurface(field.Bounds(), surface.Bounds) {
			return nil, ErrInvalidTerrainHeightfield
		}
		n.heightfields[surfaceID] = field
	}
	for _, blocker := range definition.Blockers {
		n.blockers[blocker.ID] = blocker
		n.enabled[blocker.ID] = blocker.Enabled
	}
	return n, nil
}

func (n *GameplayNavigator) ResolveMove(from world.Position, displacement world.Vec3, agent Agent) (world.Position, error) {
	sourceAtFrom, ok := n.surfaceAt(from.Layer, from.X, from.Z)
	if !ok {
		if _, exists := n.surfaces[from.Layer]; !exists {
			return from, ErrUnsupportedLayer
		}
		return from, ErrBlocked
	}
	sourceGroundY := n.surfaceHeightAt(sourceAtFrom, from.X, from.Z)

	toX := from.X + displacement.X
	toZ := from.Z + displacement.Z
	if n.movementBlocked(from.Layer, from.X, from.Z, toX, toZ, agent.Radius) {
		return from, ErrBlocked
	}

	if targetLayer, ok := n.portalTarget(from.Layer, from.X, from.Z, toX, toZ); ok {
		sourceSurface, sourceFound := n.surfaceAt(from.Layer, toX, toZ)
		if !sourceFound {
			return from, ErrBlocked
		}
		targetSurface, targetFound := n.surfaceAt(targetLayer, toX, toZ)
		if !targetFound {
			return from, ErrBlocked
		}
		if n.movementBlocked(targetLayer, from.X, from.Z, toX, toZ, agent.Radius) {
			return from, ErrBlocked
		}
		sourceY := n.surfaceHeightAt(sourceSurface, toX, toZ)
		targetY := n.surfaceHeightAt(targetSurface, toX, toZ)
		if !stepAllowed(sourceY, targetY, agent.MaxStepHeight) {
			return from, ErrBlocked
		}
		return world.Position{X: toX, Y: targetY, Z: toZ, Layer: targetLayer}, nil
	}

	targetSurface, ok := n.surfaceAt(from.Layer, toX, toZ)
	if !ok {
		return from, ErrBlocked
	}
	next := world.Position{X: toX, Y: n.surfaceHeightAt(targetSurface, toX, toZ), Z: toZ, Layer: from.Layer}
	if !stepAllowed(sourceGroundY, next.Y, agent.MaxStepHeight) {
		return from, ErrBlocked
	}
	return next, nil
}

// GroundHeightAt resolves the authoritative movement surface at one world-space X/Z. Baked
// heightfields take precedence only inside their own bounds; the gameplay plane remains the fallback.
func (n *GameplayNavigator) GroundHeightAt(layer world.LayerID, x, z float32) (float32, bool) {
	surface, ok := n.surfaceAt(layer, x, z)
	if !ok {
		return 0, false
	}
	return n.surfaceHeightAt(surface, x, z), true
}

// ResolveGroundPosition projects one trusted Server-authored anchor onto the same height source used
// by movement. It is intended for startup spawn/home authoring and never accepts a Client position.
func (n *GameplayNavigator) ResolveGroundPosition(position world.Position) (world.Position, error) {
	height, ok := n.GroundHeightAt(position.Layer, position.X, position.Z)
	if !ok {
		if _, exists := n.surfaces[position.Layer]; !exists {
			return position, ErrUnsupportedLayer
		}
		return position, ErrBlocked
	}
	position.Y = height
	return position, nil
}

func (n *GameplayNavigator) surfaceHeightAt(surface gameplayworld.Surface, x, z float32) float32 {
	if field := n.heightfields[surface.ID]; field != nil {
		if height, ok := field.HeightAt(x, z); ok {
			return height
		}
	}
	return surface.Plane.HeightAt(x, z)
}

func (n *GameplayNavigator) HasLineOfSight(from, to world.Position) bool {
	return n.hasLineOfSight(from, to, "")
}

// HasLineOfSightIgnoringBlocker 用於對 blocker 本體的互動，例如攻擊城門。
// 目標 blocker 不應因為射線終點落在自身 AABB 而遮蔽自己；其他 blocker 仍照常阻擋。
func (n *GameplayNavigator) HasLineOfSightIgnoringBlocker(from, to world.Position, ignoreBlockerID string) bool {
	return n.hasLineOfSight(from, to, ignoreBlockerID)
}

func (n *GameplayNavigator) hasLineOfSight(from, to world.Position, ignoreBlockerID string) bool {
	if _, ok := n.surfaceAt(from.Layer, from.X, from.Z); !ok {
		return false
	}
	if _, ok := n.surfaceAt(to.Layer, to.X, to.Z); !ok {
		return false
	}

	n.mu.RLock()
	defer n.mu.RUnlock()
	for id, blocker := range n.blockers {
		if id == ignoreBlockerID || !n.enabled[id] || !blocker.BlocksLOS {
			continue
		}
		if segmentIntersectsAABB(from, to, blocker) {
			return false
		}
	}
	return true
}

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

func (n *GameplayNavigator) BlockerDefinition(id string) (gameplayworld.Blocker, error) {
	blocker, ok := n.blockers[id]
	if !ok {
		return gameplayworld.Blocker{}, ErrUnknownBlocker
	}
	return blocker, nil
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

func terrainFieldWithinSurface(field terrainheight.BoundsXZ, surface gameplayworld.BoundsXZ) bool {
	const epsilon = float32(0.01)
	return field.MinX >= surface.MinX-epsilon && field.MaxX <= surface.MaxX+epsilon &&
		field.MinZ >= surface.MinZ-epsilon && field.MaxZ <= surface.MaxZ+epsilon
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
