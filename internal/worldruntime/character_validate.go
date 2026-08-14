package worldruntime

import (
	"strconv"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/world"
)

func (r *Runtime) validateEntityTarget(actor world.EntityState, prepared combat.PreparedAction) (world.EntityID, error) {
	rawID, err := strconv.ParseUint(prepared.Target.ID, 10, 64)
	if err != nil || rawID == 0 { return 0, ErrInvalidEntityTarget }
	targetID := world.EntityID(rawID)
	if targetID == actor.ID { return 0, ErrSelfTarget }
	target, ok := r.world.Entity(targetID)
	if !ok { return 0, ErrInvalidEntityTarget }
	state, ok := r.characters.State(targetID)
	if !ok { return 0, character.ErrCharacterNotFound }
	if state.Defeated { return 0, character.ErrCharacterDefeated }
	if actor.Transform.Position.Layer != target.Transform.Position.Layer { return 0, ErrEntityWrongLayer }
	rangeSq := prepared.Definition.Range * prepared.Definition.Range
	if actor.Transform.Position.DistanceSquared(target.Transform.Position) > rangeSq { return 0, ErrEntityOutOfRange }
	if r.dynamic == nil { return 0, ErrDynamicWorldUnavailable }
	if !r.dynamic.HasLineOfSight(actor.Transform.Position, target.Transform.Position) { return 0, ErrEntityNoLineOfSight }
	return targetID, nil
}
