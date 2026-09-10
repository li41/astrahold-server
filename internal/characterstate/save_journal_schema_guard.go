package characterstate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// UnmarshalJSON keeps save-journal schema boundaries fail-closed before legacy
// migration can discard fields that did not exist in that schema version.
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

	*wire = saveJournalWireRecord(decoded)
	return nil
}
