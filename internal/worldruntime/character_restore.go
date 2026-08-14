package worldruntime

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterRestoreRequiresTrustedIdentity = errors.New("worldruntime: character restore requires trusted identity")
	ErrCharacterRestoreIdentityMismatch        = errors.New("worldruntime: character restore identity mismatch")
	ErrCharacterRestoreWorldMismatch           = errors.New("worldruntime: character restore gameplay world mismatch")
	ErrCharacterRestoreInvalid                 = errors.New("worldruntime: invalid character restore")
	ErrCharacterRestoreDefeatedUnsupported     = errors.New("worldruntime: defeated character restore is not supported")
)

// CharacterRestore is immutable durable state prepared outside the world owner.
// Store/network I/O must complete before this value is enqueued with JoinRequest.
type CharacterRestore struct {
	CharacterID characteridentity.ID
	Revision    uint64
	World       protocol.WorldIdentity
	HP          uint32
	MaxHP       uint32
	Defeated    bool
	Transform   world.Transform
}

func CharacterRestoreFromRecord(record characterstate.Record) CharacterRestore {
	return CharacterRestore{
		CharacterID: record.CharacterID,
		Revision:    record.Revision,
		World: protocol.WorldIdentity{
			WorldID:        record.Snapshot.World.WorldID,
			Revision:       record.Snapshot.World.Revision,
			GameplaySHA256: record.Snapshot.World.GameplaySHA256,
		},
		HP:        record.Snapshot.HP,
		MaxHP:     record.Snapshot.MaxHP,
		Defeated:  record.Snapshot.Defeated,
		Transform: world.Transform{Position: record.Snapshot.Position, Yaw: record.Snapshot.Yaw},
	}
}

// ValidateCharacterRestore is shared by the transport pre-welcome boundary and the
// world-owner join transaction. It deliberately accepts only exact Gameplay World
// provenance; migration between world revisions is a separate policy stage.
func ValidateCharacterRestore(identity characteridentity.Binding, restore CharacterRestore, currentWorld protocol.WorldIdentity) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrCharacterRestoreRequiresTrustedIdentity
	}
	if restore.CharacterID != identity.ID {
		return ErrCharacterRestoreIdentityMismatch
	}
	if restore.Revision == 0 || !restore.World.Valid() || !currentWorld.Valid() {
		return ErrCharacterRestoreInvalid
	}
	if restore.World != currentWorld {
		return ErrCharacterRestoreWorldMismatch
	}
	if restore.Defeated {
		return ErrCharacterRestoreDefeatedUnsupported
	}
	if restore.MaxHP == 0 || restore.HP == 0 || restore.HP > restore.MaxHP {
		return ErrCharacterRestoreInvalid
	}
	for _, value := range []float32{
		restore.Transform.Position.X,
		restore.Transform.Position.Y,
		restore.Transform.Position.Z,
		restore.Transform.Yaw,
	} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return ErrCharacterRestoreInvalid
		}
	}
	return nil
}
