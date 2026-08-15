package worldruntime

import (
	"errors"
	"math"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrCharacterRestoreRequiresTrustedIdentity = errors.New("worldruntime: character restore requires trusted identity")
	ErrCharacterRestoreIdentityMismatch        = errors.New("worldruntime: character restore identity mismatch")
	ErrCharacterRestoreWorldMismatch           = errors.New("worldruntime: character restore gameplay world mismatch")
	ErrCharacterRestoreInvalid                 = errors.New("worldruntime: invalid character restore")
	ErrCharacterRestoreDefeatedUnsupported     = errors.New("worldruntime: legacy defeated character restore is not supported")
	ErrCharacterRestoreRespawnPolicyUnavailable = errors.New("worldruntime: defeated character restore requires respawn policy")
)

// CharacterRestore is immutable durable state prepared outside the world owner.
// Store/network I/O must complete before this value is enqueued with JoinRequest.
type CharacterRestore struct {
	SchemaVersion uint16
	CharacterID   characteridentity.ID
	Revision      uint64
	World         protocol.WorldIdentity
	HP            uint32
	MaxHP         uint32
	Defeated      bool
	Transform     world.Transform
	Respawn       characterstate.DefeatedRespawn
}

func CharacterRestoreFromRecord(record characterstate.Record) CharacterRestore {
	return CharacterRestore{
		SchemaVersion: record.SchemaVersion,
		CharacterID:   record.CharacterID,
		Revision:      record.Revision,
		World: protocol.WorldIdentity{
			WorldID:        record.Snapshot.World.WorldID,
			Revision:       record.Snapshot.World.Revision,
			GameplaySHA256: record.Snapshot.World.GameplaySHA256,
		},
		HP:        record.Snapshot.HP,
		MaxHP:     record.Snapshot.MaxHP,
		Defeated:  record.Snapshot.Defeated,
		Transform: world.Transform{Position: record.Snapshot.Position, Yaw: record.Snapshot.Yaw},
		Respawn:   record.Snapshot.Respawn,
	}
}

// ValidateCharacterRestore is shared by the transport pre-welcome boundary and the
// world-owner join transaction. It accepts exact Gameplay World provenance only.
// Policy-specific destination validation is repeated by Runtime because transport does
// not own respawnpolicy.Service.
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
	if restore.MaxHP == 0 || restore.HP > restore.MaxHP {
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
	if restore.Defeated {
		if restore.SchemaVersion < characterstate.SchemaVersion {
			return ErrCharacterRestoreDefeatedUnsupported
		}
		if restore.HP != 0 || !validRestoreDeathContext(restore.Respawn.Context) || restore.Respawn.SpawnPointID == "" || !validRestoreSpawnClass(restore.Respawn.SpawnClass) {
			return ErrCharacterRestoreInvalid
		}
		for _, value := range []float32{restore.Respawn.Position.X, restore.Respawn.Position.Y, restore.Respawn.Position.Z} {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return ErrCharacterRestoreInvalid
			}
		}
		return nil
	}
	if restore.HP == 0 || restore.Respawn != (characterstate.DefeatedRespawn{}) {
		return ErrCharacterRestoreInvalid
	}
	return nil
}

func validRestoreDeathContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege:
		return true
	default:
		return false
	}
}

func validRestoreSpawnClass(class respawnpolicy.SpawnClass) bool {
	switch class {
	case respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint, respawnpolicy.SpawnClassSiege:
		return true
	default:
		return false
	}
}

func (r *Runtime) validateCharacterRestore(s *session.Session, restore CharacterRestore) error {
	if s == nil {
		return session.ErrInvalidSession
	}
	currentWorld := protocol.WorldIdentity{
		WorldID:        r.characterStateWorld.WorldID,
		Revision:       r.characterStateWorld.Revision,
		GameplaySHA256: r.characterStateWorld.GameplaySHA256,
	}
	return ValidateCharacterRestore(s.CharacterIdentity, restore, currentWorld)
}
