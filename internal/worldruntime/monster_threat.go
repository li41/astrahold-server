package worldruntime

import (
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/threat"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) autonomousMeleeAgentForEntity(entityID world.EntityID) *autonomousMeleeAgent {
	if r == nil || entityID == 0 {
		return nil
	}
	for i := range r.autonomousMeleeAgents {
		if r.autonomousMeleeAgents[i].config.EntityID == entityID {
			return &r.autonomousMeleeAgents[i]
		}
	}
	return nil
}

func ensureAgentThreatTable(agent *autonomousMeleeAgent) *threat.Table {
	if agent == nil {
		return nil
	}
	if agent.threat == nil {
		agent.threat = threat.New()
	}
	return agent.threat
}

// recordMonsterThreatDamage consumes the same Server-confirmed actual damage fact used by loot
// contribution, but stores it in a fully separate encounter table. Session identity is used only to
// validate that this mutation came from the current authoritative player owner; threat itself is
// keyed by stable EntityID.
func (r *Runtime) recordMonsterThreatDamage(monsterID, actorID world.EntityID, sourceSessionID session.ID, actualDamage uint32) {
	if r == nil || monsterID == 0 || actorID == 0 || sourceSessionID == 0 || actualDamage == 0 {
		return
	}
	agent := r.autonomousMeleeAgentForEntity(monsterID)
	if agent == nil {
		return
	}
	monster, ok := r.world.Entity(monsterID)
	if !ok || monster.Kind != world.EntityMonster {
		return
	}
	actor, ok := r.world.Entity(actorID)
	if !ok || actor.Kind != world.EntityPlayer {
		return
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID {
		return
	}
	ensureAgentThreatTable(agent).Add(actorID, uint64(actualDamage))
}

func (r *Runtime) clearMonsterThreat(monsterID world.EntityID) {
	agent := r.autonomousMeleeAgentForEntity(monsterID)
	if agent == nil || agent.threat == nil {
		return
	}
	agent.threat.Clear()
}

func (r *Runtime) activePlayerSession(entityID world.EntityID) bool {
	if r == nil || entityID == 0 {
		return false
	}
	for _, s := range r.sessions.List() {
		if s.EntityID == entityID {
			return true
		}
	}
	return false
}

// highestAutonomousMeleeThreatTarget returns the highest-threat target still legal for this
// encounter and prunes dead, disconnected, wrong-layer, out-of-leash or LOS-invalid entries.
func (r *Runtime) highestAutonomousMeleeThreatTarget(agent *autonomousMeleeAgent, actor world.EntityState, tick uint64) (world.EntityState, bool) {
	if agent == nil || agent.threat == nil || agent.threat.Len() == 0 {
		return world.EntityState{}, false
	}
	id, _, ok := agent.threat.HighestValid(func(targetID world.EntityID) bool {
		_, valid := r.autonomousMeleeTarget(actor, agent.config, targetID, tick)
		return valid
	})
	if !ok {
		return world.EntityState{}, false
	}
	target, ok := r.world.Entity(id)
	return target, ok
}
