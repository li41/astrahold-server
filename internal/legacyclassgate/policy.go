// Package legacyclassgate contains the temporary fixed-class authorization contract for legacy v27 actions.
// New classless skill legality must not depend on this package.
package legacyclassgate

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
)

// ErrWrongClass preserves the legacy rejection identity and diagnostic text while the v27 action
// contract is still accepted by the Server.
var ErrWrongClass = errors.New("classaction: action unavailable for class")

func requiredClass(actionID string) (classid.ID, bool) {
	switch actionID {
	case "oathguard-sword-strike", "oathguard-fortify":
		return classid.Oathguard, true
	case "breaker-heavy-slash", "breaker-stagger-strike":
		return classid.Breaker, true
	case "ranger-hunting-shot", "ranger-armor-piercing-arrow":
		return classid.Ranger, true
	case "starfire-fire-bolt", "starfire-cold-star-channel":
		return classid.StarfireMage, true
	case "oathhealer-oathlight-strike":
		return classid.Oathhealer, true
	case "shadowblade-dual-blade-strike", "shadowblade-rift-stab", "shadowblade-flaw-execute":
		return classid.Shadowblade, true
	default:
		return "", false
	}
}

// Validate applies only the legacy fixed-class gate. Unknown or classless actions are deliberately
// outside this compatibility contract and pass through to their authoritative legality owner.
func Validate(actionID string, actual classid.ID) error {
	required, ok := requiredClass(actionID)
	if !ok {
		return nil
	}
	if actual != required {
		return ErrWrongClass
	}
	return nil
}
