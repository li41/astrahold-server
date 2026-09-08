// Package classaction defines canonical profession requirements and class-resource outcomes for actions.
package classaction

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/targetresource"
)

const (
	OathguardSwordStrike       = "oathguard-sword-strike"
	BreakerHeavySlash          = "breaker-heavy-slash"
	RangerHuntingShot          = "ranger-hunting-shot"
	StarfireFireBolt           = "starfire-fire-bolt"
	OathhealerOathlightStrike  = "oathhealer-oathlight-strike"
	ShadowbladeDualBladeStrike = "shadowblade-dual-blade-strike"
	ShadowbladeFlawExecute     = "shadowblade-flaw-execute"
)

var ErrWrongClass = errors.New("classaction: action unavailable for class")

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
	RequiredClass       classid.ID
	AcceptedResource    classresource.ID
	AcceptedGain        uint32
	HitResource         classresource.ID
	HitGain             uint32
	HitProgressResource classresource.ID
	HitProgressGain     uint32
	HitTargetResource   targetresource.ID
	HitTargetGain       uint32
	HitTargetMax        uint32
	HitTargetICDSeconds float64
	RequireSideOrBack   bool
	TargetSpend         TargetResourceSpendPolicy
}

func ForAction(actionID string) (Policy, bool) {
	switch actionID {
	case OathguardSwordStrike:
		return Policy{RequiredClass: classid.Oathguard, HitResource: classresource.Resolve, HitGain: 8}, true
	case BreakerHeavySlash:
		return Policy{RequiredClass: classid.Breaker, HitResource: classresource.Momentum, HitGain: 10}, true
	case RangerHuntingShot:
		return Policy{RequiredClass: classid.Ranger, HitResource: classresource.HuntMomentum, HitGain: 8}, true
	case StarfireFireBolt:
		return Policy{RequiredClass: classid.StarfireMage, AcceptedResource: classresource.StarHeat, AcceptedGain: 8}, true
	case OathhealerOathlightStrike:
		return Policy{RequiredClass: classid.Oathhealer, HitProgressResource: classresource.OathSeal, HitProgressGain: 20}, true
	case ShadowbladeDualBladeStrike:
		return Policy{RequiredClass: classid.Shadowblade, HitTargetResource: targetresource.Flaw, HitTargetGain: 1, HitTargetMax: 3, HitTargetICDSeconds: 2.5, RequireSideOrBack: true}, true
	case ShadowbladeFlawExecute:
		return Policy{
			RequiredClass: classid.Shadowblade,
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

func ValidateClass(actionID string, actual classid.ID) error {
	policy, ok := ForAction(actionID)
	if !ok || policy.RequiredClass == "" {
		return nil
	}
	if actual != policy.RequiredClass {
		return ErrWrongClass
	}
	return nil
}
