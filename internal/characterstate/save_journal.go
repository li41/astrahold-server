package characterstate

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/world"
)

const (
	LegacySaveJournalSchemaVersion uint16 = 1
	SaveJournalSchemaVersion       uint16 = 2
	saveCheckpointSchemaVersion    uint16 = 1
	saveJournalIDSize                     = 16
	maxSaveJournalPayload                 = 1 << 20
)

var (
	saveJournalMagic = []byte("ASTRAHOLD-CHARACTER-STATE-SAVE-JOURNAL-V1\n")
	saveCRCTable     = crc32.MakeTable(crc32.Castagnoli)
)

var (
	ErrInvalidSaveJournalPath        = errors.New("characterstate: invalid save journal path")
	ErrCorruptSaveJournal            = errors.New("characterstate: corrupt save journal")
	ErrSaveJournalClosed             = errors.New("characterstate: save journal closed")
	ErrSaveJournalRecordOverflow     = errors.New("characterstate: save journal record id overflow")
	ErrInvalidSaveCheckpointPath     = errors.New("characterstate: invalid save checkpoint path")
	ErrCorruptSaveCheckpoint         = errors.New("characterstate: corrupt save checkpoint")
	ErrSaveCheckpointJournalMismatch = errors.New("characterstate: save checkpoint journal mismatch")
	ErrSaveCheckpointAhead           = errors.New("characterstate: save checkpoint ahead of journal")
	ErrSaveCheckpointOffsetMismatch  = errors.New("characterstate: save checkpoint offset mismatch")
)

// SaveJournalRecord is a durable Store-application command. ExpectedRevision is captured
// immediately before journal append. It makes replay unambiguous: current==expected means
// not yet applied; current==expected+1 with the same snapshot means this exact command was
// applied before a crash but its checkpoint did not advance.
type SaveJournalRecord struct {
	RecordID         uint64
	ExpectedRevision uint64
	Intent           SaveIntent
	EndOffset        int64
}

type SaveCheckpoint struct {
	JournalID string
	RecordID  uint64
	Offset    int64
}

type SaveJournal struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	journalID    [saveJournalIDSize]byte
	journalIDHex string
	lastRecordID uint64
	endOffset    int64
	recordEnds   []int64
	repairedTail bool
	closed       bool
}

type SaveCheckpointStore struct {
	path string
}

type saveJournalWireRecord struct {
	SchemaVersion    uint16                  `json:"schema_version"`
	RecordID         uint64                  `json:"record_id"`
	ExpectedRevision uint64                  `json:"expected_revision"`
	IntentID         uint64                  `json:"intent_id"`
	CharacterID      string                  `json:"character_id"`
	Snapshot         saveJournalWireSnapshot `json:"snapshot"`
}

type saveJournalWireSnapshot struct {
	WorldID         string               `json:"world_id"`
	WorldRevision   string               `json:"world_revision"`
	GameplaySHA256  string               `json:"gameplay_sha256"`
	HP              uint32               `json:"hp"`
	MaxHP           uint32               `json:"max_hp"`
	MP              uint32               `json:"mp,omitempty"`
	MaxMP           uint32               `json:"max_mp,omitempty"`
	Defeated        bool                 `json:"defeated"`
	X               float32              `json:"x"`
	Y               float32              `json:"y"`
	Z               float32              `json:"z"`
	Layer           world.LayerID        `json:"layer"`
	Yaw             float32              `json:"yaw"`
	DefeatedRespawn *wireDefeatedRespawn `json:"defeated_respawn,omitempty"`
}

type saveCheckpointWire struct {
	SchemaVersion uint16 `json:"schema_version"`
	JournalID     string `json:"journal_id"`
	RecordID      uint64 `json:"record_id"`
	Offset        int64  `json:"offset"`
}

func OpenSaveJournal(path string) (*SaveJournal, error) {
	if path == "" {
		return nil, ErrInvalidSaveJournalPath
	}
	if err := ensureSaveJournalParentDir(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	journal := &SaveJournal{file: file, path: path}
	if err := journal.openOrInitialize(); err != nil {
		_ = file.Close()
		return nil, err
	}
	return journal, nil
}

func NewSaveCheckpointStore(path string) (*SaveCheckpointStore, error) {
	if path == "" {
		return nil, ErrInvalidSaveCheckpointPath
	}
	if err := ensureSaveJournalParentDir(path); err != nil {
		return nil, err
	}
	return &SaveCheckpointStore{path: path}, nil
}

func (j *SaveJournal) ID() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.journalIDHex
}

func (j *SaveJournal) Path() string { return j.path }

func (j *SaveJournal) LastRecordID() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lastRecordID
}

func (j *SaveJournal) RepairedTail() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.repairedTail
}

func (j *SaveJournal) InitialCheckpoint() SaveCheckpoint {
	j.mu.Lock()
	defer j.mu.Unlock()
	return SaveCheckpoint{JournalID: j.journalIDHex, Offset: saveJournalHeaderSize()}
}

func (j *SaveJournal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	j.closed = true
	if err := j.file.Sync(); err != nil {
		_ = j.file.Close()
		return err
	}
	return j.file.Close()
}

// Append advances durable save-intent truth only after the complete frame has been fsync'ed.
// expectedRevision is part of the durable command and therefore survives restart.
func (j *SaveJournal) Append(intent SaveIntent, expectedRevision uint64) (SaveJournalRecord, error) {
	if err := validateSaveIntent(intent); err != nil {
		return SaveJournalRecord{}, err
	}
	if expectedRevision == ^uint64(0) {
		return SaveJournalRecord{}, ErrRevisionOverflow
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return SaveJournalRecord{}, ErrSaveJournalClosed
	}
	if j.lastRecordID == ^uint64(0) {
		return SaveJournalRecord{}, ErrSaveJournalRecordOverflow
	}
	stat, err := j.file.Stat()
	if err != nil {
		return SaveJournalRecord{}, err
	}
	if stat.Size() < j.endOffset {
		return SaveJournalRecord{}, fmt.Errorf("%w: journal size=%d below durable end=%d", ErrCorruptSaveJournal, stat.Size(), j.endOffset)
	}
	if stat.Size() > j.endOffset {
		if err := j.file.Truncate(j.endOffset); err != nil {
			return SaveJournalRecord{}, err
		}
	}

	recordID := j.lastRecordID + 1
	payload, err := encodeSaveJournalRecord(recordID, expectedRevision, intent)
	if err != nil {
		return SaveJournalRecord{}, err
	}
	frame := makeSaveJournalFrame(payload)
	written, err := j.file.WriteAt(frame, j.endOffset)
	if err != nil {
		return SaveJournalRecord{}, err
	}
	if written != len(frame) {
		return SaveJournalRecord{}, io.ErrShortWrite
	}
	if err := j.file.Sync(); err != nil {
		return SaveJournalRecord{}, err
	}

	end := j.endOffset + int64(len(frame))
	j.lastRecordID = recordID
	j.endOffset = end
	j.recordEnds = append(j.recordEnds, end)
	return SaveJournalRecord{RecordID: recordID, ExpectedRevision: expectedRevision, Intent: intent, EndOffset: end}, nil
}

func (j *SaveJournal) RecordsAfter(checkpoint SaveCheckpoint, limit int) ([]SaveJournalRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, ErrSaveJournalClosed
	}
	if err := j.validateCheckpointLocked(checkpoint); err != nil {
		return nil, err
	}
	if checkpoint.RecordID == j.lastRecordID {
		return nil, nil
	}

	count := int(j.lastRecordID - checkpoint.RecordID)
	if limit > 0 && limit < count {
		count = limit
	}
	out := make([]SaveJournalRecord, 0, count)
	offset := checkpoint.Offset
	expectedRecordID := checkpoint.RecordID + 1
	for len(out) < count {
		record, next, err := j.readRecordAtLocked(offset)
		if err != nil {
			return nil, err
		}
		if record.RecordID != expectedRecordID {
			return nil, fmt.Errorf("%w: record id got=%d want=%d", ErrCorruptSaveJournal, record.RecordID, expectedRecordID)
		}
		record.EndOffset = next
		out = append(out, record)
		offset = next
		expectedRecordID++
	}
	return out, nil
}

func (s *SaveCheckpointStore) Path() string { return s.path }

func (s *SaveCheckpointStore) Load(journal *SaveJournal) (SaveCheckpoint, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return journal.InitialCheckpoint(), nil
	}
	if err != nil {
		return SaveCheckpoint{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire saveCheckpointWire
	if err := decoder.Decode(&wire); err != nil {
		return SaveCheckpoint{}, fmt.Errorf("%w: decode: %v", ErrCorruptSaveCheckpoint, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return SaveCheckpoint{}, fmt.Errorf("%w: trailing JSON value", ErrCorruptSaveCheckpoint)
		}
		return SaveCheckpoint{}, fmt.Errorf("%w: trailing data: %v", ErrCorruptSaveCheckpoint, err)
	}
	if wire.SchemaVersion != saveCheckpointSchemaVersion || wire.JournalID == "" || wire.Offset <= 0 {
		return SaveCheckpoint{}, ErrCorruptSaveCheckpoint
	}
	checkpoint := SaveCheckpoint{JournalID: wire.JournalID, RecordID: wire.RecordID, Offset: wire.Offset}
	if err := journal.ValidateCheckpoint(checkpoint); err != nil {
		return SaveCheckpoint{}, err
	}
	return checkpoint, nil
}

func (s *SaveCheckpointStore) Save(journal *SaveJournal, record SaveJournalRecord) (SaveCheckpoint, error) {
	checkpoint, err := journal.checkpointForRecord(record)
	if err != nil {
		return SaveCheckpoint{}, err
	}
	wire := saveCheckpointWire{
		SchemaVersion: saveCheckpointSchemaVersion,
		JournalID:     checkpoint.JournalID,
		RecordID:      checkpoint.RecordID,
		Offset:        checkpoint.Offset,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return SaveCheckpoint{}, err
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".tmp-")
	if err != nil {
		return SaveCheckpoint{}, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return SaveCheckpoint{}, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return SaveCheckpoint{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return SaveCheckpoint{}, err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return SaveCheckpoint{}, err
	}
	if err := syncDirectory(dir); err != nil {
		return SaveCheckpoint{}, err
	}
	return checkpoint, nil
}

func (j *SaveJournal) ValidateCheckpoint(checkpoint SaveCheckpoint) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.validateCheckpointLocked(checkpoint)
}

func (j *SaveJournal) checkpointForRecord(record SaveJournalRecord) (SaveCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return SaveCheckpoint{}, ErrSaveJournalClosed
	}
	if record.RecordID == 0 || record.RecordID > j.lastRecordID {
		return SaveCheckpoint{}, ErrSaveCheckpointAhead
	}
	expectedEnd := j.recordEnds[record.RecordID-1]
	if record.EndOffset != expectedEnd {
		return SaveCheckpoint{}, ErrSaveCheckpointOffsetMismatch
	}
	return SaveCheckpoint{JournalID: j.journalIDHex, RecordID: record.RecordID, Offset: expectedEnd}, nil
}

func (j *SaveJournal) validateCheckpointLocked(checkpoint SaveCheckpoint) error {
	if checkpoint.JournalID != j.journalIDHex {
		return ErrSaveCheckpointJournalMismatch
	}
	if checkpoint.RecordID > j.lastRecordID {
		return ErrSaveCheckpointAhead
	}
	expectedOffset := saveJournalHeaderSize()
	if checkpoint.RecordID > 0 {
		expectedOffset = j.recordEnds[checkpoint.RecordID-1]
	}
	if checkpoint.Offset != expectedOffset {
		return fmt.Errorf("%w: got=%d want=%d", ErrSaveCheckpointOffsetMismatch, checkpoint.Offset, expectedOffset)
	}
	return nil
}

func (j *SaveJournal) openOrInitialize() error {
	stat, err := j.file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		if _, err := rand.Read(j.journalID[:]); err != nil {
			return err
		}
		header := make([]byte, 0, saveJournalHeaderSize())
		header = append(header, saveJournalMagic...)
		header = append(header, j.journalID[:]...)
		if _, err := j.file.WriteAt(header, 0); err != nil {
			return err
		}
		if err := j.file.Sync(); err != nil {
			return err
		}
		j.journalIDHex = hex.EncodeToString(j.journalID[:])
		j.endOffset = saveJournalHeaderSize()
		return nil
	}
	if stat.Size() < saveJournalHeaderSize() {
		return fmt.Errorf("%w: incomplete header", ErrCorruptSaveJournal)
	}
	header := make([]byte, saveJournalHeaderSize())
	if _, err := j.file.ReadAt(header, 0); err != nil {
		return err
	}
	if !bytes.Equal(header[:len(saveJournalMagic)], saveJournalMagic) {
		return fmt.Errorf("%w: bad magic", ErrCorruptSaveJournal)
	}
	copy(j.journalID[:], header[len(saveJournalMagic):])
	allZero := true
	for _, value := range j.journalID {
		if value != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return fmt.Errorf("%w: empty journal id", ErrCorruptSaveJournal)
	}
	j.journalIDHex = hex.EncodeToString(j.journalID[:])
	return j.scanExisting(stat.Size())
}

func (j *SaveJournal) scanExisting(size int64) error {
	offset := saveJournalHeaderSize()
	expectedRecordID := uint64(1)
	for offset < size {
		remaining := size - offset
		if remaining < 4 {
			return j.repairTornTail(offset)
		}
		var lengthBytes [4]byte
		if _, err := j.file.ReadAt(lengthBytes[:], offset); err != nil {
			return err
		}
		length := binary.BigEndian.Uint32(lengthBytes[:])
		if length == 0 || length > maxSaveJournalPayload {
			return fmt.Errorf("%w: invalid payload length=%d at offset=%d", ErrCorruptSaveJournal, length, offset)
		}
		frameEnd := offset + 4 + int64(length) + 4
		if frameEnd > size {
			return j.repairTornTail(offset)
		}
		record, next, err := j.readRecordAtLocked(offset)
		if err != nil {
			return err
		}
		if next != frameEnd || record.RecordID != expectedRecordID {
			return fmt.Errorf("%w: record id got=%d want=%d", ErrCorruptSaveJournal, record.RecordID, expectedRecordID)
		}
		j.recordEnds = append(j.recordEnds, next)
		j.lastRecordID = record.RecordID
		j.endOffset = next
		offset = next
		expectedRecordID++
	}
	if j.endOffset == 0 {
		j.endOffset = saveJournalHeaderSize()
	}
	return nil
}

func (j *SaveJournal) repairTornTail(offset int64) error {
	if err := j.file.Truncate(offset); err != nil {
		return err
	}
	if err := j.file.Sync(); err != nil {
		return err
	}
	j.endOffset = offset
	j.repairedTail = true
	return nil
}

func (j *SaveJournal) readRecordAtLocked(offset int64) (SaveJournalRecord, int64, error) {
	var lengthBytes [4]byte
	if _, err := j.file.ReadAt(lengthBytes[:], offset); err != nil {
		return SaveJournalRecord{}, 0, err
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > maxSaveJournalPayload {
		return SaveJournalRecord{}, 0, fmt.Errorf("%w: invalid payload length=%d", ErrCorruptSaveJournal, length)
	}
	payload := make([]byte, int(length))
	if _, err := j.file.ReadAt(payload, offset+4); err != nil {
		return SaveJournalRecord{}, 0, err
	}
	var crcBytes [4]byte
	if _, err := j.file.ReadAt(crcBytes[:], offset+4+int64(length)); err != nil {
		return SaveJournalRecord{}, 0, err
	}
	wantCRC := binary.BigEndian.Uint32(crcBytes[:])
	gotCRC := crc32.Checksum(payload, saveCRCTable)
	if gotCRC != wantCRC {
		return SaveJournalRecord{}, 0, fmt.Errorf("%w: crc mismatch at offset=%d", ErrCorruptSaveJournal, offset)
	}
	recordID, expectedRevision, intent, err := decodeSaveJournalRecord(payload)
	if err != nil {
		return SaveJournalRecord{}, 0, err
	}
	next := offset + 4 + int64(length) + 4
	return SaveJournalRecord{RecordID: recordID, ExpectedRevision: expectedRevision, Intent: intent, EndOffset: next}, next, nil
}

func encodeSaveJournalRecord(recordID, expectedRevision uint64, intent SaveIntent) ([]byte, error) {
	wire := saveJournalWireRecord{
		SchemaVersion:    SaveJournalSchemaVersion,
		RecordID:         recordID,
		ExpectedRevision: expectedRevision,
		IntentID:         intent.IntentID,
		CharacterID:      string(intent.Identity.ID),
		Snapshot:         snapshotToSaveJournalWire(intent.Snapshot),
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > maxSaveJournalPayload {
		return nil, fmt.Errorf("%w: encoded payload size=%d", ErrInvalidSnapshot, len(payload))
	}
	return payload, nil
}

func decodeSaveJournalRecord(payload []byte) (uint64, uint64, SaveIntent, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire saveJournalWireRecord
	if err := decoder.Decode(&wire); err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: decode record: %v", ErrCorruptSaveJournal, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: trailing record data", ErrCorruptSaveJournal)
	}
	if (wire.SchemaVersion != LegacySaveJournalSchemaVersion && wire.SchemaVersion != SaveJournalSchemaVersion) || wire.RecordID == 0 || wire.IntentID == 0 || wire.ExpectedRevision == ^uint64(0) {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: invalid record header", ErrCorruptSaveJournal)
	}
	identity, err := characteridentity.NewTrusted(wire.CharacterID)
	if err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: character identity: %v", ErrCorruptSaveJournal, err)
	}
	intent := SaveIntent{IntentID: wire.IntentID, Identity: identity, Snapshot: saveJournalWireToSnapshot(wire.SchemaVersion, wire.Snapshot)}
	if err := validateSaveIntent(intent); err != nil {
		return 0, 0, SaveIntent{}, fmt.Errorf("%w: intent: %v", ErrCorruptSaveJournal, err)
	}
	return wire.RecordID, wire.ExpectedRevision, intent, nil
}

func snapshotToSaveJournalWire(snapshot Snapshot) saveJournalWireSnapshot {
	wire := saveJournalWireSnapshot{
		WorldID: snapshot.World.WorldID, WorldRevision: snapshot.World.Revision, GameplaySHA256: snapshot.World.GameplaySHA256,
		HP: snapshot.HP, MaxHP: snapshot.MaxHP, MP: snapshot.MP, MaxMP: snapshot.MaxMP, Defeated: snapshot.Defeated,
		X: snapshot.Position.X, Y: snapshot.Position.Y, Z: snapshot.Position.Z, Layer: snapshot.Position.Layer, Yaw: snapshot.Yaw,
	}
	if snapshot.Defeated {
		respawn := snapshot.Respawn
		wire.DefeatedRespawn = &wireDefeatedRespawn{
			Context: respawn.Context, SpawnPointID: respawn.SpawnPointID, SpawnClass: respawn.SpawnClass,
			X: respawn.Position.X, Y: respawn.Position.Y, Z: respawn.Position.Z, Layer: respawn.Position.Layer,
			RemainingTicks: respawn.RemainingTicks, CheckpointID: respawn.CheckpointID,
		}
	}
	return wire
}

func saveJournalWireToSnapshot(schemaVersion uint16, wire saveJournalWireSnapshot) Snapshot {
	mp, maxMP := wire.MP, wire.MaxMP
	if schemaVersion == LegacySaveJournalSchemaVersion {
		// v1 save-journal records predate authoritative MP persistence. Matching Store.Load
		// compatibility, they resume from the legacy full resource pool rather than inventing
		// a partially known resource state after restart.
		mp, maxMP = LegacyDefaultMaxMP, LegacyDefaultMaxMP
	}
	snapshot := Snapshot{
		World: WorldRef{WorldID: wire.WorldID, Revision: wire.WorldRevision, GameplaySHA256: wire.GameplaySHA256},
		HP: wire.HP, MaxHP: wire.MaxHP, MP: mp, MaxMP: maxMP, Defeated: wire.Defeated,
		Position: world.Position{X: wire.X, Y: wire.Y, Z: wire.Z, Layer: wire.Layer}, Yaw: wire.Yaw,
	}
	if wire.DefeatedRespawn != nil {
		snapshot.Respawn = DefeatedRespawn{
			Context: wire.DefeatedRespawn.Context, SpawnPointID: wire.DefeatedRespawn.SpawnPointID, SpawnClass: wire.DefeatedRespawn.SpawnClass,
			Position: world.Position{X: wire.DefeatedRespawn.X, Y: wire.DefeatedRespawn.Y, Z: wire.DefeatedRespawn.Z, Layer: wire.DefeatedRespawn.Layer},
			RemainingTicks: wire.DefeatedRespawn.RemainingTicks, CheckpointID: wire.DefeatedRespawn.CheckpointID,
		}
	}
	return snapshot
}

func validateSaveIntent(intent SaveIntent) error {
	if intent.IntentID == 0 {
		return ErrUnknownSaveIntent
	}
	if err := validateTrustedIdentity(intent.Identity); err != nil {
		return err
	}
	return validateSnapshotV3(intent.Snapshot)
}

func makeSaveJournalFrame(payload []byte) []byte {
	frame := make([]byte, 4+len(payload)+4)
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	binary.BigEndian.PutUint32(frame[4+len(payload):], crc32.Checksum(payload, saveCRCTable))
	return frame
}

func saveJournalHeaderSize() int64 {
	return int64(len(saveJournalMagic) + saveJournalIDSize)
}

func ensureSaveJournalParentDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o750)
}
