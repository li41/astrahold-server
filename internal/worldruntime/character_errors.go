package worldruntime

import "errors"

var (
	ErrInvalidEntityTarget          = errors.New("worldruntime: invalid entity target")
	ErrSelfTarget                   = errors.New("worldruntime: self target")
	ErrEntityWrongLayer             = errors.New("worldruntime: entity target wrong layer")
	ErrEntityOutOfRange             = errors.New("worldruntime: entity target out of range")
	ErrEntityNoLineOfSight          = errors.New("worldruntime: entity target no line of sight")
	ErrResurrectionTargetNotPlayer  = errors.New("worldruntime: resurrection target is not a player")
)
