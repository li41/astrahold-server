package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	sessionLeafRevocationDistributionSchemaVersion      uint16 = 1
	sessionLeafRevocationDistributionAckSchemaVersion   uint16 = 1
	sessionLeafRevocationDistributionMaxDefinitionBytes        = 16 * 1024
	sessionLeafRevocationDistributionMaxAckBytes               = 16 * 1024
	sessionLeafRevocationDistributionMaxInstanceIDBytes        = 64
	sessionLeafRevocationDistributionMaxLease                  = 24 * time.Hour
)

var (
	sessionLoginTrustedProxyLeafRevocationDistributionFile = flag.String(
		"session-login-trusted-proxy-leaf-revocation-distribution-file",
		"",
		"Optional F.25 revocation distribution lease manifest; requires the F.24 revocation file plus instance-id and ack-file and bounds stale multi-instance authority",
	)
	sessionLoginTrustedProxyLeafRevocationInstanceID = flag.String(
		"session-login-trusted-proxy-leaf-revocation-instance-id",
		"",
		"Stable F.25 instance identifier written to the local revocation acknowledgement file",
	)
	sessionLoginTrustedProxyLeafRevocationAckFile = flag.String(
		"session-login-trusted-proxy-leaf-revocation-ack-file",
		"",
		"Local durable F.25 acknowledgement file used as the per-instance epoch floor and convergence evidence",
	)
)

var (
	errSessionLeafRevocationDistributionConfig      = errors.New("worldd: invalid trusted proxy leaf revocation distribution config")
	errSessionLeafRevocationDistributionUnavailable = errors.New("worldd: trusted proxy leaf revocation distribution authority unavailable")
)

type sessionLeafRevocationDistributionDefinition struct {
	SchemaVersion             uint16 `json:"schema_version"`
	Epoch                     uint64 `json:"epoch"`
	RevocationAuthoritySHA256 string `json:"revocation_authority_sha256"`
	ValidUntil                string `json:"valid_until"`
}

type sessionLeafRevocationDistributionSnapshot struct {
	epoch           uint64
	authorityDigest [sha256.Size]byte
	validUntil      time.Time
}

type sessionLeafRevocationDistributionAck struct {
	SchemaVersion             uint16 `json:"schema_version"`
	InstanceID                string `json:"instance_id"`
	Epoch                     uint64 `json:"epoch"`
	RevocationRevision        string `json:"revocation_revision"`
	RevocationAuthoritySHA256 string `json:"revocation_authority_sha256"`
	ValidUntil                string `json:"valid_until"`
	AcknowledgedAt            string `json:"acknowledged_at"`
}

type sessionLeafRevocationDistributionMetadata struct {
	Enabled         bool
	InstanceID      string
	Epoch           uint64
	ValidUntil      time.Time
	AckHealthy      bool
	LeaseValid      bool
	AuthoritySHA256 string
}

type sessionLeafRevocationDistributionRuntime struct {
	mu             sync.RWMutex
	definitionFile string
	instanceID     string
	ackFile        string
	current        *sessionLeafRevocationDistributionSnapshot
	ackHealthy     bool
	now            func() time.Time
}

func sessionLeafRevocationDistributionRequested() bool {
	return strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationDistributionFile) != "" ||
		strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationInstanceID) != "" ||
		strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationAckFile) != ""
}

func validateSessionLeafRevocationDistributionFlags(edgePolicyFile, revocationFile string) error {
	if !sessionLeafRevocationDistributionRequested() {
		return nil
	}
	if strings.TrimSpace(edgePolicyFile) == "" || strings.TrimSpace(revocationFile) == "" {
		return fmt.Errorf("%w: F.25 distribution requires session-login-trusted-proxy-edge-policy-file and session-login-trusted-proxy-leaf-revocation-file", errSessionLeafRevocationDistributionConfig)
	}
	distributionFile := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationDistributionFile)
	instanceID := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationInstanceID)
	ackFile := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationAckFile)
	if distributionFile == "" || instanceID == "" || ackFile == "" {
		return fmt.Errorf("%w: distribution-file, instance-id, and ack-file must be set together", errSessionLeafRevocationDistributionConfig)
	}
	if err := validateSessionLeafRevocationDistributionInstanceID(instanceID); err != nil {
		return err
	}
	if sameCleanPath(distributionFile, revocationFile) || sameCleanPath(ackFile, revocationFile) || sameCleanPath(ackFile, distributionFile) {
		return fmt.Errorf("%w: revocation, distribution, and ack files must be distinct", errSessionLeafRevocationDistributionConfig)
	}
	return nil
}

func sameCleanPath(a, b string) bool {
	if strings.TrimSpace(a) == "" || strings.TrimSpace(b) == "" {
		return false
	}
	aa, errA := filepath.Abs(filepath.Clean(a))
	bb, errB := filepath.Abs(filepath.Clean(b))
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return aa == bb
}

func validateSessionLeafRevocationDistributionInstanceID(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > sessionLeafRevocationDistributionMaxInstanceIDBytes {
		return fmt.Errorf("%w: instance-id must be 1..%d trimmed bytes", errSessionLeafRevocationDistributionConfig, sessionLeafRevocationDistributionMaxInstanceIDBytes)
	}
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' {
			continue
		}
		return fmt.Errorf("%w: instance-id contains unsupported characters", errSessionLeafRevocationDistributionConfig)
	}
	return nil
}

func newSessionLeafRevocationDistributionRuntime(owner *reloadableSessionLeafRevocation, revocation *sessionLeafRevocationSnapshot, now func() time.Time) (*sessionLeafRevocationDistributionRuntime, error) {
	if !sessionLeafRevocationDistributionRequested() {
		return nil, nil
	}
	if owner == nil || revocation == nil {
		return nil, errSessionLeafRevocationDistributionConfig
	}
	if now == nil {
		now = time.Now
	}
	distributionFile := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationDistributionFile)
	instanceID := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationInstanceID)
	ackFile := strings.TrimSpace(*sessionLoginTrustedProxyLeafRevocationAckFile)
	if distributionFile == "" || instanceID == "" || ackFile == "" {
		return nil, fmt.Errorf("%w: distribution-file, instance-id, and ack-file must be set together", errSessionLeafRevocationDistributionConfig)
	}
	if err := validateSessionLeafRevocationDistributionInstanceID(instanceID); err != nil {
		return nil, err
	}
	if sameCleanPath(distributionFile, owner.definitionFile) || sameCleanPath(ackFile, owner.definitionFile) || sameCleanPath(ackFile, distributionFile) {
		return nil, fmt.Errorf("%w: revocation, distribution, and ack files must be distinct", errSessionLeafRevocationDistributionConfig)
	}
	current, err := loadSessionLeafRevocationDistributionSnapshot(distributionFile, revocation.authorityDigest, now().UTC())
	if err != nil {
		return nil, err
	}
	runtime := &sessionLeafRevocationDistributionRuntime{
		definitionFile: distributionFile,
		instanceID:     instanceID,
		ackFile:        ackFile,
		current:        current,
		now:            now,
	}
	if floor, err := loadSessionLeafRevocationDistributionAck(ackFile); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if err := runtime.validateAckFloor(floor, current); err != nil {
		return nil, err
	}
	if err := runtime.writeAck(revocation.revision, current, now().UTC()); err != nil {
		return nil, err
	}
	runtime.ackHealthy = true
	return runtime, nil
}

func loadSessionLeafRevocationDistributionSnapshot(path string, expected [sha256.Size]byte, now time.Time) (*sessionLeafRevocationDistributionSnapshot, error) {
	data, err := readSessionProxyMTLSFile(path, sessionLeafRevocationDistributionMaxDefinitionBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: distribution file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var definition sessionLeafRevocationDistributionDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode distribution file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionLeafRevocationDistributionConfig)
		}
		return nil, fmt.Errorf("%w: trailing distribution data: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if definition.SchemaVersion != sessionLeafRevocationDistributionSchemaVersion {
		return nil, fmt.Errorf("%w: schema_version must be %d", errSessionLeafRevocationDistributionConfig, sessionLeafRevocationDistributionSchemaVersion)
	}
	if definition.Epoch == 0 {
		return nil, fmt.Errorf("%w: epoch must be greater than zero", errSessionLeafRevocationDistributionConfig)
	}
	digest, err := parseSessionLeafRevocationDistributionDigest(definition.RevocationAuthoritySHA256)
	if err != nil {
		return nil, err
	}
	if digest != expected {
		return nil, fmt.Errorf("%w: revocation_authority_sha256 does not match the F.24 revocation authority", errSessionLeafRevocationDistributionConfig)
	}
	validUntil, err := parseSessionLeafRevocationDistributionTime(definition.ValidUntil, "valid_until")
	if err != nil {
		return nil, err
	}
	now = now.UTC()
	if !now.Before(validUntil) {
		return nil, fmt.Errorf("%w: distribution lease is already expired", errSessionLeafRevocationDistributionConfig)
	}
	if validUntil.After(now.Add(sessionLeafRevocationDistributionMaxLease)) {
		return nil, fmt.Errorf("%w: distribution lease exceeds %s", errSessionLeafRevocationDistributionConfig, sessionLeafRevocationDistributionMaxLease)
	}
	return &sessionLeafRevocationDistributionSnapshot{epoch: definition.Epoch, authorityDigest: digest, validUntil: validUntil}, nil
}

func parseSessionLeafRevocationDistributionDigest(value string) ([sha256.Size]byte, error) {
	if len(value) != sha256.Size*2 || strings.TrimSpace(value) != value || strings.ToLower(value) != value {
		return [sha256.Size]byte{}, fmt.Errorf("%w: revocation_authority_sha256 must be exactly 64 lowercase hex characters", errSessionLeafRevocationDistributionConfig)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, fmt.Errorf("%w: revocation_authority_sha256 is invalid", errSessionLeafRevocationDistributionConfig)
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return digest, nil
}

func parseSessionLeafRevocationDistributionTime(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || value != parsed.UTC().Format(time.RFC3339) {
		return time.Time{}, fmt.Errorf("%w: %s must be canonical UTC RFC3339 seconds", errSessionLeafRevocationDistributionConfig, field)
	}
	return parsed.UTC(), nil
}

func loadSessionLeafRevocationDistributionAck(path string) (sessionLeafRevocationDistributionAck, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionLeafRevocationDistributionAck{}, err
	}
	if len(data) == 0 || len(data) > sessionLeafRevocationDistributionMaxAckBytes {
		return sessionLeafRevocationDistributionAck{}, fmt.Errorf("%w: ack file is empty or too large", errSessionLeafRevocationDistributionConfig)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var ack sessionLeafRevocationDistributionAck
	if err := decoder.Decode(&ack); err != nil {
		return sessionLeafRevocationDistributionAck{}, fmt.Errorf("%w: decode ack file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return sessionLeafRevocationDistributionAck{}, fmt.Errorf("%w: trailing ack data", errSessionLeafRevocationDistributionConfig)
	}
	if ack.SchemaVersion != sessionLeafRevocationDistributionAckSchemaVersion || ack.Epoch == 0 {
		return sessionLeafRevocationDistributionAck{}, fmt.Errorf("%w: invalid ack schema or epoch", errSessionLeafRevocationDistributionConfig)
	}
	if err := validateSessionLeafRevocationDistributionInstanceID(ack.InstanceID); err != nil {
		return sessionLeafRevocationDistributionAck{}, err
	}
	if strings.TrimSpace(ack.RevocationRevision) == "" || ack.RevocationRevision != strings.TrimSpace(ack.RevocationRevision) || len(ack.RevocationRevision) > sessionProxyMTLSMaxRevisionBytes {
		return sessionLeafRevocationDistributionAck{}, fmt.Errorf("%w: invalid ack revocation_revision", errSessionLeafRevocationDistributionConfig)
	}
	if _, err := parseSessionLeafRevocationDistributionDigest(ack.RevocationAuthoritySHA256); err != nil {
		return sessionLeafRevocationDistributionAck{}, err
	}
	if _, err := parseSessionLeafRevocationDistributionTime(ack.ValidUntil, "ack valid_until"); err != nil {
		return sessionLeafRevocationDistributionAck{}, err
	}
	if _, err := parseSessionLeafRevocationDistributionTime(ack.AcknowledgedAt, "ack acknowledged_at"); err != nil {
		return sessionLeafRevocationDistributionAck{}, err
	}
	return ack, nil
}

func (r *sessionLeafRevocationDistributionRuntime) validateAckFloor(ack sessionLeafRevocationDistributionAck, candidate *sessionLeafRevocationDistributionSnapshot) error {
	if r == nil || candidate == nil {
		return errSessionLeafRevocationDistributionConfig
	}
	if ack.InstanceID != r.instanceID {
		return fmt.Errorf("%w: ack instance_id does not match configured instance-id", errSessionLeafRevocationDistributionConfig)
	}
	digest, err := parseSessionLeafRevocationDistributionDigest(ack.RevocationAuthoritySHA256)
	if err != nil {
		return err
	}
	validUntil, err := parseSessionLeafRevocationDistributionTime(ack.ValidUntil, "ack valid_until")
	if err != nil {
		return err
	}
	if ack.Epoch > candidate.epoch {
		return fmt.Errorf("%w: distribution epoch rollback below durable ack floor", errSessionLeafRevocationDistributionConfig)
	}
	if ack.Epoch == candidate.epoch && (digest != candidate.authorityDigest || !validUntil.Equal(candidate.validUntil)) {
		return fmt.Errorf("%w: distribution epoch reuse conflicts with durable ack floor", errSessionLeafRevocationDistributionConfig)
	}
	return nil
}

func (r *sessionLeafRevocationDistributionRuntime) loadCandidate(revocation *sessionLeafRevocationSnapshot, now time.Time) (*sessionLeafRevocationDistributionSnapshot, bool, error) {
	if r == nil || revocation == nil {
		return nil, false, errSessionLeafRevocationDistributionConfig
	}
	candidate, err := loadSessionLeafRevocationDistributionSnapshot(r.definitionFile, revocation.authorityDigest, now.UTC())
	if err != nil {
		return nil, false, err
	}
	r.mu.RLock()
	current := r.current
	r.mu.RUnlock()
	if current == nil {
		return nil, false, errSessionLeafRevocationDistributionConfig
	}
	if candidate.epoch < current.epoch {
		return nil, false, fmt.Errorf("%w: distribution epoch rollback", errSessionLeafRevocationDistributionConfig)
	}
	if candidate.epoch == current.epoch {
		if candidate.authorityDigest != current.authorityDigest || !candidate.validUntil.Equal(current.validUntil) {
			return nil, false, fmt.Errorf("%w: distribution epoch reuse with different authority or lease", errSessionLeafRevocationDistributionConfig)
		}
		return candidate, false, nil
	}
	return candidate, true, nil
}

func (r *reloadableSessionLeafRevocation) reloadDistributed(candidate *sessionLeafRevocationSnapshot) (sessionLeafRevocationReloadResult, error) {
	if r == nil || r.distribution == nil || candidate == nil {
		return sessionLeafRevocationReloadResult{}, errSessionLeafRevocationDistributionConfig
	}
	now := r.now().UTC()
	distributionCandidate, distributionChanged, err := r.distribution.loadCandidate(candidate, now)
	if err != nil {
		return sessionLeafRevocationReloadResult{}, err
	}
	result, err := r.publishCandidate(candidate, distributionChanged)
	if err != nil {
		return sessionLeafRevocationReloadResult{}, err
	}

	r.distribution.mu.Lock()
	r.distribution.current = distributionCandidate
	r.distribution.ackHealthy = false
	r.distribution.mu.Unlock()

	ackErr := r.distribution.writeAck(result.Revision, distributionCandidate, now)
	ackHealthy := ackErr == nil
	r.distribution.mu.Lock()
	r.distribution.ackHealthy = ackHealthy
	r.distribution.mu.Unlock()

	result.DistributionEnabled = true
	result.DistributionChanged = distributionChanged
	result.DistributionEpoch = distributionCandidate.epoch
	result.DistributionValidUntil = distributionCandidate.validUntil
	result.DistributionAckHealthy = ackHealthy
	return result, nil
}

func (r *sessionLeafRevocationDistributionRuntime) writeAck(revocationRevision string, snapshot *sessionLeafRevocationDistributionSnapshot, acknowledgedAt time.Time) error {
	if r == nil || snapshot == nil || strings.TrimSpace(r.ackFile) == "" {
		return errSessionLeafRevocationDistributionConfig
	}
	acknowledgedAt = acknowledgedAt.UTC().Truncate(time.Second)
	ack := sessionLeafRevocationDistributionAck{
		SchemaVersion:             sessionLeafRevocationDistributionAckSchemaVersion,
		InstanceID:                r.instanceID,
		Epoch:                     snapshot.epoch,
		RevocationRevision:        revocationRevision,
		RevocationAuthoritySHA256: hex.EncodeToString(snapshot.authorityDigest[:]),
		ValidUntil:                snapshot.validUntil.UTC().Format(time.RFC3339),
		AcknowledgedAt:            acknowledgedAt.Format(time.RFC3339),
	}
	data, err := json.Marshal(ack)
	if err != nil {
		return fmt.Errorf("%w: encode ack: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	data = append(data, '\n')
	if len(data) > sessionLeafRevocationDistributionMaxAckBytes {
		return fmt.Errorf("%w: encoded ack exceeds bound", errSessionLeafRevocationDistributionConfig)
	}
	dir := filepath.Dir(r.ackFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("%w: create ack directory: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	temp, err := os.CreateTemp(dir, ".leaf-revocation-ack-*")
	if err != nil {
		return fmt.Errorf("%w: create ack temp file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	tempName := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempName)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("%w: chmod ack temp file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("%w: write ack temp file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("%w: fsync ack temp file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("%w: close ack temp file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if err := os.Rename(tempName, r.ackFile); err != nil {
		_ = os.Remove(tempName)
		return fmt.Errorf("%w: publish ack file: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	if directory, err := os.Open(dir); err == nil {
		syncErr := directory.Sync()
		closeErr := directory.Close()
		if syncErr != nil {
			return fmt.Errorf("%w: fsync ack directory: %v", errSessionLeafRevocationDistributionConfig, syncErr)
		}
		if closeErr != nil {
			return fmt.Errorf("%w: close ack directory: %v", errSessionLeafRevocationDistributionConfig, closeErr)
		}
	} else {
		return fmt.Errorf("%w: open ack directory: %v", errSessionLeafRevocationDistributionConfig, err)
	}
	return nil
}

func (r *sessionLeafRevocationDistributionRuntime) authorityAvailable(authorityDigest [sha256.Size]byte, now time.Time) bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current == nil || !r.ackHealthy || r.current.authorityDigest != authorityDigest {
		return false
	}
	return now.UTC().Before(r.current.validUntil)
}

func (r *sessionLeafRevocationDistributionRuntime) metadata(now time.Time) sessionLeafRevocationDistributionMetadata {
	if r == nil {
		return sessionLeafRevocationDistributionMetadata{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current == nil {
		return sessionLeafRevocationDistributionMetadata{Enabled: true, InstanceID: r.instanceID}
	}
	return sessionLeafRevocationDistributionMetadata{
		Enabled:         true,
		InstanceID:      r.instanceID,
		Epoch:           r.current.epoch,
		ValidUntil:      r.current.validUntil,
		AckHealthy:      r.ackHealthy,
		LeaseValid:      now.UTC().Before(r.current.validUntil),
		AuthoritySHA256: hex.EncodeToString(r.current.authorityDigest[:]),
	}
}

func sessionLeafRevocationDistributionMetadataForAttributor(a *sessionSourceAttributor) sessionLeafRevocationDistributionMetadata {
	if a == nil || a.edgePolicy == nil || a.edgePolicy.leafRevocation == nil || a.edgePolicy.leafRevocation.distribution == nil {
		return sessionLeafRevocationDistributionMetadata{}
	}
	runtime := a.edgePolicy.leafRevocation
	return runtime.distribution.metadata(runtime.now().UTC())
}
