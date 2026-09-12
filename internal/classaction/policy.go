// Package classaction defines shipped legacy action IDs and their resource/effect policy fixtures.
// It does not authorize fixed professions; current classless legality is decided by the authoritative
// action, learned-skill and equipment paths.
package classaction

import (
	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/targetresource"
)

const (
	OathguardSwordStrike       = "oathguard-sword-strike"
	OathguardFortify           = "oathguard-fortify"
	BreakerHeavySlash          = "breaker-heavy-slash"
	BreakerStaggerStrike       = "breaker-stagger-strike"
	RangerHuntingShot          = "ranger-hunting-shot"
	RangerArmorPiercingArrow   = "ranger-armor-piercing-arrow"
	StarfireFireBolt           = "starfire-fire-bolt"
	StarfireColdStarChannel    = "starfire-cold-star-channel"
	OathhealerOathlightStrike  = "oathhealer-oathlight-strike"
	ShadowbladeDualBladeStrike = "shadowblade-dual-blade-strike"
	ShadowbladeRiftStab        = "shadowblade-rift-stab"
	ShadowbladeFlawExecute     = "shadowblade-flaw-execute"
)

type TargetResourceSpendTier struct {
	Amount uint32
	Damage uint32
}

type TargetResourceSpendPolicy struct {
	ResourceID targetresource.ID
	Tiers      []TargetResourceSpendTier
}

func (p TargetResourceSpendPolicy) Resolve(current uint32) (amount, damage uint32, ok bool) {
	if p.ResourceID == "" || current == 0 {
		return 0, 0, false
	}
	for _, tier := range p.Tiers {
		if tier.Amount == 0 || tier.Damage == 0 || tier.Amount > current || tier.Amount <= amount {
			continue
		}
		amount = tier.Amount
		damage = tier.Damage
	}
	return amount, damage, amount > 0
}

type Policy struct {
	CostResource               actionresource.ID
	CostAmount                 uint32
	AcceptedResource           actionresource.ID
	AcceptedGain               uint32
	AcceptedReductionResource  actionresource.ID
	AcceptedReductionAmount    uint32
	HitResource                actionresource.ID
	HitGain                    uint32
	HitProgressResource        actionresource.ID
	HitProgressGain            uint32
	HitTargetResource          targetresource.ID
	HitTargetGain              uint32
	HitTargetMax               uint32
	HitTargetICDSeconds        float64
	HitTargetPreserveReadyTick bool
	RequireSideOrBack          bool
	TargetSpend                TargetResourceSpendPolicy
}

func ForAction(actionID string) (Policy, bool) {
	switch actionID {
	case OathguardSwordStrike:
		return Policy{HitResource: actionresource.Resolve, HitGain: 8}, true
	case OathguardFortify:
		return Policy{CostResource: actionresource.Resolve, CostAmount: 30}, true
	case BreakerHeavySlash:
		return Policy{HitResource: actionresource.Momentum, HitGain: 10}, true
	case BreakerStaggerStrike:
		return Policy{CostResource: actionresource.Momentum, CostAmount: 20}, true
	case RangerHuntingShot:
		return Policy{HitResource: actionresource.HuntMomentum, HitGain: 8}, true
	case RangerArmorPiercingArrow:
		return Policy{CostResource: actionresource.HuntMomentum, CostAmount: 30}, true
	case StarfireFireBolt:
		return Policy{AcceptedResource: actionresource.StarHeat, AcceptedGain: 8}, true
	case StarfireColdStarChannel:
		return Policy{AcceptedReductionResource: actionresource.StarHeat, AcceptedReductionAmount: 45}, true
	case OathhealerOathlightStrike:
		return Policy{HitProgressResource: actionresource.OathSeal, HitProgressGain: 20}, true
	case ShadowbladeDualBladeStrike:
		return Policy{HitTargetResource: targetresource.Flaw, HitTargetGain: 1, HitTargetMax: 3, HitTargetICDSeconds: 2.5, RequireSideOrBack: true}, true
	case ShadowbladeRiftStab:
		return Policy{HitTargetResource: targetresource.Flaw, HitTargetGain: 1, HitTargetMax: 3, HitTargetPreserveReadyTick: true, RequireSideOrBack: true}, true
	case ShadowbladeFlawExecute:
		return Policy{
			TargetSpend: TargetResourceSpendPolicy{
				ResourceID: targetresource.Flaw,
				Tiers: []TargetResourceSpendTier{
					{Amount: 1, Damage: 140},
					{Amount: 2, Damage: 220},
					{Amount: 3, Damage: 300},
				},
			},
		}, true
	default:
		return Policy{}, false
	}
}
