package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var ErrCommandQueueFull = errors.New("worldruntime: command queue full")

type command interface{ name() string }

type registerSessionCommand struct{ session *session.Session }

func (registerSessionCommand) name() string { return "register_session" }

type unregisterSessionCommand struct{ id session.ID }

func (unregisterSessionCommand) name() string { return "unregister_session" }

type characterAdmissionCommand struct {
	identity   characterAdmissionOperation
	completion chan error
}

func (c characterAdmissionCommand) name() string {
	if c.identity.release {
		return "release_character_admission"
	}
	if c.identity.ownership != nil {
		return "plan_character_connection"
	}
	return "admit_character"
}

type joinCommand struct {
	request    JoinRequest
	completion chan error
}

func (joinCommand) name() string { return "join_world" }

type ownershipLookupCommand struct {
	identity   characteridentity.Binding
	result     *SessionOwnershipFence
	completion chan error
}

func (ownershipLookupCommand) name() string { return "lookup_character_ownership" }

type ownershipTransferCommand struct {
	request    OwnershipTransferRequest
	completion chan error
}

func (ownershipTransferCommand) name() string { return "transfer_character_ownership" }

type leaveCommand struct {
	id        session.ID
	ownership SessionOwnershipFence
}

func (leaveCommand) name() string { return "leave_world" }

type moveInputCommand struct {
	sessionID session.ID
	sequence  uint32
	input     protocol.ClientMoveInput
	ownership SessionOwnershipFence
}

func (moveInputCommand) name() string { return "move_input" }

type teleportCommand struct {
	entityID world.EntityID
	position world.Position
}

func (teleportCommand) name() string { return "teleport_entity" }

type teleportBatchCommand struct{ requests []TeleportRequest }

func (teleportBatchCommand) name() string { return "teleport_batch" }

type spawnEntityCommand struct{ request SpawnEntityRequest }

func (spawnEntityCommand) name() string { return "spawn_entity" }

// useActionCommand is the existing bounded Reliable client-intent carrier. Equipment, pickup and
// respawn keep their own typed protocol payloads and are routed before combat preparation; none is a skill.
type useActionCommand struct {
	sessionID session.ID
	sequence  uint32
	action    protocol.ClientUseAction
	equipment *protocol.ClientEquipmentCommand
	pickup    *protocol.ClientPickupItem
	respawn   *protocol.ClientRespawnRequest
	ownership SessionOwnershipFence
}

func (c useActionCommand) name() string {
	if c.equipment != nil {
		return "equipment_command"
	}
	if c.pickup != nil {
		return "pickup_item"
	}
	if c.respawn != nil {
		return "respawn_request"
	}
	return "use_action"
}

type equipmentCommand = useActionCommand

type npcCommand struct {
	sessionID session.ID
	sequence  uint32
	intent    protocol.ClientInteractNPC
	ownership SessionOwnershipFence
}

func (npcCommand) name() string { return "interact_npc" }

type shopCommand struct {
	sessionID session.ID
	sequence  uint32
	intent    protocol.ClientShopCommand
	ownership SessionOwnershipFence
}

func (shopCommand) name() string { return "shop_command" }

// setBlockerCommand remains the existing Step dispatch carrier for DynamicWorld mutations.
type setBlockerCommand struct {
	id                  string
	enabled             bool
	startNextSiegeRound bool
}

func (c setBlockerCommand) name() string {
	if c.startNextSiegeRound {
		return "start_next_siege_round"
	}
	return "set_blocker"
}

type commandQueue struct{ ch chan command }

func newCommandQueue(capacity int) *commandQueue {
	if capacity <= 0 {
		panic("worldruntime: command queue capacity must be > 0")
	}
	return &commandQueue{ch: make(chan command, capacity)}
}
func (q *commandQueue) tryPush(c command) error {
	select {
	case q.ch <- c:
		return nil
	default:
		return ErrCommandQueueFull
	}
}
func (q *commandQueue) drain(max int) []command {
	if max <= 0 {
		return nil
	}
	out := make([]command, 0, max)
	for len(out) < max {
		select {
		case c := <-q.ch:
			out = append(out, c)
		default:
			return out
		}
	}
	return out
}
func (q *commandQueue) depth() int { return len(q.ch) }
