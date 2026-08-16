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
	if loaded.Revision != 1 || loaded.SchemaVersion != SchemaVersion || len(loaded.Accounts) != 0 || len(loaded.RecoveryGrants) != 0 {
		t.Fatalf("loaded=%+v", loaded)
	}

	next := loaded
	next.Revision = 2
	next.Accounts = append(next.Accounts, testAccount("acct-test", "alice", "alice-character"))
	if err := SaveIfRevision(path, 1, next); err != nil {
		t.Fatal(err)
	}
	if err := SaveIfRevision(path, 1, next); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale CAS err=%v want revision conflict", err)
	}
}

func TestStoreLoadsLegacySchemaV3WithoutRecoveryGrants(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts-v3.json")
	data := `{"schema_version":3,"revision":7,"accounts":[{"account_id":"acct-test","login_id":"alice","password_argon2id":"x","credential_version":1,"created_at":"2026-08-16T00:00:00Z","password_changed_at":"2026-08-16T00:00:00Z","character_id":"alice-character"}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != LegacySchemaVersion || loaded.Revision != 7 || len(loaded.RecoveryGrants) != 0 {
		t.Fatalf("loaded=%+v", loaded)
	}

	loaded.RecoveryGrants = []RecoveryGrant{{RecoveryID: "not-allowed"}}
	if err := Validate(loaded); err == nil {
		t.Fatal("schema v3 must reject recovery grants")
	}
}

func TestStoreValidatesRecoveryGrants(t *testing.T) {
	definition := NewEmpty()
	definition.Accounts = []Account{testAccount("acct-test", "alice", "alice-character")}
	definition.RecoveryGrants = []RecoveryGrant{{
		RecoveryID:        "recovery-1",
		AccountID:         "acct-test",
		CredentialVersion: 1,
		TokenSHA256:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		IssuedAt:          "2026-08-16T00:00:00Z",
		NotBefore:         "2026-08-16T00:00:00Z",
		ExpiresAt:         "2026-08-16T00:15:00Z",
	}}
	if err := Validate(definition); err != nil {
		t.Fatal(err)
	}

	cases := map[string]func(*Definition){
		"unknown-account": func(candidate *Definition) { candidate.RecoveryGrants[0].AccountID = "missing" },
		"zero-generation": func(candidate *Definition) { candidate.RecoveryGrants[0].CredentialVersion = 0 },
		"bad-hash":        func(candidate *Definition) { candidate.RecoveryGrants[0].TokenSHA256 = "xyz" },
		"before-issued":   func(candidate *Definition) { candidate.RecoveryGrants[0].NotBefore = "2026-08-15T23:59:59Z" },
		"empty-window":    func(candidate *Definition) { candidate.RecoveryGrants[0].ExpiresAt = candidate.RecoveryGrants[0].NotBefore },
		"too-long":        func(candidate *Definition) { candidate.RecoveryGrants[0].ExpiresAt = "2026-08-17T00:00:01Z" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := definition
			candidate.RecoveryGrants = append([]RecoveryGrant(nil), definition.RecoveryGrants...)
			mutate(&candidate)
			if err := Validate(candidate); err == nil {
				t.Fatal("invalid recovery grant must fail closed")
			}
		})
	}

	duplicate := definition
	duplicate.RecoveryGrants = append([]RecoveryGrant(nil), definition.RecoveryGrants...)
	second := definition.RecoveryGrants[0]
	second.RecoveryID = "recovery-2"
	duplicate.RecoveryGrants = append(duplicate.RecoveryGrants, second)
	if err := Validate(duplicate); err == nil {
		t.Fatal("duplicate token digest must fail")
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
		testAccount("a", "same", "c1"),
		testAccount("b", "same", "c2"),
	}
	if err := Validate(definition); err == nil {
		t.Fatal("duplicate login must fail")
	}
}

func testAccount(accountID, loginID, characterID string) Account {
	return Account{
		AccountID:         accountID,
		LoginID:           loginID,
		PasswordArgon2ID:  "$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHRzYWx0c2FsdA$ZGlnaWVzdGRpZ2VzdGRpZ2VzdGRpZ2VzdGRpZ2VzdDE",
		CredentialVersion: 1,
		CreatedAt:         "2026-08-16T00:00:00Z",
		PasswordChangedAt: "2026-08-16T00:00:00Z",
		CharacterID:       characterID,
	}
}
