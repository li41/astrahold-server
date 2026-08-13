package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

var ErrCommandQueueFull = errors.New("worldruntime: command queue full")

type command interface{ name() string }

type registerSessionCommand struct{ session *session.Session }

func (registerSessionCommand) name() string { return "register_session" }

type unregisterSessionCommand struct{ id session.ID }

func (unregisterSessionCommand) name() string { return "unregister_session" }

type joinCommand struct{ request JoinRequest }

func (joinCommand) name() string { return "join_world" }

type leaveCommand struct{ id session.ID }

func (leaveCommand) name() string { return "leave_world" }

type moveInputCommand struct {
	sessionID session.ID
	sequence  uint32
	input     protocol.ClientMoveInput
}

func (moveInputCommand) name() string { return "move_input" }

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
