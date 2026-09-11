// Package loadlab 提供 Astrahold 的 headless 壓測工具與量測資料結構。
package loadlab

import (
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type Scenario string

const (
	ScenarioDistributed   Scenario = "distributed"
	ScenarioCrowd         Scenario = "crowd"
	ScenarioTeleportChurn Scenario = "teleport-churn"

	s3e9MixedMovementLeg = 250 * time.Millisecond
)

var (
	ErrUnknownScenario       = errors.New("loadlab: unknown scenario")
	s3e9MixedMovementEnabled = os.Getenv("ASTRAHOLD_S3E9_MIXED_MOVEMENT") == "1"
)

func ParseScenario(value string) (Scenario, error) {
	scenario := Scenario(value)
	switch scenario {
	case ScenarioDistributed, ScenarioCrowd, ScenarioTeleportChurn:
		return scenario, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnknownScenario, value)
	}
}

type scenarioLayout struct {
	ground gameplayworld.Surface
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
	if scenario == ScenarioTeleportChurn {
		if err := validateTeleportChurnLayout(layout, totalClients); err != nil {
			return nil, err
		}
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
	return scenarioLayout{ground: ground}, nil
}

func spawnPosition(layout scenarioLayout, scenario Scenario, index, total int) world.Position {
	switch scenario {
	case ScenarioCrowd:
		return pointOnSurface(layout.ground, gridPoint(crowdBounds(layout), index, total, 0.25))
	case ScenarioTeleportChurn:
		west, east := teleportChurnBounds(layout)
		groupSize := total / 2
		localIndex := index % groupSize
		bounds := west
		if index >= groupSize {
			bounds = east
		}
		return pointOnSurface(layout.ground, gridPoint(bounds, localIndex, groupSize, 0.25))
	default:
		return pointOnSurface(layout.ground, gridPoint(layout.ground.Bounds, index, total, 2))
	}
}

// crowdBounds keeps a dense benchmark cohort inside one AOI without depending on
// any gameplay blocker, gate, building, or presentation layout. It deliberately
// uses the north-central portion of the authored ground so the benchmark remains
// stable when starter-village props change.
func crowdBounds(layout scenarioLayout) gameplayworld.BoundsXZ {
	ground := layout.ground.Bounds
	centerX := (ground.MinX + ground.MaxX) * 0.5
	maxZ := ground.MaxZ - 2
	minZ := maxZ - 18
	if minZ < ground.MinZ+2 {
		minZ = ground.MinZ + 2
	}
	return gameplayworld.BoundsXZ{
		MinX: max32(ground.MinX+2, centerX-12),
		MaxX: min32(ground.MaxX-2, centerX+12),
		MinZ: minZ,
		MaxZ: maxZ,
	}
}

// TeleportChurnTargets 回傳 S3-E.7 的 deterministic authoritative teleport plan。
// 500-client case 會把兩個 250 人群組各前 125 人交換到另一群相同 grid slot，
// 因此所有 Session 都會同時失去半個舊 AOI、取得半個新 AOI。
func TeleportChurnTargets(def gameplayworld.Definition, totalClients int) (map[world.EntityID]world.Position, error) {
	layout, err := buildLayout(def, ScenarioTeleportChurn)
	if err != nil {
		return nil, err
	}
	if err := validateTeleportChurnLayout(layout, totalClients); err != nil {
		return nil, err
	}
	west, east := teleportChurnBounds(layout)
	groupSize := totalClients / 2
	moversPerGroup := groupSize / 2
	targets := make(map[world.EntityID]world.Position, moversPerGroup*2)
	for localIndex := 0; localIndex < moversPerGroup; localIndex++ {
		westID := world.EntityID(localIndex + 1)
		eastID := world.EntityID(groupSize + localIndex + 1)
		westTarget := pointOnSurface(layout.ground, gridPoint(west, localIndex, groupSize, 0.25))
		eastTarget := pointOnSurface(layout.ground, gridPoint(east, localIndex, groupSize, 0.25))
		targets[westID] = eastTarget
		targets[eastID] = westTarget
	}
	return targets, nil
}

func validateTeleportChurnLayout(layout scenarioLayout, totalClients int) error {
	if totalClients < 4 || totalClients%4 != 0 {
		return errors.New("loadlab: teleport-churn clients must be >= 4 and divisible by 4")
	}
	west, east := teleportChurnBounds(layout)
	if west.MinX >= west.MaxX || west.MinZ >= west.MaxZ || east.MinX >= east.MaxX || east.MinZ >= east.MaxZ {
		return errors.New("loadlab: ground surface is too small for teleport-churn clusters")
	}
	dx := east.MinX - west.MaxX
	dz := east.MinZ - west.MaxZ
	if math.Sqrt(float64(dx*dx+dz*dz)) <= 64 {
		return errors.New("loadlab: teleport-churn clusters must be more than AOI radius apart")
	}
	return nil
}

func teleportChurnBounds(layout scenarioLayout) (gameplayworld.BoundsXZ, gameplayworld.BoundsXZ) {
	ground := layout.ground.Bounds
	// Keep both deterministic clusters well inside authored ground. S3-E.9 drives
	// bounded ClientMoveInput around these boxes; the inset prevents the load
	// fixture itself from manufacturing boundary corrections or village collisions.
	// The nearest cluster corners remain more than the 64m AOI radius apart.
	return gameplayworld.BoundsXZ{
		MinX: ground.MinX + 11,
		MaxX: ground.MinX + 23,
		MinZ: ground.MinZ + 10,
		MaxZ: ground.MinZ + 22,
	}, gameplayworld.BoundsXZ{
		MinX: ground.MaxX - 24,
		MaxX: ground.MaxX - 12,
		MinZ: ground.MaxZ - 25,
		MaxZ: ground.MaxZ - 13,
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

// S3E9MixedStationaryEntity 將相鄰兩個 Entity 視為一組，兩組 stationary / movers 交錯。
// Teleport-churn 的 adjacent combat pairs 因此可挑固定 stationary hot targets；其餘約一半
// Client 持續走正常 ClientMoveInput，並讓低/高 SessionID 都同時包含 movers 與 observers。
func S3E9MixedStationaryEntity(entityID world.EntityID) bool {
	if entityID == 0 {
		return true
	}
	return ((uint64(entityID)-1)/2)%2 == 0
}

func distributedMovementDirection(entityID world.EntityID, phase int) (float32, float32) {
	directions := [8][2]float32{
		{1, 0}, {0.70710677, 0.70710677}, {0, 1}, {-0.70710677, 0.70710677},
		{-1, 0}, {-0.70710677, -0.70710677}, {0, -1}, {0.70710677, -0.70710677},
	}
	direction := directions[(int(entityID)+phase)%len(directions)]
	return direction[0], direction[1]
}

func s3e9MixedMovementDirection(entityID world.EntityID, elapsed time.Duration) (float32, float32) {
	phase := int(elapsed / s3e9MixedMovementLeg)
	return distributedMovementDirection(entityID, phase)
}

// MovementDirection 回傳 deterministic input pattern，避免 Load Lab 本身使用大量 RNG。
func MovementDirection(scenario Scenario, entityID world.EntityID, elapsed time.Duration) (float32, float32) {
	phase := int(elapsed / (2 * time.Second))
	switch scenario {
	case ScenarioCrowd:
		return 0, -1
	case ScenarioTeleportChurn:
		if !s3e9MixedMovementEnabled || S3E9MixedStationaryEntity(entityID) {
			return 0, 0
		}
		// Mixed soak needs real ClientMoveInput and AOI churn, not repeated world
		// boundary corrections. Eight short compass legs form a bounded 2s cycle.
		return s3e9MixedMovementDirection(entityID, elapsed)
	default:
		return distributedMovementDirection(entityID, phase)
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
