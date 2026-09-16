package characterstate

import (
	"crypto/sha256"
	"fmt"
	"math"
	"path/filepath"
	"strings"

	"github.com/li41/astrahold-server/internal/appearance"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func primaryStatsToWire(primary characterstats.Primary) *wirePrimaryStats {
	constitution, intelligence, spirit, charisma := primary.Constitution, primary.Intelligence, primary.Spirit, primary.Charisma
	return &wirePrimaryStats{Strength: primary.Strength, Agility: primary.Agility, Constitution: &constitution, Intelligence: &intelligence, Spirit: &spirit, Charisma: &charisma}
}

func hasSixPrimaryWireFields(wire *wirePrimaryStats) bool {
	return wire != nil && wire.Constitution != nil && wire.Intelligence != nil && wire.Spirit != nil && wire.Charisma != nil
}

func primaryStatsFromWire(schemaVersion uint16, wire *wirePrimaryStats) (characterstats.Primary, error) {
	if schemaVersion < PrimaryStatsSchemaVersion {
		return characterstats.DefaultPrimary(), nil
	}
	if wire == nil {
		return characterstats.Primary{}, ErrInvalidSnapshot
	}
	if schemaVersion == PrimaryStatsSchemaVersion {
		if hasSixPrimaryWireFields(wire) {
			return characterstats.Primary{}, ErrInvalidSnapshot
		}
		primary := characterstats.DefaultPrimary()
		switch {
		case wire.Strength == 0:
		case wire.Strength < characterstats.BasePrimaryValue:
			return characterstats.Primary{}, ErrInvalidSnapshot
		default:
			primary.Strength = wire.Strength
		}
		switch {
		case wire.Agility == 0:
		case wire.Agility < characterstats.BasePrimaryValue:
			return characterstats.Primary{}, ErrInvalidSnapshot
		default:
			primary.Agility = wire.Agility
		}
		return primary, nil
	}
	if !hasSixPrimaryWireFields(wire) {
		return characterstats.Primary{}, ErrInvalidSnapshot
	}
	primary := characterstats.Primary{Strength: wire.Strength, Agility: wire.Agility, Constitution: *wire.Constitution, Intelligence: *wire.Intelligence, Spirit: *wire.Spirit, Charisma: *wire.Charisma}
	if err := characterstats.ValidateBase(primary); err != nil {
		return characterstats.Primary{}, ErrInvalidSnapshot
	}
	return primary, nil
}

func (s *Store) recordPath(id characteridentity.ID) string {
	sum := sha256.Sum256([]byte(id))
	return filepath.Join(s.root, fmt.Sprintf("%x.json", sum[:]))
}

func validateTrustedIdentity(identity characteridentity.Binding) error {
	if !identity.Valid() || identity.Assurance != characteridentity.AssuranceTrusted {
		return ErrIdentityNotDurable
	}
	return nil
}

func validateSnapshotV14(snapshot Snapshot) error {
	if !validMapID(snapshot.World.MapID) {
		return ErrInvalidSnapshot
	}
	return validateSnapshotV13(snapshot)
}

func validateSnapshotV13(snapshot Snapshot) error {
	if !appearance.ValidSelection(snapshot.SkinID) {
		return ErrInvalidSnapshot
	}
	return validateSnapshotV12(snapshot)
}
func validateSnapshotV12(snapshot Snapshot) error {
	if err := characterstats.ValidateBase(snapshot.PrimaryStats); err != nil {
		return ErrInvalidSnapshot
	}
	return validateSnapshotV11(snapshot)
}

func validateSnapshotV11(snapshot Snapshot) error { return validateSnapshotV10(snapshot) }
func validateSnapshotV10(snapshot Snapshot) error { return validateSnapshotV8(snapshot) }
func validateSnapshotV9(snapshot Snapshot) error {
	if snapshot.Inventory.HasItemInstances() {
		return ErrInvalidSnapshot
	}
	return validateSnapshotV8(snapshot)
}
func validateSnapshotV8(snapshot Snapshot) error {
	if err := validateSnapshotV7(snapshot); err != nil {
		return err
	}
	if err := validateLearnedSkills(snapshot.LearnedSkills); err != nil {
		return err
	}
	return validateCombatLoadoutLearned(snapshot.CombatLoadout, snapshot.LearnedSkills)
}
func validateSnapshotV7(snapshot Snapshot) error {
	if err := validateSnapshotV6(snapshot); err != nil {
		return err
	}
	return validateCombatLoadout(snapshot.CombatLoadout)
}
func validateSnapshotV6(snapshot Snapshot) error { return validateSnapshotV5(snapshot) }
func validateSnapshotV5(snapshot Snapshot) error {
	if err := validateSnapshotV3(snapshot); err != nil {
		return err
	}
	return validateInventoryState(snapshot.Inventory)
}
func validateSnapshotV4(snapshot Snapshot) error {
	if err := validateSnapshotV3(snapshot); err != nil {
		return err
	}
	if snapshot.Inventory.OffHand != "" {
		return ErrInvalidSnapshot
	}
	return validateInventoryState(snapshot.Inventory)
}
func validateSnapshotV3(snapshot Snapshot) error {
	if err := validateSnapshotBase(snapshot); err != nil {
		return err
	}
	if err := validateRespawnSnapshot(snapshot); err != nil {
		return err
	}
	if snapshot.MaxMP == 0 || snapshot.MP > snapshot.MaxMP {
		return ErrInvalidSnapshot
	}
	return nil
}

func validateRespawnSnapshot(snapshot Snapshot) error {
	if snapshot.Defeated {
		if !validDeathContext(snapshot.Respawn.Context) || snapshot.Respawn.SpawnPointID == "" || !validSpawnClass(snapshot.Respawn.SpawnClass) || !finitePosition(snapshot.Respawn.Position) {
			return ErrInvalidSnapshot
		}
		return nil
	}
	if snapshot.Respawn != (DefeatedRespawn{}) {
		return ErrInvalidSnapshot
	}
	return nil
}

func validateSnapshotBase(snapshot Snapshot) error {
	if snapshot.World.WorldID == "" || snapshot.World.Revision == "" || !validSHA256(snapshot.World.GameplaySHA256) {
		return ErrInvalidSnapshot
	}
	if snapshot.MaxHP == 0 || snapshot.HP > snapshot.MaxHP {
		return ErrInvalidSnapshot
	}
	if snapshot.Defeated {
		if snapshot.HP != 0 {
			return ErrInvalidSnapshot
		}
	} else if snapshot.HP == 0 {
		return ErrInvalidSnapshot
	}
	if !finitePosition(snapshot.Position) || !finite32(snapshot.Yaw) {
		return ErrInvalidSnapshot
	}
	if err := characterstats.ValidateBase(snapshot.PrimaryStats); err != nil {
		return ErrInvalidSnapshot
	}
	return nil
}

func validDeathContext(context respawnpolicy.DeathContext) bool {
	switch context {
	case respawnpolicy.DeathContextPvE, respawnpolicy.DeathContextPvP, respawnpolicy.DeathContextSiege:
		return true
	default:
		return false
	}
}
func validSpawnClass(class respawnpolicy.SpawnClass) bool {
	switch class {
	case respawnpolicy.SpawnClassSafe, respawnpolicy.SpawnClassCheckpoint, respawnpolicy.SpawnClassSiege:
		return true
	default:
		return false
	}
}
func finitePosition(position world.Position) bool {
	return finite32(position.X) && finite32(position.Y) && finite32(position.Z)
}
func finite32(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
func validMapID(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') {
			continue
		}
		return false
	}
	return true
}
