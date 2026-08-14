package replication

import (
	"errors"

	"github.com/li41/astrahold-server/internal/world"
)

// Tier 描述 transform replication 的相對時效需求。
// TierCritical 目前保留給 self / explicit target / objective；self transform 由 PositionCorrection 處理。
type Tier uint8

const (
	TierCritical Tier = iota
	TierNear
	TierMid
	TierFar
)

var ErrInvalidPolicy = errors.New("replication: invalid policy")

// Policy 以 replication build 次數表達 cadence，而不是直接綁死 TickRate。
// 例如 SnapshotRate=10Hz 時，NearEveryBuilds=1 / MidEveryBuilds=2 / FarEveryBuilds=5
// 分別約為 10Hz / 5Hz / 2Hz。
type Policy struct {
	NearRadius float32
	MidRadius  float32

	NearEveryBuilds uint64
	MidEveryBuilds  uint64
	FarEveryBuilds  uint64

	NearRefreshEveryBuilds uint64
	MidRefreshEveryBuilds  uint64
	FarRefreshEveryBuilds  uint64

	MaxTransformsPerBuild int
}

func DefaultPolicy() Policy {
	return Policy{
		NearRadius: 12,
		MidRadius:  32,

		NearEveryBuilds: 1,
		MidEveryBuilds:  2,
		FarEveryBuilds:  5,

		NearRefreshEveryBuilds: 10,
		MidRefreshEveryBuilds:  20,
		FarRefreshEveryBuilds:  40,

		MaxTransformsPerBuild: 64,
	}
}

func (p Policy) IsZero() bool { return p == (Policy{}) }

func (p Policy) Validate() error {
	if p.NearRadius <= 0 || p.MidRadius <= p.NearRadius {
		return ErrInvalidPolicy
	}
	if p.NearEveryBuilds == 0 || p.MidEveryBuilds == 0 || p.FarEveryBuilds == 0 {
		return ErrInvalidPolicy
	}
	if p.NearRefreshEveryBuilds < p.NearEveryBuilds ||
		p.MidRefreshEveryBuilds < p.MidEveryBuilds ||
		p.FarRefreshEveryBuilds < p.FarEveryBuilds {
		return ErrInvalidPolicy
	}
	if p.MaxTransformsPerBuild <= 0 {
		return ErrInvalidPolicy
	}
	return nil
}

func resolvedPolicy(p Policy) Policy {
	if p.IsZero() {
		return DefaultPolicy()
	}
	if err := p.Validate(); err != nil {
		panic(err)
	}
	return p
}

func (p Policy) tier(self, target world.Position) Tier {
	dx := target.X - self.X
	dy := target.Y - self.Y
	dz := target.Z - self.Z
	distanceSquared := dx*dx + dy*dy + dz*dz
	if distanceSquared <= p.NearRadius*p.NearRadius {
		return TierNear
	}
	if distanceSquared <= p.MidRadius*p.MidRadius {
		return TierMid
	}
	return TierFar
}

func (p Policy) cadence(tier Tier) uint64 {
	switch tier {
	case TierNear:
		return p.NearEveryBuilds
	case TierMid:
		return p.MidEveryBuilds
	case TierFar:
		return p.FarEveryBuilds
	default:
		return 1
	}
}

func (p Policy) refresh(tier Tier) uint64 {
	switch tier {
	case TierNear:
		return p.NearRefreshEveryBuilds
	case TierMid:
		return p.MidRefreshEveryBuilds
	case TierFar:
		return p.FarRefreshEveryBuilds
	default:
		return 1
	}
}
