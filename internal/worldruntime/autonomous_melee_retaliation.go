package worldruntime

import "github.com/li41/astrahold-server/internal/world"

// provokeAutonomousMeleeMonster gives an otherwise-idle autonomous melee monster a Server-owned
// retaliation target after that player has already dealt legal authoritative damage. It deliberately
// does not replace an existing combat target and does not interrupt evade; a full threat table is not
// introduced by this first retaliation slice. Normal autonomous target validation still enforces
// alive state, layer, leash and line of sight on the next AI step.
func (r *Runtime) provokeAutonomousMeleeMonster(monsterID, attackerID world.EntityID) {
	if monsterID == 0 || attackerID == 0 {
		return
	}
	for i := range r.autonomousMeleeAgents {
		agent := &r.autonomousMeleeAgents[i]
		if agent.config.EntityID != monsterID {
			continue
		}
		if agent.returningHome || agent.targetID != 0 {
			return
		}
		agent.targetID = attackerID
		return
	}
}
