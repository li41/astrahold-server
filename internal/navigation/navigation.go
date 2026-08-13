// Package navigation 定義伺服器權威導航／碰撞介面。
//
// 第一階段只有簡單平面實作；未來 World Compiler 會輸出真正的導航資料，
// movement package 不需要因此改寫。
package navigation

import (
	"errors"

	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrBlocked          = errors.New("navigation: movement blocked")
	ErrUnsupportedLayer = errors.New("navigation: unsupported layer")
)

// Agent 描述導航解算需要的角色幾何資訊。
type Agent struct {
	Radius        float32
	MaxStepHeight float32
}

// Navigator 是世界移動與 LOS 的抽象層。
type Navigator interface {
	ResolveMove(from world.Position, displacement world.Vec3, agent Agent) (world.Position, error)
	HasLineOfSight(from, to world.Position) bool
}
