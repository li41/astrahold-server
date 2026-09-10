// Package classaction defines legacy action resource outcomes. Action authorization is owned by the legacy class-gate compatibility boundary.
package classaction

import (
	"github.com/li41/astrahold-server/internal/classresource"
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
	CostResource               classresource.ID
	CostAmount                 uint32
	AcceptedResource           classresource.ID
	AcceptedGain               uint32
	AcceptedReductionResource  classresource.ID
	AcceptedReductionAmount    uint32
	HitResource                classresource.ID
	HitGain                    uint32
	HitProgressResource        classresource.ID
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
		return Policy{HitResource: classresource.Resolve, HitGain: 8}, true
	case OathguardFortify:
		return Policy{CostResource: classresource.Resolve, CostAmount: 30}, true
	case BreakerHeavySlash:
		return Policy{HitResource: classresource.Momentum, HitGain: 10}, true
	case BreakerStaggerStrike:
		return Policy{CostResource: classresource.Momentum, CostAmount: 20}, true
	case RangerHuntingShot:
		return Policy{HitResource: classresource.HuntMomentum, HitGain: 8}, true
	case RangerArmorPiercingArrow:
		return Policy{CostResource: classresource.HuntMomentum, CostAmount: 30}, true
	case StarfireFireBolt:
		return Policy{AcceptedResource: classresource.StarHeat, AcceptedGain: 8}, true
	case StarfireColdStarChannel:
		return Policy{AcceptedReductionResource: classresource.StarHeat, AcceptedReductionAmount: 45}, true
	case OathhealerOathlightStrike:
		return Policy{HitProgressResource: classresource.OathSeal, HitProgressGain: 20}, true
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
