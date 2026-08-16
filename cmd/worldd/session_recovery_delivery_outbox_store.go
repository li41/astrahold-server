package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func (o *sessionRecoveryDurableOutbox) loadAndRestore(provider *staticSessionRecoveryProvider) error {
	entries, err := os.ReadDir(o.dir)
	if err != nil {
		return fmt.Errorf("%w: read recovery outbox directory: %v", errSessionLoginConfig, err)
	}
	now := o.now().UTC()
	loaded := make([]sessionRecoveryOutboxRecord, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") && strings.HasSuffix(name, ".tmp") {
			path := filepath.Join(o.dir, name)
			info, statErr := os.Lstat(path)
			if statErr != nil {
				return fmt.Errorf("%w: stat recovery outbox temp: %v", errSessionLoginConfig, statErr)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
				return fmt.Errorf("%w: unsafe recovery outbox temp file", errSessionLoginConfig)
			}
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("%w: remove recovery outbox temp: %v", errSessionLoginConfig, err)
			}
			continue
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".json") {
			return fmt.Errorf("%w: unexpected recovery outbox entry %q", errSessionLoginConfig, name)
		}
		path := filepath.Join(o.dir, name)
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("%w: stat recovery outbox record: %v", errSessionLoginConfig, err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 || info.Size() <= 0 || info.Size() > sessionRecoveryOutboxMaxRecordBytes {
			return fmt.Errorf("%w: recovery outbox record %q must be a 0600 regular bounded file", errSessionLoginConfig, name)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: read recovery outbox record: %v", errSessionLoginConfig, err)
		}
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		var record sessionRecoveryOutboxRecord
		if err := decoder.Decode(&record); err != nil {
			clear(data)
			return fmt.Errorf("%w: decode recovery outbox record %q: %v", errSessionLoginConfig, name, err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			clear(data)
			return fmt.Errorf("%w: recovery outbox record %q has trailing data", errSessionLoginConfig, name)
		}
		clear(data)
		if err := o.validateRecord(record); err != nil {
			return err
		}
		if name != record.DeliveryID+".json" {
			return fmt.Errorf("%w: recovery outbox filename does not match delivery_id", errSessionLoginConfig)
		}
		expiresAt, _ := parseSessionRecoveryOutboxTime(record.ExpiresAt)
		if !now.Before(expiresAt) {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("%w: remove expired recovery outbox record: %v", errSessionLoginConfig, err)
			}
			continue
		}
		loaded = append(loaded, record)
	}
	if len(loaded) > o.maxRecords || len(loaded) > provider.maxActive {
		return fmt.Errorf("%w: recovery outbox contains %d live records beyond configured bounds", errSessionLoginConfig, len(loaded))
	}
	sort.Slice(loaded, func(i, j int) bool { return loaded[i].RequestID < loaded[j].RequestID })
	for _, record := range loaded {
		if err := provider.restoreSessionRecoveryOutboxRecord(record); err != nil {
			return err
		}
		o.records[record.RequestID] = record
		o.owners[record.RequestID] = provider
		o.ready[record.RequestID] = record.DeliveryState == sessionRecoveryOutboxStatePending
	}
	if len(loaded) > 0 {
		o.logf("recovery outbox: outcome=restored records=%d pending=%d", len(loaded), o.pendingCountLocked())
	}
	return syncSessionRecoveryOutboxDir(o.dir)
}

func (o *sessionRecoveryDurableOutbox) validateRecord(record sessionRecoveryOutboxRecord) error {
	if record.SchemaVersion != sessionRecoveryOutboxRecordSchemaVersion ||
		!validSessionRecoveryOutboxToken(record.RequestID, accountrecovery.MaxRequestIDBytes) ||
		!validSessionRecoveryOutboxToken(record.LoginID, accountrecovery.MaxLoginIDBytes) ||
		!validSessionRecoveryOutboxToken(record.AccountID, accountrecovery.MaxAccountIDBytes) ||
		record.CredentialVersion == 0 ||
		len(record.VerifierSHA256) != sha256.Size*2 || strings.ToLower(record.VerifierSHA256) != record.VerifierSHA256 {
		return fmt.Errorf("%w: invalid recovery outbox record identity", errSessionLoginConfig)
	}
	if _, err := hex.DecodeString(record.VerifierSHA256); err != nil {
		return fmt.Errorf("%w: invalid recovery outbox verifier digest", errSessionLoginConfig)
	}
	if record.DeliveryID == "" || record.DeliveryID != recoveryDeliveryID(record.RequestID) {
		return fmt.Errorf("%w: invalid recovery outbox delivery_id", errSessionLoginConfig)
	}
	expiresAt, err := parseSessionRecoveryOutboxTime(record.ExpiresAt)
	if err != nil || expiresAt.IsZero() {
		return fmt.Errorf("%w: invalid recovery outbox expires_at", errSessionLoginConfig)
	}
	if record.VerificationAttempts < 0 || record.VerificationAttempts > 20 || record.DeliveryAttempts < 0 || record.DeliveryAttempts > sessionRecoveryOutboxMaxDeliveryAttempts {
		return fmt.Errorf("%w: invalid recovery outbox attempt counters", errSessionLoginConfig)
	}
	switch record.DeliveryState {
	case sessionRecoveryOutboxStatePending:
		if !record.Active || !validSessionRecoveryOutboxToken(record.Destination, accountrecovery.MaxDeliveryDestinationBytes) || record.Proof == "" || len(record.Proof) > accountrecovery.MaxProofBytes {
			return fmt.Errorf("%w: invalid pending recovery outbox record", errSessionLoginConfig)
		}
		if _, err := parseSessionRecoveryOutboxTime(record.NextAttemptAt); err != nil {
			return fmt.Errorf("%w: invalid recovery outbox next_attempt_at", errSessionLoginConfig)
		}
		digest := sha256.Sum256([]byte(record.Proof))
		if hex.EncodeToString(digest[:]) != record.VerifierSHA256 {
			return fmt.Errorf("%w: recovery outbox proof/verifier mismatch", errSessionLoginConfig)
		}
	case sessionRecoveryOutboxStateDelivered:
		if !record.Active || record.Destination != "" || record.Proof != "" || record.NextAttemptAt != "" {
			return fmt.Errorf("%w: invalid delivered recovery outbox record", errSessionLoginConfig)
		}
	case sessionRecoveryOutboxStateFailed:
		if record.Active || record.Destination != "" || record.Proof != "" || record.NextAttemptAt != "" {
			return fmt.Errorf("%w: invalid failed recovery outbox record", errSessionLoginConfig)
		}
	default:
		return fmt.Errorf("%w: invalid recovery outbox delivery_state", errSessionLoginConfig)
	}
	return nil
}

func validSessionRecoveryOutboxToken(value string, max int) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= max
}

func parseSessionRecoveryOutboxTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, errors.New("empty time")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func sameSessionRecoveryOutboxDelivery(a, b sessionRecoveryOutboxRecord) bool {
	return a.RequestID == b.RequestID &&
		a.DeliveryID == b.DeliveryID &&
		a.LoginID == b.LoginID &&
		a.AccountID == b.AccountID &&
		a.CredentialVersion == b.CredentialVersion &&
		a.VerifierSHA256 == b.VerifierSHA256 &&
		a.ExpiresAt == b.ExpiresAt &&
		a.Destination == b.Destination &&
		a.Proof == b.Proof
}

func (o *sessionRecoveryDurableOutbox) writeRecordLocked(record sessionRecoveryOutboxRecord) error {
	if err := o.validateRecord(record); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	defer clear(data)
	tempPath := filepath.Join(o.dir, "."+record.DeliveryID+".tmp")
	finalPath := filepath.Join(o.dir, record.DeliveryID+".json")
	_ = os.Remove(tempPath)
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = os.Remove(tempPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return err
	}
	removeTemp = false
	if err := os.Chmod(finalPath, 0o600); err != nil {
		return err
	}
	return syncSessionRecoveryOutboxDir(o.dir)
}

func (o *sessionRecoveryDurableOutbox) deleteRecordLocked(requestID string) error {
	record, exists := o.records[requestID]
	if !exists {
		delete(o.owners, requestID)
		delete(o.ready, requestID)
		return nil
	}
	path := filepath.Join(o.dir, record.DeliveryID+".json")
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(o.records, requestID)
	delete(o.owners, requestID)
	delete(o.ready, requestID)
	return syncSessionRecoveryOutboxDir(o.dir)
}

func syncSessionRecoveryOutboxDir(dir string) error {
	file, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}

func (o *sessionRecoveryDurableOutbox) pruneExpiredLocked(now time.Time) {
	type expiredOwner struct {
		requestID string
		owner     *staticSessionRecoveryProvider
	}
	expired := make([]expiredOwner, 0)
	for requestID, record := range o.records {
		expiresAt, err := parseSessionRecoveryOutboxTime(record.ExpiresAt)
		if err == nil && now.Before(expiresAt) {
			continue
		}
		owner := o.owners[requestID]
		if o.deleteRecordLocked(requestID) == nil {
			expired = append(expired, expiredOwner{requestID: requestID, owner: owner})
		}
	}
	// The caller holds o.mu; challenge cleanup is deliberately deferred to the
	// worker's explicit expiry path or provider-side TTL pruning to avoid lock
	// inversion with provider mutexes.
	_ = expired
}

func (o *sessionRecoveryDurableOutbox) pendingCountLocked() int {
	count := 0
	for _, record := range o.records {
		if record.DeliveryState == sessionRecoveryOutboxStatePending {
			count++
		}
	}
	return count
}

func (o *sessionRecoveryDurableOutbox) recordCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.records)
}
