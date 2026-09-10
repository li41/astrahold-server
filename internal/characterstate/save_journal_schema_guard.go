package characterstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// UnmarshalJSON keeps save-journal schema boundaries fail-closed before legacy
// migration can discard fields that did not exist in that schema version. After
// those guards pass, the loadout-only schema is normalized in memory to the
// learned-skill schema using only skills that were already configured in combat.
func (wire *saveJournalWireRecord) UnmarshalJSON(data []byte) error {
	type wireAlias saveJournalWireRecord

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var decoded wireAlias
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return err
	}
	if decoded.SchemaVersion < LoadoutSaveJournalSchemaVersion && len(decoded.Snapshot.CombatLoadout) != 0 {
		return fmt.Errorf("combat loadout requires save journal schema %d", LoadoutSaveJournalSchemaVersion)
	}
	if decoded.SchemaVersion < LearnedSkillsSaveJournalSchemaVersion && len(decoded.Snapshot.LearnedSkills) != 0 {
		return fmt.Errorf("learned skills require save journal schema %d", LearnedSkillsSaveJournalSchemaVersion)
	}

	if decoded.SchemaVersion == LoadoutSaveJournalSchemaVersion {
		decoded.Snapshot.LearnedSkills = append([]string(nil), decoded.Snapshot.CombatLoadout...)
		decoded.SchemaVersion = LearnedSkillsSaveJournalSchemaVersion
	}

	*wire = saveJournalWireRecord(decoded)
	return nil
}
