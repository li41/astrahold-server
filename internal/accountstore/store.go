package accountstore

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	LegacySchemaVersion  uint16 = 3
	SchemaVersion        uint16 = 4
	MaxAccountIDBytes           = 128
	MaxLoginIDBytes             = 128
	MaxCharacterIDBytes         = 128
	MaxRecoveryIDBytes          = 128
	RecoveryTokenHashBytes      = 32
	MaxRecoveryTTL              = 24 * time.Hour
)

var (
	ErrInvalidStore      = errors.New("accountstore: invalid store")
	ErrRevisionConflict = errors.New("accountstore: revision conflict")
)

type Definition struct {
	SchemaVersion  uint16          `json:"schema_version"`
	Revision       uint64          `json:"revision"`
	Accounts       []Account       `json:"accounts"`
	RecoveryGrants []RecoveryGrant `json:"recovery_grants,omitempty"`
}

type Account struct {
	AccountID           string `json:"account_id"`
	LoginID             string `json:"login_id"`
	PasswordArgon2ID    string `json:"password_argon2id"`
	CredentialVersion   uint64 `json:"credential_version"`
	CreatedAt           string `json:"created_at"`
	PasswordChangedAt   string `json:"password_changed_at"`
	DisabledAt          string `json:"disabled_at,omitempty"`
	CharacterID         string `json:"character_id"`
	AllowActiveTakeover bool   `json:"allow_active_takeover,omitempty"`
}

type RecoveryGrant struct {
	RecoveryID        string `json:"recovery_id"`
	AccountID         string `json:"account_id"`
	CredentialVersion uint64 `json:"credential_version"`
	TokenSHA256       string `json:"token_sha256"`
	IssuedAt          string `json:"issued_at"`
	NotBefore         string `json:"not_before"`
	ExpiresAt         string `json:"expires_at"`
}

func NewEmpty() Definition {
	return Definition{SchemaVersion: SchemaVersion, Revision: 1, Accounts: []Account{}, RecoveryGrants: []RecoveryGrant{}}
}

func Load(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("accountstore: read %q: %w", path, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition Definition
	if err := decoder.Decode(&definition); err != nil {
		return Definition{}, fmt.Errorf("%w: decode: %v", ErrInvalidStore, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Definition{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidStore)
		}
		return Definition{}, fmt.Errorf("%w: trailing data: %v", ErrInvalidStore, err)
	}
	if err := Validate(definition); err != nil {
		return Definition{}, err
	}
	return definition, nil
}

func Validate(definition Definition) error {
	if (definition.SchemaVersion != LegacySchemaVersion && definition.SchemaVersion != SchemaVersion) || definition.Revision == 0 {
		return ErrInvalidStore
	}
	accountIDs := make(map[string]struct{}, len(definition.Accounts))
	loginIDs := make(map[string]struct{}, len(definition.Accounts))
	for index, account := range definition.Accounts {
		if err := validateTrimmed("account_id", account.AccountID, MaxAccountIDBytes); err != nil {
			return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
		}
		if _, exists := accountIDs[account.AccountID]; exists {
			return fmt.Errorf("%w: duplicate account_id %q", ErrInvalidStore, account.AccountID)
		}
		accountIDs[account.AccountID] = struct{}{}
		if err := validateTrimmed("login_id", account.LoginID, MaxLoginIDBytes); err != nil {
			return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
		}
		if _, exists := loginIDs[account.LoginID]; exists {
			return fmt.Errorf("%w: duplicate login_id %q", ErrInvalidStore, account.LoginID)
		}
		loginIDs[account.LoginID] = struct{}{}
		if strings.TrimSpace(account.PasswordArgon2ID) == "" || account.PasswordArgon2ID != strings.TrimSpace(account.PasswordArgon2ID) {
			return fmt.Errorf("%w: account[%d] password_argon2id must be non-empty and trimmed", ErrInvalidStore, index)
		}
		if account.CredentialVersion == 0 {
			return fmt.Errorf("%w: account[%d] credential_version must be > 0", ErrInvalidStore, index)
		}
		if err := validateTimestamp("created_at", account.CreatedAt); err != nil {
			return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
		}
		if err := validateTimestamp("password_changed_at", account.PasswordChangedAt); err != nil {
			return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
		}
		if account.DisabledAt != "" {
			if err := validateTimestamp("disabled_at", account.DisabledAt); err != nil {
				return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
			}
		}
		if err := validateTrimmed("character_id", account.CharacterID, MaxCharacterIDBytes); err != nil {
			return fmt.Errorf("%w: account[%d] %v", ErrInvalidStore, index, err)
		}
	}

	if definition.SchemaVersion == LegacySchemaVersion {
		if len(definition.RecoveryGrants) != 0 {
			return fmt.Errorf("%w: schema_version %d does not permit recovery_grants", ErrInvalidStore, LegacySchemaVersion)
		}
		return nil
	}

	recoveryIDs := make(map[string]struct{}, len(definition.RecoveryGrants))
	tokenHashes := make(map[string]struct{}, len(definition.RecoveryGrants))
	for index, grant := range definition.RecoveryGrants {
		if err := validateTrimmed("recovery_id", grant.RecoveryID, MaxRecoveryIDBytes); err != nil {
			return fmt.Errorf("%w: recovery_grant[%d] %v", ErrInvalidStore, index, err)
		}
		if _, exists := recoveryIDs[grant.RecoveryID]; exists {
			return fmt.Errorf("%w: duplicate recovery_id %q", ErrInvalidStore, grant.RecoveryID)
		}
		recoveryIDs[grant.RecoveryID] = struct{}{}
		if _, exists := accountIDs[grant.AccountID]; !exists {
			return fmt.Errorf("%w: recovery_grant[%d] references unknown account_id %q", ErrInvalidStore, index, grant.AccountID)
		}
		if grant.CredentialVersion == 0 {
			return fmt.Errorf("%w: recovery_grant[%d] credential_version must be > 0", ErrInvalidStore, index)
		}
		if len(grant.TokenSHA256) != RecoveryTokenHashBytes*2 || strings.ToLower(grant.TokenSHA256) != grant.TokenSHA256 {
			return fmt.Errorf("%w: recovery_grant[%d] token_sha256 must be 64 lowercase hex characters", ErrInvalidStore, index)
		}
		if _, err := hex.DecodeString(grant.TokenSHA256); err != nil {
			return fmt.Errorf("%w: recovery_grant[%d] token_sha256 is invalid", ErrInvalidStore, index)
		}
		if _, exists := tokenHashes[grant.TokenSHA256]; exists {
			return fmt.Errorf("%w: duplicate recovery token digest", ErrInvalidStore)
		}
		tokenHashes[grant.TokenSHA256] = struct{}{}
		issuedAt, err := parseTimestamp("issued_at", grant.IssuedAt)
		if err != nil {
			return fmt.Errorf("%w: recovery_grant[%d] %v", ErrInvalidStore, index, err)
		}
		notBefore, err := parseTimestamp("not_before", grant.NotBefore)
		if err != nil {
			return fmt.Errorf("%w: recovery_grant[%d] %v", ErrInvalidStore, index, err)
		}
		expiresAt, err := parseTimestamp("expires_at", grant.ExpiresAt)
		if err != nil {
			return fmt.Errorf("%w: recovery_grant[%d] %v", ErrInvalidStore, index, err)
		}
		if notBefore.Before(issuedAt) {
			return fmt.Errorf("%w: recovery_grant[%d] not_before must not precede issued_at", ErrInvalidStore, index)
		}
		if !expiresAt.After(notBefore) {
			return fmt.Errorf("%w: recovery_grant[%d] expires_at must be after not_before", ErrInvalidStore, index)
		}
		if expiresAt.Sub(issuedAt) > MaxRecoveryTTL {
			return fmt.Errorf("%w: recovery_grant[%d] lifetime exceeds %s", ErrInvalidStore, index, MaxRecoveryTTL)
		}
	}
	return nil
}

func Save(path string, definition Definition) error {
	if err := Validate(definition); err != nil {
		return err
	}
	return saveAtomic(path, definition)
}

func SaveIfRevision(path string, expectedRevision uint64, definition Definition) error {
	current, err := Load(path)
	if err != nil {
		return err
	}
	if current.Revision != expectedRevision {
		return fmt.Errorf("%w: current=%d expected=%d", ErrRevisionConflict, current.Revision, expectedRevision)
	}
	if definition.Revision != expectedRevision+1 {
		return fmt.Errorf("%w: next=%d expected=%d", ErrRevisionConflict, definition.Revision, expectedRevision+1)
	}
	return Save(path, definition)
}

func saveAtomic(path string, definition Definition) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("accountstore: create directory %q: %w", directory, err)
	}
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return fmt.Errorf("accountstore: encode: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(directory, ".accounts-*.tmp")
	if err != nil {
		return fmt.Errorf("accountstore: create temp file: %w", err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("accountstore: chmod temp file: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("accountstore: write temp file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("accountstore: fsync temp file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("accountstore: close temp file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("accountstore: rename temp file: %w", err)
	}
	keep = true
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("accountstore: chmod store: %w", err)
	}
	dir, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("accountstore: open directory for fsync: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("accountstore: fsync directory: %w", err)
	}
	return nil
}

func validateTrimmed(name, value string, maxBytes int) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxBytes {
		return fmt.Errorf("%s must be 1..%d trimmed bytes", name, maxBytes)
	}
	return nil
}

func validateTimestamp(name, value string) error {
	_, err := parseTimestamp(name, value)
	return err
}

func parseTimestamp(name, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("%s is required", name)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC {
		return time.Time{}, fmt.Errorf("%s must be UTC RFC3339", name)
	}
	return parsed, nil
}
