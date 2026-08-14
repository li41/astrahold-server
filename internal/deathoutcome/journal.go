package deathoutcome

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

	"github.com/li41/astrahold-server/internal/world"
)

const (
	JournalSchemaVersion    uint16 = 1
	checkpointSchemaVersion uint16 = 1
	journalIDSize                  = 16
	maxJournalPayload              = 1 << 20
)

var (
	journalMagic = []byte("ASTRAHOLD-DEATH-OUTCOME-JOURNAL-V1\n")
	crcTable     = crc32.MakeTable(crc32.Castagnoli)
)

var (
	ErrInvalidJournalPath       = errors.New("deathoutcome: invalid journal path")
	ErrCorruptJournal           = errors.New("deathoutcome: corrupt journal")
	ErrJournalClosed            = errors.New("deathoutcome: journal closed")
	ErrJournalRecordOverflow    = errors.New("deathoutcome: journal record id overflow")
	ErrInvalidCheckpointPath    = errors.New("deathoutcome: invalid checkpoint path")
	ErrCorruptCheckpoint        = errors.New("deathoutcome: corrupt checkpoint")
	ErrCheckpointJournalMismatch = errors.New("deathoutcome: checkpoint journal mismatch")
	ErrCheckpointAhead          = errors.New("deathoutcome: checkpoint ahead of journal")
	ErrCheckpointOffsetMismatch = errors.New("deathoutcome: checkpoint offset mismatch")
)

type JournalRecord struct {
	RecordID  uint64
	Event     Event
	EndOffset int64
}

type Checkpoint struct {
	JournalID string
	RecordID  uint64
	Offset    int64
}

type Journal struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	journalID    [journalIDSize]byte
	journalIDHex string
	lastRecordID uint64
	endOffset    int64
	recordEnds   []int64
	repairedTail bool
	closed       bool
}

type journalWireRecord struct {
	SchemaVersion uint16           `json:"schema_version"`
	RecordID      uint64           `json:"record_id"`
	Event         journalWireEvent `json:"event"`
}

type journalWireEvent struct {
	EventID                    uint64             `json:"event_id"`
	EntityID                   world.EntityID     `json:"entity_id"`
	DefeatRevision             uint64             `json:"defeat_revision"`
	Context                    string             `json:"context"`
	DefeatedTick               uint64             `json:"defeated_tick"`
	RespawnPolicyRevision      string             `json:"respawn_policy_revision"`
	DeathPenaltyPolicyRevision string             `json:"death_penalty_policy_revision"`
	Respawn                    journalWireRespawn `json:"respawn"`
	PenaltyTransactionApplied  bool               `json:"penalty_transaction_applied"`
	CheckpointForfeited        bool               `json:"checkpoint_forfeited"`
}

type journalWireRespawn struct {
	Scheduled    bool          `json:"scheduled"`
	SpawnPointID string        `json:"spawn_point_id"`
	SpawnClass   string        `json:"spawn_class"`
	X            float32       `json:"x"`
	Y            float32       `json:"y"`
	Z            float32       `json:"z"`
	Layer        world.LayerID `json:"layer"`
	DueTick      uint64        `json:"due_tick"`
}

type checkpointWire struct {
	SchemaVersion uint16 `json:"schema_version"`
	JournalID     string `json:"journal_id"`
	RecordID      uint64 `json:"record_id"`
	Offset        int64  `json:"offset"`
}

type CheckpointStore struct {
	path string
}

func OpenJournal(path string) (*Journal, error) {
	if path == "" {
		return nil, ErrInvalidJournalPath
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	journal := &Journal{file: file, path: path}
	if err := journal.openOrInitialize(); err != nil {
		file.Close()
		return nil, err
	}
	return journal, nil
}

func NewCheckpointStore(path string) (*CheckpointStore, error) {
	if path == "" {
		return nil, ErrInvalidCheckpointPath
	}
	if err := ensureParentDir(path); err != nil {
		return nil, err
	}
	return &CheckpointStore{path: path}, nil
}

func (j *Journal) ID() string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.journalIDHex
}

func (j *Journal) Path() string { return j.path }

func (j *Journal) LastRecordID() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.lastRecordID
}

func (j *Journal) RepairedTail() bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.repairedTail
}

func (j *Journal) InitialCheckpoint() Checkpoint {
	j.mu.Lock()
	defer j.mu.Unlock()
	return Checkpoint{JournalID: j.journalIDHex, Offset: journalHeaderSize()}
}

func (j *Journal) Close() error {
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

// Append only advances journal truth after the complete frame has been fsync'ed.
// A failed write/sync leaves endOffset unchanged, so the same offset is overwritten on retry.
func (j *Journal) Append(event Event) (JournalRecord, error) {
	if err := validateJournalEvent(event); err != nil {
		return JournalRecord{}, err
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return JournalRecord{}, ErrJournalClosed
	}
	if j.lastRecordID == ^uint64(0) {
		return JournalRecord{}, ErrJournalRecordOverflow
	}
	if stat, err := j.file.Stat(); err != nil {
		return JournalRecord{}, err
	} else if stat.Size() != j.endOffset {
		if err := j.file.Truncate(j.endOffset); err != nil {
			return JournalRecord{}, err
		}
	}

	recordID := j.lastRecordID + 1
	payload, err := encodeJournalRecord(recordID, event)
	if err != nil {
		return JournalRecord{}, err
	}
	frame := makeFrame(payload)
	written, err := j.file.WriteAt(frame, j.endOffset)
	if err != nil {
		return JournalRecord{}, err
	}
	if written != len(frame) {
		return JournalRecord{}, io.ErrShortWrite
	}
	if err := j.file.Sync(); err != nil {
		return JournalRecord{}, err
	}

	end := j.endOffset + int64(len(frame))
	j.lastRecordID = recordID
	j.endOffset = end
	j.recordEnds = append(j.recordEnds, end)
	return JournalRecord{RecordID: recordID, Event: event, EndOffset: end}, nil
}

// RecordsAfter returns durable records after a validated checkpoint. It never advances checkpoint truth.
func (j *Journal) RecordsAfter(checkpoint Checkpoint, limit int) ([]JournalRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil, ErrJournalClosed
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
	out := make([]JournalRecord, 0, count)
	offset := checkpoint.Offset
	expected := checkpoint.RecordID + 1
	for len(out) < count {
		record, next, err := j.readRecordAtLocked(offset)
		if err != nil {
			return nil, err
		}
		if record.RecordID != expected {
			return nil, fmt.Errorf("%w: record id got=%d want=%d", ErrCorruptJournal, record.RecordID, expected)
		}
		record.EndOffset = next
		out = append(out, record)
		offset = next
		expected++
	}
	return out, nil
}

func (s *CheckpointStore) Path() string { return s.path }

func (s *CheckpointStore) Load(journal *Journal) (Checkpoint, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return journal.InitialCheckpoint(), nil
	}
	if err != nil {
		return Checkpoint{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var wire checkpointWire
	if err := decoder.Decode(&wire); err != nil {
		return Checkpoint{}, fmt.Errorf("%w: decode: %v", ErrCorruptCheckpoint, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Checkpoint{}, fmt.Errorf("%w: trailing JSON value", ErrCorruptCheckpoint)
		}
		return Checkpoint{}, fmt.Errorf("%w: trailing data: %v", ErrCorruptCheckpoint, err)
	}
	if wire.SchemaVersion != checkpointSchemaVersion || wire.JournalID == "" || wire.Offset <= 0 {
		return Checkpoint{}, ErrCorruptCheckpoint
	}
	checkpoint := Checkpoint{JournalID: wire.JournalID, RecordID: wire.RecordID, Offset: wire.Offset}
	if err := journal.ValidateCheckpoint(checkpoint); err != nil {
		return Checkpoint{}, err
	}
	return checkpoint, nil
}

// Save persists consumer progress atomically: temp-file fsync, rename, then directory fsync.
func (s *CheckpointStore) Save(journal *Journal, record JournalRecord) (Checkpoint, error) {
	checkpoint, err := journal.checkpointForRecord(record)
	if err != nil {
		return Checkpoint{}, err
	}
	wire := checkpointWire{
		SchemaVersion: checkpointSchemaVersion,
		JournalID:     checkpoint.JournalID,
		RecordID:      checkpoint.RecordID,
		Offset:        checkpoint.Offset,
	}
	data, err := json.Marshal(wire)
	if err != nil {
		return Checkpoint{}, err
	}
	data = append(data, '\n')

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(s.path)+".tmp-")
	if err != nil {
		return Checkpoint{}, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return Checkpoint{}, err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return Checkpoint{}, err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return Checkpoint{}, err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		_ = os.Remove(tmpName)
		return Checkpoint{}, err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return Checkpoint{}, err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return Checkpoint{}, syncErr
	}
	if closeErr != nil {
		return Checkpoint{}, closeErr
	}
	return checkpoint, nil
}

func (j *Journal) ValidateCheckpoint(checkpoint Checkpoint) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.validateCheckpointLocked(checkpoint)
}

func (j *Journal) checkpointForRecord(record JournalRecord) (Checkpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return Checkpoint{}, ErrJournalClosed
	}
	if record.RecordID == 0 || record.RecordID > j.lastRecordID {
		return Checkpoint{}, ErrCheckpointAhead
	}
	expectedEnd := j.recordEnds[record.RecordID-1]
	if record.EndOffset != expectedEnd {
		return Checkpoint{}, ErrCheckpointOffsetMismatch
	}
	return Checkpoint{JournalID: j.journalIDHex, RecordID: record.RecordID, Offset: expectedEnd}, nil
}

func (j *Journal) validateCheckpointLocked(checkpoint Checkpoint) error {
	if checkpoint.JournalID != j.journalIDHex {
		return ErrCheckpointJournalMismatch
	}
	if checkpoint.RecordID > j.lastRecordID {
		return ErrCheckpointAhead
	}
	expectedOffset := journalHeaderSize()
	if checkpoint.RecordID > 0 {
		expectedOffset = j.recordEnds[checkpoint.RecordID-1]
	}
	if checkpoint.Offset != expectedOffset {
		return fmt.Errorf("%w: got=%d want=%d", ErrCheckpointOffsetMismatch, checkpoint.Offset, expectedOffset)
	}
	return nil
}

func (j *Journal) openOrInitialize() error {
	stat, err := j.file.Stat()
	if err != nil {
		return err
	}
	if stat.Size() == 0 {
		if _, err := rand.Read(j.journalID[:]); err != nil {
			return err
		}
		header := make([]byte, 0, journalHeaderSize())
		header = append(header, journalMagic...)
		header = append(header, j.journalID[:]...)
		if _, err := j.file.WriteAt(header, 0); err != nil {
			return err
		}
		if err := j.file.Sync(); err != nil {
			return err
		}
		j.journalIDHex = hex.EncodeToString(j.journalID[:])
		j.endOffset = journalHeaderSize()
		return nil
	}
	if stat.Size() < journalHeaderSize() {
		return fmt.Errorf("%w: incomplete header", ErrCorruptJournal)
	}
	header := make([]byte, journalHeaderSize())
	if _, err := j.file.ReadAt(header, 0); err != nil {
		return err
	}
	if !bytes.Equal(header[:len(journalMagic)], journalMagic) {
		return fmt.Errorf("%w: bad magic", ErrCorruptJournal)
	}
	copy(j.journalID[:], header[len(journalMagic):])
	allZero := true
	for _, value := range j.journalID {
		if value != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return fmt.Errorf("%w: empty journal id", ErrCorruptJournal)
	}
	j.journalIDHex = hex.EncodeToString(j.journalID[:])
	return j.scanExisting(stat.Size())
}

func (j *Journal) scanExisting(size int64) error {
	offset := journalHeaderSize()
	expected := uint64(1)
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
		if length == 0 || length > maxJournalPayload {
			return fmt.Errorf("%w: invalid payload length=%d at offset=%d", ErrCorruptJournal, length, offset)
		}
		frameEnd := offset + 4 + int64(length) + 4
		if frameEnd > size {
			return j.repairTornTail(offset)
		}
		record, next, err := j.readRecordAtLocked(offset)
		if err != nil {
			return err
		}
		if next != frameEnd || record.RecordID != expected {
			return fmt.Errorf("%w: record id got=%d want=%d", ErrCorruptJournal, record.RecordID, expected)
		}
		j.recordEnds = append(j.recordEnds, next)
		j.lastRecordID = record.RecordID
		j.endOffset = next
		offset = next
		expected++
	}
	if j.endOffset == 0 {
		j.endOffset = journalHeaderSize()
	}
	return nil
}

func (j *Journal) repairTornTail(offset int64) error {
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

func (j *Journal) readRecordAtLocked(offset int64) (JournalRecord, int64, error) {
	var lengthBytes [4]byte
	if _, err := j.file.ReadAt(lengthBytes[:], offset); err != nil {
		return JournalRecord{}, 0, err
	}
	length := binary.BigEndian.Uint32(lengthBytes[:])
	if length == 0 || length > maxJournalPayload {
		return JournalRecord{}, 0, fmt.Errorf("%w: invalid payload length=%d", ErrCorruptJournal, length)
	}
	payload := make([]byte, int(length))
	if _, err := j.file.ReadAt(payload, offset+4); err != nil {
		return JournalRecord{}, 0, err
	}
	var crcBytes [4]byte
	if _, err := j.file.ReadAt(crcBytes[:], offset+4+int64(length)); err != nil {
		return JournalRecord{}, 0, err
	}
	wantCRC := binary.BigEndian.Uint32(crcBytes[:])
	gotCRC := crc32.Checksum(payload, crcTable)
	if gotCRC != wantCRC {
		return JournalRecord{}, 0, fmt.Errorf("%w: crc mismatch at offset=%d", ErrCorruptJournal, offset)
	}
	recordID, event, err := decodeJournalRecord(payload)
	if err != nil {
		return JournalRecord{}, 0, err
	}
	next := offset + 4 + int64(length) + 4
	return JournalRecord{RecordID: recordID, Event: event, EndOffset: next}, next, nil
}

func encodeJournalRecord(recordID uint64, event Event) ([]byte, error) {
	wire := journalWireRecord{
		SchemaVersion: JournalSchemaVersion,
		RecordID:      recordID,
		Event:         eventToWire(event),
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || len(payload) > maxJournalPayload {
		return nil, fmt.Errorf("%w: encoded payload size=%d", ErrInvalidEvent, len(payload))
	}
	return payload, nil
}

func decodeJournalRecord(payload []byte) (uint64, Event, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var wire journalWireRecord
	if err := decoder.Decode(&wire); err != nil {
		return 0, Event{}, fmt.Errorf("%w: decode record: %v", ErrCorruptJournal, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return 0, Event{}, fmt.Errorf("%w: trailing record data", ErrCorruptJournal)
	}
	if wire.SchemaVersion != JournalSchemaVersion || wire.RecordID == 0 {
		return 0, Event{}, fmt.Errorf("%w: invalid record header", ErrCorruptJournal)
	}
	event := wireToEvent(wire.Event)
	if err := validateJournalEvent(event); err != nil {
		return 0, Event{}, fmt.Errorf("%w: event: %v", ErrCorruptJournal, err)
	}
	return wire.RecordID, event, nil
}

func makeFrame(payload []byte) []byte {
	frame := make([]byte, 4+len(payload)+4)
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)
	binary.BigEndian.PutUint32(frame[4+len(payload):], crc32.Checksum(payload, crcTable))
	return frame
}

func validateJournalEvent(event Event) error {
	if event.EventID == 0 || event.EntityID == 0 || event.DefeatRevision == 0 || !validContext(event.Context) {
		return ErrInvalidEvent
	}
	if event.CheckpointForfeited && !event.PenaltyTransactionApplied {
		return fmt.Errorf("%w: checkpoint forfeiture requires applied penalty transaction", ErrInvalidEvent)
	}
	if event.Respawn.Scheduled {
		if event.Respawn.SpawnPointID == "" || !validSpawnClass(event.Respawn.SpawnClass) || event.Respawn.DueTick <= event.DefeatedTick {
			return fmt.Errorf("%w: invalid respawn binding", ErrInvalidEvent)
		}
	} else if event.Respawn.SpawnPointID != "" || event.Respawn.SpawnClass != "" || event.Respawn.Position != (world.Position{}) || event.Respawn.DueTick != 0 {
		return fmt.Errorf("%w: unscheduled respawn must not carry binding fields", ErrInvalidEvent)
	}
	return nil
}

func eventToWire(event Event) journalWireEvent {
	return journalWireEvent{
		EventID:                    event.EventID,
		EntityID:                   event.EntityID,
		DefeatRevision:             event.DefeatRevision,
		Context:                    string(event.Context),
		DefeatedTick:               event.DefeatedTick,
		RespawnPolicyRevision:      event.RespawnPolicyRevision,
		DeathPenaltyPolicyRevision: event.DeathPenaltyPolicyRevision,
		Respawn: journalWireRespawn{
			Scheduled:    event.Respawn.Scheduled,
			SpawnPointID: event.Respawn.SpawnPointID,
			SpawnClass:   string(event.Respawn.SpawnClass),
			X:            event.Respawn.Position.X,
			Y:            event.Respawn.Position.Y,
			Z:            event.Respawn.Position.Z,
			Layer:        event.Respawn.Position.Layer,
			DueTick:      event.Respawn.DueTick,
		},
		PenaltyTransactionApplied: event.PenaltyTransactionApplied,
		CheckpointForfeited:       event.CheckpointForfeited,
	}
}

func wireToEvent(wire journalWireEvent) Event {
	return Event{
		EventID:                    wire.EventID,
		EntityID:                   wire.EntityID,
		DefeatRevision:             wire.DefeatRevision,
		Context:                    deathContextFromString(wire.Context),
		DefeatedTick:               wire.DefeatedTick,
		RespawnPolicyRevision:      wire.RespawnPolicyRevision,
		DeathPenaltyPolicyRevision: wire.DeathPenaltyPolicyRevision,
		Respawn: RespawnBinding{
			Scheduled:    wire.Respawn.Scheduled,
			SpawnPointID: wire.Respawn.SpawnPointID,
			SpawnClass:   spawnClassFromString(wire.Respawn.SpawnClass),
			Position: world.Position{
				X:     wire.Respawn.X,
				Y:     wire.Respawn.Y,
				Z:     wire.Respawn.Z,
				Layer: wire.Respawn.Layer,
			},
			DueTick: wire.Respawn.DueTick,
		},
		PenaltyTransactionApplied: wire.PenaltyTransactionApplied,
		CheckpointForfeited:       wire.CheckpointForfeited,
	}
}

func deathContextFromString(value string) respawnpolicy.DeathContext {
	return respawnpolicy.DeathContext(value)
}

func spawnClassFromString(value string) respawnpolicy.SpawnClass {
	return respawnpolicy.SpawnClass(value)
}

func journalHeaderSize() int64 {
	return int64(len(journalMagic) + journalIDSize)
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o750)
}
