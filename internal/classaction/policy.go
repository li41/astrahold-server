// Package classaction defines canonical profession requirements and class-resource outcomes for actions.
package classaction

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/classresource"
)

const (
	OathguardSwordStrike       = "oathguard-sword-strike"
	BreakerHeavySlash          = "breaker-heavy-slash"
	RangerHuntingShot          = "ranger-hunting-shot"
	StarfireFireBolt           = "starfire-fire-bolt"
	OathhealerOathlightStrike  = "oathhealer-oathlight-strike"
)

var ErrWrongClass = errors.New("classaction: action unavailable for class")

type Policy struct {
	RequiredClass       classid.ID
	AcceptedResource    classresource.ID
	AcceptedGain        uint32
	HitResource         classresource.ID
	HitGain             uint32
	HitProgressResource classresource.ID
	HitProgressGain     uint32
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
