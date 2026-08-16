package accountstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreSaveLoadAndRevisionCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := NewEmpty()
	if err := Save(path, definition); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o want=600", info.Mode().Perm())
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 1 || loaded.SchemaVersion != SchemaVersion || len(loaded.Accounts) != 0 {
		t.Fatalf("loaded=%+v", loaded)
	}

	next := loaded
	next.Revision = 2
	next.Accounts = append(next.Accounts, Account{
		AccountID:         "acct-test",
		LoginID:           "alice",
		PasswordArgon2ID:  "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$ZGlnaWVzdGRpZ2VzdGRpZ2VzdGRpZ2VzdGRpZ2VzdDE",
		CredentialVersion: 1,
		CreatedAt:         "2026-08-16T00:00:00Z",
		PasswordChangedAt: "2026-08-16T00:00:00Z",
		CharacterID:       "alice-character",
	})
	if err := SaveIfRevision(path, 1, next); err != nil {
		t.Fatal(err)
	}
	if err := SaveIfRevision(path, 1, next); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale CAS err=%v want revision conflict", err)
	}
}

func TestStoreRejectsDuplicateLoginAndUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	data := `{"schema_version":3,"revision":1,"unknown":true,"accounts":[]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("unknown field must fail closed")
	}

	definition := NewEmpty()
	definition.Accounts = []Account{
		{AccountID: "a", LoginID: "same", PasswordArgon2ID: "x", CredentialVersion: 1, CreatedAt: "2026-08-16T00:00:00Z", PasswordChangedAt: "2026-08-16T00:00:00Z", CharacterID: "c1"},
		{AccountID: "b", LoginID: "same", PasswordArgon2ID: "y", CredentialVersion: 1, CreatedAt: "2026-08-16T00:00:00Z", PasswordChangedAt: "2026-08-16T00:00:00Z", CharacterID: "c2"},
	}
	if err := Validate(definition); err == nil {
		t.Fatal("duplicate login must fail")
	}
}
