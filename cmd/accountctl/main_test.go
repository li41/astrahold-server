package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/accountstore"
)

func TestMigrateSchemaV3ToV4PreservesAccounts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := accountDefinitionForTest(accountstore.LegacySchemaVersion, "old-password-123")
	if err := accountstore.Save(path, definition); err != nil {
		t.Fatal(err)
	}
	if err := runMigrate([]string{"-path", path}); err != nil {
		t.Fatal(err)
	}
	loaded, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != accountstore.SchemaVersion || loaded.Revision != definition.Revision+1 || len(loaded.Accounts) != 1 || len(loaded.RecoveryGrants) != 0 {
		t.Fatalf("loaded=%+v", loaded)
	}
	if loaded.Accounts[0].LoginID != "alice" || loaded.Accounts[0].CredentialVersion != 1 {
		t.Fatalf("account changed during migration: %+v", loaded.Accounts[0])
	}
	if err := runMigrate([]string{"-path", path}); err != nil {
		t.Fatalf("current-schema migration must be idempotent: %v", err)
	}
}

func TestRecoveryIssueResetAndReuseRejection(t *testing.T) {
	now := time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "accounts.json")
	tokenPath := filepath.Join(t.TempDir(), "recovery.token")
	definition := accountDefinitionForTest(accountstore.SchemaVersion, "old-password-123")
	if err := accountstore.Save(path, definition); err != nil {
		t.Fatal(err)
	}
	entropy := make([]byte, recoveryTokenBytes)
	for index := range entropy {
		entropy[index] = byte(index + 1)
	}
	if err := issueRecovery(path, "alice", tokenPath, 15*time.Minute, now, bytes.NewReader(entropy)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode=%o want=600", info.Mode().Perm())
	}
	token, err := readRecoveryToken(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(token)
	storeBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(storeBytes, token) {
		t.Fatal("plaintext recovery token leaked into durable store")
	}
	digest := sha256.Sum256(token)
	if !bytes.Contains(storeBytes, []byte(hex.EncodeToString(digest[:]))) {
		t.Fatal("recovery token digest missing from store")
	}
	issued, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(issued.RecoveryGrants) != 1 || issued.RecoveryGrants[0].CredentialVersion != 1 {
		t.Fatalf("grants=%+v", issued.RecoveryGrants)
	}

	newPassword := []byte("new-password-456")
	if err := resetPassword(path, token, newPassword, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	reset, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reset.Accounts[0].CredentialVersion != 2 || len(reset.RecoveryGrants) != 0 {
		t.Fatalf("reset store=%+v", reset)
	}
	if reset.Accounts[0].PasswordChangedAt != now.Add(time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("password_changed_at=%q", reset.Accounts[0].PasswordChangedAt)
	}
	parsed, err := parsePasswordHash(reset.Accounts[0].PasswordArgon2ID)
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(newPassword, parsed) || verifyPassword([]byte("old-password-123"), parsed) {
		t.Fatal("reset password verifier does not match the new password exclusively")
	}
	revision := reset.Revision
	if err := resetPassword(path, token, []byte("third-password-789"), now.Add(2*time.Minute)); err == nil {
		t.Fatal("consumed recovery token must not be reusable")
	}
	afterReuse, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if afterReuse.Revision != revision {
		t.Fatalf("failed reuse mutated store revision: %d -> %d", revision, afterReuse.Revision)
	}
}

func TestRecoveryGrantIsLatestOnlyAndGenerationBound(t *testing.T) {
	now := time.Date(2026, 8, 16, 2, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := accountDefinitionForTest(accountstore.SchemaVersion, "old-password-123")
	if err := accountstore.Save(path, definition); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(t.TempDir(), "first.token")
	secondPath := filepath.Join(t.TempDir(), "second.token")
	if err := issueRecovery(path, "alice", firstPath, 15*time.Minute, now, bytes.NewReader(bytes.Repeat([]byte{1}, recoveryTokenBytes))); err != nil {
		t.Fatal(err)
	}
	if err := issueRecovery(path, "alice", secondPath, 15*time.Minute, now.Add(time.Minute), bytes.NewReader(bytes.Repeat([]byte{2}, recoveryTokenBytes))); err != nil {
		t.Fatal(err)
	}
	loaded, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.RecoveryGrants) != 1 {
		t.Fatalf("reissue must retain one latest grant, got %d", len(loaded.RecoveryGrants))
	}
	firstToken, err := readRecoveryToken(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(firstToken)
	if err := resetPassword(path, firstToken, []byte("new-password-456"), now.Add(2*time.Minute)); err == nil {
		t.Fatal("superseded recovery token must fail")
	}

	secondToken, err := readRecoveryToken(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(secondToken)
	loaded.Accounts[0].CredentialVersion++
	loaded.Revision++
	if err := accountstore.SaveIfRevision(path, loaded.Revision-1, loaded); err != nil {
		t.Fatal(err)
	}
	if err := resetPassword(path, secondToken, []byte("new-password-456"), now.Add(2*time.Minute)); err == nil {
		t.Fatal("recovery token from stale credential generation must fail")
	}
}

func TestRecoveryExactExpiryRejectsWithoutMutation(t *testing.T) {
	now := time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "accounts.json")
	tokenPath := filepath.Join(t.TempDir(), "recovery.token")
	if err := accountstore.Save(path, accountDefinitionForTest(accountstore.SchemaVersion, "old-password-123")); err != nil {
		t.Fatal(err)
	}
	if err := issueRecovery(path, "alice", tokenPath, time.Minute, now, bytes.NewReader(bytes.Repeat([]byte{7}, recoveryTokenBytes))); err != nil {
		t.Fatal(err)
	}
	token, err := readRecoveryToken(tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(token)
	before, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := resetPassword(path, token, []byte("new-password-456"), now.Add(time.Minute)); err == nil {
		t.Fatal("exact expires_at must reject recovery")
	}
	after, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || after.Accounts[0].CredentialVersion != before.Accounts[0].CredentialVersion {
		t.Fatal("expired recovery attempt mutated store")
	}
}

func TestRehashPasswordMigratesPolicyWithoutChangingPasswordAge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	definition := accountDefinitionForTest(accountstore.SchemaVersion, "old-password-123")
	originalChangedAt := definition.Accounts[0].PasswordChangedAt
	if err := accountstore.Save(path, definition); err != nil {
		t.Fatal(err)
	}
	password := []byte("old-password-123")
	if err := rehashPassword(path, "alice", password, argon2MemoryKiB, 4, argon2Threads); err != nil {
		t.Fatal(err)
	}
	loaded, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Accounts[0].CredentialVersion != 2 || loaded.Accounts[0].PasswordChangedAt != originalChangedAt {
		t.Fatalf("rehash account=%+v", loaded.Accounts[0])
	}
	parsed, err := parsePasswordHash(loaded.Accounts[0].PasswordArgon2ID)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.time != 4 || parsed.memory != argon2MemoryKiB || parsed.threads != argon2Threads || !verifyPassword(password, parsed) {
		t.Fatalf("rehash policy m=%d t=%d p=%d", parsed.memory, parsed.time, parsed.threads)
	}
	revision := loaded.Revision
	if err := rehashPassword(path, "alice", password, argon2MemoryKiB, 4, argon2Threads); err != nil {
		t.Fatal(err)
	}
	idempotent, err := accountstore.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if idempotent.Revision != revision {
		t.Fatal("already-targeted KDF policy must not rewrite store")
	}
	if err := rehashPassword(path, "alice", []byte("wrong-password-000"), argon2MemoryKiB, 5, argon2Threads); err == nil {
		t.Fatal("wrong current password must not migrate KDF")
	}
}

func accountDefinitionForTest(schema uint16, password string) accountstore.Definition {
	return accountstore.Definition{
		SchemaVersion: schema,
		Revision:      1,
		Accounts: []accountstore.Account{{
			AccountID:           "acct-alice",
			LoginID:             "alice",
			PasswordArgon2ID:    deterministicPHC(password, []byte("0123456789abcdef"), argon2MemoryKiB, argon2Time, argon2Threads),
			CredentialVersion:   1,
			CreatedAt:           "2026-08-16T00:00:00Z",
			PasswordChangedAt:   "2026-08-16T00:00:00Z",
			CharacterID:         "alice-character",
			AllowActiveTakeover: true,
		}},
		RecoveryGrants: []accountstore.RecoveryGrant{},
	}
}

func deterministicPHC(password string, salt []byte, memory, timeCost uint32, threads uint8) string {
	digest := argon2.IDKey([]byte(password), salt, timeCost, memory, threads, argon2DigestBytes)
	defer clear(digest)
	return "$argon2id$v=19$m=" + strconvForTest(memory) + ",t=" + strconvForTest(timeCost) + ",p=" + strconvForTest(uint32(threads)) + "$" +
		base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(digest)
}

func strconvForTest(value uint32) string {
	return strings.TrimSpace(fmtUint(value))
}

func fmtUint(value uint32) string {
	if value == 0 {
		return "0"
	}
	var digits [10]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
