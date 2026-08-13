// Package loadlab 提供 Astrahold Siege Load Lab 的 headless 壓測工具與量測資料結構。
package loadlab

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type Scenario string

const (
	ScenarioDistributed   Scenario = "distributed"
	ScenarioGateZerg      Scenario = "gate-zerg"
	ScenarioVerticalSiege Scenario = "vertical-siege"
)

var ErrUnknownScenario = errors.New("loadlab: unknown scenario")

func ParseScenario(value string) (Scenario, error) {
	scenario := Scenario(value)
	switch scenario {
	case ScenarioDistributed, ScenarioGateZerg, ScenarioVerticalSiege:
		return scenario, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownScenario, value)
	}
}

type scenarioLayout struct {
	ground gameplayworld.Surface
	west   gameplayworld.Surface
	east   gameplayworld.Surface
	wall   gameplayworld.Surface
	gate   gameplayworld.Blocker
}

// NewPlayerFactory 建立只供 Load Lab 使用的 deterministic spawn factory。
// 它仍透過正常 TCP accept -> Session -> EnqueueJoin 進入世界，不繞過 Runtime。
func NewPlayerFactory(def gameplayworld.Definition, scenario Scenario, totalClients int) (tcpudp.PlayerFactory, error) {
	if totalClients <= 0 {
		return nil, errors.New("loadlab: totalClients must be > 0")
	}
	layout, err := buildLayout(def, scenario)
	if err != nil {
		return nil, err
	}

	return func(_ session.ID, entityID world.EntityID) tcpudp.PlayerSpec {
		index := int(uint64(entityID) - 1)
		position := spawnPosition(layout, scenario, index, totalClients)
		return tcpudp.PlayerSpec{
			Entity: world.EntityState{
				ID:        entityID,
				Kind:      world.EntityPlayer,
				Transform: world.Transform{Position: position},
			},
			Speed:         6,
			Radius:        def.Agent.Radius,
			MaxStepHeight: def.Agent.MaxStepHeight,
			AOIRadius:     64,
		}
	}, nil
}

func buildLayout(def gameplayworld.Definition, scenario Scenario) (scenarioLayout, error) {
	if _, err := ParseScenario(string(scenario)); err != nil {
		return scenarioLayout{}, err
	}
	ground, ok := surfaceByID(def, "ground")
	if !ok {
		return scenarioLayout{}, errors.New("loadlab: ground surface is required")
	}
	gate, ok := blockerByID(def, "main-gate")
	if !ok {
		return scenarioLayout{}, errors.New("loadlab: main-gate blocker is required")
	}
	layout := scenarioLayout{ground: ground, gate: gate}
	if scenario != ScenarioVerticalSiege {
		return layout, nil
	}
	if layout.west, ok = surfaceByID(def, "west-stair"); !ok {
		return scenarioLayout{}, errors.New("loadlab: west-stair surface is required for vertical-siege")
	}
	if layout.east, ok = surfaceByID(def, "east-stair"); !ok {
		return scenarioLayout{}, errors.New("loadlab: east-stair surface is required for vertical-siege")
	}
	if layout.wall, ok = surfaceByID(def, "front-wall-walk"); !ok {
		return scenarioLayout{}, errors.New("loadlab: front-wall-walk surface is required for vertical-siege")
	}
	return layout, nil
}

func spawnPosition(layout scenarioLayout, scenario Scenario, index, total int) world.Position {
	switch scenario {
	case ScenarioGateZerg:
		return pointOnSurface(layout.ground, gridPoint(gateApproachBounds(layout), index, total, 0.25))
	case ScenarioVerticalSiege:
		role := index % 10
		switch {
		case role < 5:
			return pointOnSurface(layout.ground, gridPoint(gateApproachBounds(layout), index/2, maxInt(1, total/2), 0.25))
		case role == 5:
			return pointOnSurface(layout.west, gridPoint(layout.west.Bounds, index/10, maxInt(1, total/10), 0.15))
		case role == 6:
			return pointOnSurface(layout.east, gridPoint(layout.east.Bounds, index/10, maxInt(1, total/10), 0.15))
		default:
			return pointOnSurface(layout.wall, gridPoint(layout.wall.Bounds, index/3, maxInt(1, total*3/10), 0.15))
		}
	default:
		return pointOnSurface(layout.ground, gridPoint(layout.ground.Bounds, index, total, 2))
	}
}

func gateApproachBounds(layout scenarioLayout) gameplayworld.BoundsXZ {
	centerX := (layout.gate.Bounds.MinX + layout.gate.Bounds.MaxX) * 0.5
	minZ := layout.gate.Bounds.MinZ - 18
	if minZ < layout.ground.Bounds.MinZ+1 {
		minZ = layout.ground.Bounds.MinZ + 1
	}
	maxZ := layout.gate.Bounds.MinZ - 1
	return gameplayworld.BoundsXZ{
		MinX: max32(layout.ground.Bounds.MinX+1, centerX-12),
		MaxX: min32(layout.ground.Bounds.MaxX-1, centerX+12),
		MinZ: minZ,
		MaxZ: maxZ,
	}
}

type xzPoint struct{ x, z float32 }

func gridPoint(bounds gameplayworld.BoundsXZ, index, total int, margin float32) xzPoint {
	if total < 1 {
		total = 1
	}
	cols := int(math.Ceil(math.Sqrt(float64(total))))
	if cols < 1 {
		cols = 1
	}
	rows := (total + cols - 1) / cols
	minX, maxX := bounds.MinX+margin, bounds.MaxX-margin
	minZ, maxZ := bounds.MinZ+margin, bounds.MaxZ-margin
	if minX > maxX {
		minX, maxX = bounds.MinX, bounds.MaxX
	}
	if minZ > maxZ {
		minZ, maxZ = bounds.MinZ, bounds.MaxZ
	}
	col := index % cols
	row := (index / cols) % maxInt(1, rows)
	fx := (float32(col) + 0.5) / float32(cols)
	fz := (float32(row) + 0.5) / float32(maxInt(1, rows))
	return xzPoint{x: minX + (maxX-minX)*fx, z: minZ + (maxZ-minZ)*fz}
}

func pointOnSurface(surface gameplayworld.Surface, point xzPoint) world.Position {
	return world.Position{
		X:     point.x,
		Y:     surface.Plane.HeightAt(point.x, point.z),
		Z:     point.z,
		Layer: surface.Layer,
	}
}

// MovementDirection 回傳 deterministic input pattern，避免 Load Lab 本身使用大量 RNG。
func MovementDirection(scenario Scenario, entityID world.EntityID, elapsed time.Duration) (float32, float32) {
	phase := int(elapsed / (2 * time.Second))
	switch scenario {
	case ScenarioGateZerg:
		return 0, 1
	case ScenarioVerticalSiege:
		role := int((uint64(entityID) - 1) % 10)
		switch {
		case role < 5:
			return 0, 1
		case role == 5 || role == 6:
			if phase%2 == 0 {
				return 0, -1
			}
			return 0, 1
		default:
			if (phase+role)%2 == 0 {
				return 1, 0
			}
			return -1, 0
		}
	default:
		directions := [8][2]float32{
			{1, 0}, {0.70710677, 0.70710677}, {0, 1}, {-0.70710677, 0.70710677},
			{-1, 0}, {-0.70710677, -0.70710677}, {0, -1}, {0.70710677, -0.70710677},
		}
		direction := directions[(int(entityID)+phase)%len(directions)]
		return direction[0], direction[1]
	}
}

func surfaceByID(def gameplayworld.Definition, id string) (gameplayworld.Surface, bool) {
	for _, surface := range def.Surfaces {
		if surface.ID == id {
			return surface, true
		}
	}
	return gameplayworld.Surface{}, false
}

func blockerByID(def gameplayworld.Definition, id string) (gameplayworld.Blocker, bool) {
	for _, blocker := range def.Blockers {
		if blocker.ID == id {
			return blocker, true
		}
	}
	return gameplayworld.Blocker{}, false
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
