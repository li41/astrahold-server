package jsonv1

import (
	"strconv"

	"github.com/li41/astrahold-server/internal/protocol"
)

// AppendMarshal appends the existing strict JSON wire payload to dst. Hot reliable gameplay
// messages use allocation-free writers when their strings are simple protocol identifiers;
// every other shape falls back to Marshal, preserving the exact existing JSON contract.
func (c Codec) AppendMarshal(dst []byte, message protocol.Message) ([]byte, error) {
	switch m := message.(type) {
	case protocol.ActionStarted:
		if m.TargetX == nil && m.TargetZ == nil && simpleJSONString(m.ActionID) && simpleJSONString(string(m.TargetKind)) && simpleJSONString(m.TargetID) {
			return appendActionStarted(dst, m), nil
		}
	case protocol.EntityVitalsState:
		return appendEntityVitalsState(dst, m), nil
	case protocol.CombatEvent:
		if m.ImpactX == nil && m.ImpactZ == nil && simpleJSONString(m.ActionID) && simpleJSONString(string(m.Result)) {
			return appendCombatEvent(dst, m), nil
		}
	}

	payload, err := c.Marshal(message)
	if err != nil {
		return dst, err
	}
	return append(dst, payload...), nil
}

// simpleJSONString accepts the protocol-ID subset whose bytes encoding/json emits verbatim
// between quotes. Anything requiring escaping, HTML escaping or UTF-8 handling uses Marshal.
func simpleJSONString(value string) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b < 0x20 || b >= 0x80 || b == '"' || b == '\\' || b == '<' || b == '>' || b == '&' {
			return false
		}
	}
	return true
}

func appendSimpleJSONString(dst []byte, value string) []byte {
	dst = append(dst, '"')
	dst = append(dst, value...)
	return append(dst, '"')
}

func appendActionStarted(dst []byte, m protocol.ActionStarted) []byte {
	dst = append(dst, `{"action_instance_id":`...)
	dst = strconv.AppendUint(dst, m.ActionInstanceID, 10)
	dst = append(dst, `,"actor_entity_id":`...)
	dst = strconv.AppendUint(dst, uint64(m.ActorEntityID), 10)
	dst = append(dst, `,"action_id":`...)
	dst = appendSimpleJSONString(dst, m.ActionID)
	dst = append(dst, `,"target_kind":`...)
	dst = appendSimpleJSONString(dst, string(m.TargetKind))
	dst = append(dst, `,"target_id":`...)
	dst = appendSimpleJSONString(dst, m.TargetID)
	return append(dst, '}')
}

func appendEntityVitalsState(dst []byte, m protocol.EntityVitalsState) []byte {
	dst = append(dst, `{"entity_id":`...)
	dst = strconv.AppendUint(dst, uint64(m.EntityID), 10)
	dst = append(dst, `,"hp":`...)
	dst = strconv.AppendUint(dst, uint64(m.HP), 10)
	dst = append(dst, `,"max_hp":`...)
	dst = strconv.AppendUint(dst, uint64(m.MaxHP), 10)
	dst = append(dst, `,"mp":`...)
	dst = strconv.AppendUint(dst, uint64(m.MP), 10)
	dst = append(dst, `,"max_mp":`...)
	dst = strconv.AppendUint(dst, uint64(m.MaxMP), 10)
	dst = append(dst, `,"defeated":`...)
	dst = strconv.AppendBool(dst, m.Defeated)
	if m.ReviveProtectionUntilTick != 0 {
		dst = append(dst, `,"revive_protection_until_tick":`...)
		dst = strconv.AppendUint(dst, m.ReviveProtectionUntilTick, 10)
	}
	return append(dst, '}')
}

func appendCombatEvent(dst []byte, m protocol.CombatEvent) []byte {
	dst = append(dst, `{"action_instance_id":`...)
	dst = strconv.AppendUint(dst, m.ActionInstanceID, 10)
	dst = append(dst, `,"actor_entity_id":`...)
	dst = strconv.AppendUint(dst, uint64(m.ActorEntityID), 10)
	dst = append(dst, `,"action_id":`...)
	dst = appendSimpleJSONString(dst, m.ActionID)
	dst = append(dst, `,"result":`...)
	dst = appendSimpleJSONString(dst, string(m.Result))
	dst = append(dst, `,"target_entity_id":`...)
	dst = strconv.AppendUint(dst, uint64(m.TargetEntityID), 10)
	dst = append(dst, `,"damage":`...)
	dst = strconv.AppendUint(dst, uint64(m.Damage), 10)
	dst = append(dst, `,"blocked":`...)
	dst = strconv.AppendBool(dst, m.Blocked)
	if m.CooldownReadyTick != 0 {
		dst = append(dst, `,"cooldown_ready_tick":`...)
		dst = strconv.AppendUint(dst, m.CooldownReadyTick, 10)
	}
	return append(dst, '}')
}
