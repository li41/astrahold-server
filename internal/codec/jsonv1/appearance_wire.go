package jsonv1

import (
	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/protocol"
)

type appearanceSnapshot struct {
	SkinID                   string `json:"skin_id,omitempty"`
	BasicAttackAffinityBonus uint32 `json:"basic_attack_affinity_bonus"`
}

func toAppearanceSnapshot(in protocol.AppearanceSnapshot) appearanceSnapshot {
	return appearanceSnapshot{
		SkinID:                   string(in.SkinID),
		BasicAttackAffinityBonus: in.BasicAttackAffinityBonus,
	}
}

func fromAppearanceSnapshot(in appearanceSnapshot) protocol.AppearanceSnapshot {
	return protocol.AppearanceSnapshot{
		SkinID:                   appearance.SkinID(in.SkinID),
		BasicAttackAffinityBonus: in.BasicAttackAffinityBonus,
	}
}
