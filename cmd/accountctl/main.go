package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/accountstore"
)

const (
	passwordMinBytes = 6
	passwordMaxBytes = 256

	argon2Version              = 19
	argon2MemoryKiB     uint32 = 64 * 1024
	argon2Time          uint32 = 3
	argon2Threads       uint8  = 4
	argon2MaxMemoryKiB  uint32 = 128 * 1024
	argon2MaxTime       uint32 = 10
	argon2MinThreads    uint8  = 1
	argon2MaxThreads    uint8  = 8
	argon2SaltBytes            = 16
	argon2MaxSaltBytes         = 64
	argon2DigestBytes          = 32

	recoveryTokenBytes = 32
	recoveryDefaultTTL = 15 * time.Minute
	recoveryMinTTL     = time.Minute
)

type passwordHash struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	digest  []byte
}

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: accountctl <init|migrate|create|set-password|issue-recovery|reset-password|rehash-password|disable|enable> [flags]")
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(os.Args[2:])
	case "migrate":
		err = runMigrate(os.Args[2:])
	case "create":
		err = runCreate(os.Args[2:])
	case "set-password":
		err = runSetPassword(os.Args[2:])
	case "issue-recovery":
		err = runIssueRecovery(os.Args[2:])
	case "reset-password":
		err = runResetPassword(os.Args[2:])
	case "rehash-password":
		err = runRehashPassword(os.Args[2:])
	case "disable":
		err = runDisabled(os.Args[2:], true)
	case "enable":
		err = runDisabled(os.Args[2:], false)
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatalf("%v", err)
	}
}

func runInit(args []string) error {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	path := flags.String("path", "", "Durable current-schema account store path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" {
		return fmt.Errorf("-path is required")
	}
	if _, err := os.Stat(*path); err == nil {
		return fmt.Errorf("store %q already exists", *path)
	} else if !os.IsNotExist(err) {
		return err
	}
	return accountstore.Save(*path, accountstore.NewEmpty())
}

func runMigrate(args []string) error {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	path := flags.String("path", "", "Durable account store path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" {
		return fmt.Errorf("-path is required")
	}
	definition, err := accountstore.Load(*path)
	if err != nil {
		return err
	}
	if definition.SchemaVersion == accountstore.SchemaVersion {
		return nil
	}
	if definition.SchemaVersion != accountstore.LegacySchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", definition.SchemaVersion)
	}
	previous := definition.Revision
	definition.SchemaVersion = accountstore.SchemaVersion
	definition.Revision++
	definition.RecoveryGrants = []accountstore.RecoveryGrant{}
	return accountstore.SaveIfRevision(*path, previous, definition)
}

func runCreate(args []string) error {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	path := flags.String("path", "", "Durable account store path")
	loginID := flags.String("login", "", "Login ID")
	characterID := flags.String("character", "", "Server-owned CharacterID")
	allowTakeover := flags.Bool("allow-active-takeover", false, "Allow Server-authorized active takeover for this account")
	passwordStdin := flags.Bool("password-stdin", false, "Read the password from stdin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*loginID) == "" || strings.TrimSpace(*characterID) == "" || !*passwordStdin {
		return fmt.Errorf("-path, -login, -character, and -password-stdin are required")
	}
	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	defer clear(password)
	phc, err := hashPassword(password)
	if err != nil {
		return err
	}
	definition, err := accountstore.Load(*path)
	if err != nil {
		return err
	}
	for _, account := range definition.Accounts {
		if account.LoginID == *loginID {
			return fmt.Errorf("login %q already exists", *loginID)
		}
	}
	accountID, err := randomAccountID()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	definition.Accounts = append(definition.Accounts, accountstore.Account{
		AccountID:           accountID,
		LoginID:             *loginID,
		PasswordArgon2ID:    phc,
		CredentialVersion:   1,
		CreatedAt:           now,
		PasswordChangedAt:   now,
		CharacterID:         *characterID,
		AllowActiveTakeover: *allowTakeover,
	})
	previous := definition.Revision
	definition.Revision++
	return accountstore.SaveIfRevision(*path, previous, definition)
}

func runSetPassword(args []string) error {
	flags := flag.NewFlagSet("set-password", flag.ContinueOnError)
	path := flags.String("path", "", "Durable account store path")
	loginID := flags.String("login", "", "Login ID")
	passwordStdin := flags.Bool("password-stdin", false, "Read the password from stdin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*loginID) == "" || !*passwordStdin {
		return fmt.Errorf("-path, -login, and -password-stdin are required")
	}
	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	defer clear(password)
	phc, err := hashPassword(password)
	if err != nil {
		return err
	}
	definition, err := accountstore.Load(*path)
	if err != nil {
		return err
	}
	found := false
	for index := range definition.Accounts {
		if definition.Accounts[index].LoginID != *loginID {
			continue
		}
		if err := incrementCredentialVersion(&definition.Accounts[index]); err != nil {
			return err
		}
		definition.Accounts[index].PasswordArgon2ID = phc
		definition.Accounts[index].PasswordChangedAt = time.Now().UTC().Format(time.RFC3339Nano)
		found = true
		break
	}
	if !found {
		return fmt.Errorf("login %q not found", *loginID)
	}
	previous := definition.Revision
	definition.Revision++
	return accountstore.SaveIfRevision(*path, previous, definition)
}

func runIssueRecovery(args []string) error {
	flags := flag.NewFlagSet("issue-recovery", flag.ContinueOnError)
	path := flags.String("path", "", "Durable schema-v4 account store path")
	loginID := flags.String("login", "", "Login ID")
	tokenOut := flags.String("token-out", "", "New 0600 file that receives the one-time recovery token")
	ttl := flags.Duration("ttl", recoveryDefaultTTL, "Recovery token lifetime (1m..24h)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*loginID) == "" || strings.TrimSpace(*tokenOut) == "" {
		return fmt.Errorf("-path, -login, and -token-out are required")
	}
	if *ttl < recoveryMinTTL || *ttl > accountstore.MaxRecoveryTTL {
		return fmt.Errorf("-ttl must be between %s and %s", recoveryMinTTL, accountstore.MaxRecoveryTTL)
	}
	return issueRecovery(*path, *loginID, *tokenOut, *ttl, time.Now().UTC(), rand.Reader)
}

func issueRecovery(path, loginID, tokenOut string, ttl time.Duration, now time.Time, random io.Reader) error {
	definition, err := accountstore.Load(path)
	if err != nil {
		return err
	}
	if definition.SchemaVersion != accountstore.SchemaVersion {
		return fmt.Errorf("recovery requires schema_version %d; run accountctl migrate first", accountstore.SchemaVersion)
	}
	var account *accountstore.Account
	for index := range definition.Accounts {
		if definition.Accounts[index].LoginID == loginID {
			account = &definition.Accounts[index]
			break
		}
	}
	if account == nil {
		return fmt.Errorf("login %q not found", loginID)
	}
	if random == nil {
		return fmt.Errorf("recovery entropy source is unavailable")
	}
	var entropy [recoveryTokenBytes]byte
	if _, err := io.ReadFull(random, entropy[:]); err != nil {
		return fmt.Errorf("read recovery entropy: %w", err)
	}
	token := make([]byte, base64.RawURLEncoding.EncodedLen(len(entropy)))
	base64.RawURLEncoding.Encode(token, entropy[:])
	clear(entropy[:])
	defer clear(token)
	digest := sha256.Sum256(token)
	digestHex := hex.EncodeToString(digest[:])
	recoveryID := "recovery-" + hex.EncodeToString(digest[:16])

	filtered := definition.RecoveryGrants[:0]
	for _, grant := range definition.RecoveryGrants {
		if grant.AccountID != account.AccountID {
			filtered = append(filtered, grant)
		}
	}
	definition.RecoveryGrants = append(filtered, accountstore.RecoveryGrant{
		RecoveryID:        recoveryID,
		AccountID:         account.AccountID,
		CredentialVersion: account.CredentialVersion,
		TokenSHA256:       digestHex,
		IssuedAt:          now.UTC().Format(time.RFC3339Nano),
		NotBefore:         now.UTC().Format(time.RFC3339Nano),
		ExpiresAt:         now.UTC().Add(ttl).Format(time.RFC3339Nano),
	})
	if err := accountstore.Validate(definition); err != nil {
		return err
	}
	if err := writeSecretFileExclusive(tokenOut, token); err != nil {
		return err
	}
	keepToken := false
	defer func() {
		if !keepToken {
			_ = os.Remove(tokenOut)
		}
	}()
	previous := definition.Revision
	definition.Revision++
	if err := accountstore.SaveIfRevision(path, previous, definition); err != nil {
		return err
	}
	keepToken = true
	return nil
}

func runResetPassword(args []string) error {
	flags := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	path := flags.String("path", "", "Durable schema-v4 account store path")
	tokenFile := flags.String("recovery-token-file", "", "File containing the one-time recovery token")
	passwordStdin := flags.Bool("password-stdin", false, "Read the new password from stdin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*tokenFile) == "" || !*passwordStdin {
		return fmt.Errorf("-path, -recovery-token-file, and -password-stdin are required")
	}
	token, err := readRecoveryToken(*tokenFile)
	if err != nil {
		return err
	}
	defer clear(token)
	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	defer clear(password)
	return resetPassword(*path, token, password, time.Now().UTC())
}

func resetPassword(path string, token, password []byte, now time.Time) error {
	definition, err := accountstore.Load(path)
	if err != nil {
		return err
	}
	if definition.SchemaVersion != accountstore.SchemaVersion {
		return fmt.Errorf("password recovery requires schema_version %d", accountstore.SchemaVersion)
	}
	digest := sha256.Sum256(token)
	digestHex := hex.EncodeToString(digest[:])
	grantIndex := -1
	for index := range definition.RecoveryGrants {
		if subtle.ConstantTimeCompare([]byte(definition.RecoveryGrants[index].TokenSHA256), []byte(digestHex)) == 1 {
			grantIndex = index
			break
		}
	}
	if grantIndex < 0 {
		return fmt.Errorf("recovery token is invalid or inactive")
	}
	grant := definition.RecoveryGrants[grantIndex]
	notBefore, err := time.Parse(time.RFC3339Nano, grant.NotBefore)
	if err != nil {
		return fmt.Errorf("recovery token is invalid or inactive")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, grant.ExpiresAt)
	if err != nil || now.Before(notBefore) || !now.Before(expiresAt) {
		return fmt.Errorf("recovery token is invalid or inactive")
	}
	accountIndex := -1
	for index := range definition.Accounts {
		if definition.Accounts[index].AccountID == grant.AccountID {
			accountIndex = index
			break
		}
	}
	if accountIndex < 0 || definition.Accounts[accountIndex].CredentialVersion != grant.CredentialVersion {
		return fmt.Errorf("recovery token is invalid or inactive")
	}
	phc, err := hashPassword(password)
	if err != nil {
		return err
	}
	account := &definition.Accounts[accountIndex]
	if err := incrementCredentialVersion(account); err != nil {
		return err
	}
	account.PasswordArgon2ID = phc
	account.PasswordChangedAt = now.UTC().Format(time.RFC3339Nano)
	filtered := definition.RecoveryGrants[:0]
	for _, candidate := range definition.RecoveryGrants {
		if candidate.AccountID != account.AccountID {
			filtered = append(filtered, candidate)
		}
	}
	definition.RecoveryGrants = filtered
	previous := definition.Revision
	definition.Revision++
	return accountstore.SaveIfRevision(path, previous, definition)
}

func runRehashPassword(args []string) error {
	flags := flag.NewFlagSet("rehash-password", flag.ContinueOnError)
	path := flags.String("path", "", "Durable account store path")
	loginID := flags.String("login", "", "Login ID")
	passwordStdin := flags.Bool("password-stdin", false, "Read the current password from stdin")
	memoryKiB := flags.Uint("memory-kib", uint(argon2MemoryKiB), "Target Argon2id memory in KiB")
	timeCost := flags.Uint("time", uint(argon2Time), "Target Argon2id time cost")
	threads := flags.Uint("threads", uint(argon2Threads), "Target Argon2id parallelism")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*loginID) == "" || !*passwordStdin {
		return fmt.Errorf("-path, -login, and -password-stdin are required")
	}
	if *memoryKiB > uint(^uint32(0)) || *timeCost > uint(^uint32(0)) || *threads > uint(^uint8(0)) {
		return fmt.Errorf("target Argon2id parameters are out of range")
	}
	targetMemory, targetTime, targetThreads := uint32(*memoryKiB), uint32(*timeCost), uint8(*threads)
	if err := validateArgon2Policy(targetMemory, targetTime, targetThreads); err != nil {
		return err
	}
	password, err := readPassword(os.Stdin)
	if err != nil {
		return err
	}
	defer clear(password)
	return rehashPassword(*path, *loginID, password, targetMemory, targetTime, targetThreads)
}

func rehashPassword(path, loginID string, password []byte, memory, timeCost uint32, threads uint8) error {
	definition, err := accountstore.Load(path)
	if err != nil {
		return err
	}
	accountIndex := -1
	for index := range definition.Accounts {
		if definition.Accounts[index].LoginID == loginID {
			accountIndex = index
			break
		}
	}
	if accountIndex < 0 {
		return fmt.Errorf("login %q not found", loginID)
	}
	parsed, err := parsePasswordHash(definition.Accounts[accountIndex].PasswordArgon2ID)
	if err != nil {
		return fmt.Errorf("current password verifier: %w", err)
	}
	if !verifyPassword(password, parsed) {
		return fmt.Errorf("current password is invalid")
	}
	if parsed.memory == memory && parsed.time == timeCost && parsed.threads == threads {
		return nil
	}
	phc, err := hashPasswordWithPolicy(password, memory, timeCost, threads)
	if err != nil {
		return err
	}
	account := &definition.Accounts[accountIndex]
	if err := incrementCredentialVersion(account); err != nil {
		return err
	}
	account.PasswordArgon2ID = phc
	// This is KDF policy migration, not a human password change. Preserve
	// PasswordChangedAt while the credential generation advances.
	previous := definition.Revision
	definition.Revision++
	return accountstore.SaveIfRevision(path, previous, definition)
}

func runDisabled(args []string, disabled bool) error {
	name := "enable"
	if disabled {
		name = "disable"
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	path := flags.String("path", "", "Durable account store path")
	loginID := flags.String("login", "", "Login ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*path) == "" || strings.TrimSpace(*loginID) == "" {
		return fmt.Errorf("-path and -login are required")
	}
	definition, err := accountstore.Load(*path)
	if err != nil {
		return err
	}
	found := false
	for index := range definition.Accounts {
		account := &definition.Accounts[index]
		if account.LoginID != *loginID {
			continue
		}
		if disabled {
			if account.DisabledAt != "" {
				return fmt.Errorf("login %q is already disabled", *loginID)
			}
			account.DisabledAt = time.Now().UTC().Format(time.RFC3339Nano)
		} else {
			if account.DisabledAt == "" {
				return fmt.Errorf("login %q is already enabled", *loginID)
			}
			account.DisabledAt = ""
		}
		if err := incrementCredentialVersion(account); err != nil {
			return err
		}
		found = true
		break
	}
	if !found {
		return fmt.Errorf("login %q not found", *loginID)
	}
	previous := definition.Revision
	definition.Revision++
	return accountstore.SaveIfRevision(*path, previous, definition)
}

func readPassword(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, passwordMaxBytes+3))
	if err != nil {
		return nil, err
	}
	data = []byte(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	if len(data) < passwordMinBytes || len(data) > passwordMaxBytes {
		clear(data)
		return nil, fmt.Errorf("password must be %d..%d bytes", passwordMinBytes, passwordMaxBytes)
	}
	if strings.ContainsAny(string(data), "\r\n") {
		clear(data)
		return nil, fmt.Errorf("password must be a single line")
	}
	return data, nil
}

func readRecoveryToken(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 512))
	if err != nil {
		return nil, err
	}
	data = []byte(strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r"))
	if len(data) == 0 || len(data) > 256 || strings.ContainsAny(string(data), "\r\n") {
		clear(data)
		return nil, fmt.Errorf("recovery token file is invalid")
	}
	return data, nil
}

func hashPassword(password []byte) (string, error) {
	return hashPasswordWithPolicy(password, argon2MemoryKiB, argon2Time, argon2Threads)
}

func hashPasswordWithPolicy(password []byte, memory, timeCost uint32, threads uint8) (string, error) {
	if err := validateArgon2Policy(memory, timeCost, threads); err != nil {
		return "", err
	}
	var salt [argon2SaltBytes]byte
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}
	digest := argon2.IDKey(password, salt[:], timeCost, memory, threads, argon2DigestBytes)
	defer clear(digest)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		memory,
		timeCost,
		threads,
		base64.RawStdEncoding.EncodeToString(salt[:]),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

func parsePasswordHash(encoded string) (passwordHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2Version) {
		return passwordHash{}, fmt.Errorf("must use Argon2id v=%d PHC format", argon2Version)
	}
	memory, timeCost, threads, err := parseArgon2Parameters(parts[3])
	if err != nil {
		return passwordHash{}, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < argon2SaltBytes || len(salt) > argon2MaxSaltBytes {
		return passwordHash{}, fmt.Errorf("salt must be %d..%d bytes of unpadded base64", argon2SaltBytes, argon2MaxSaltBytes)
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(digest) != argon2DigestBytes {
		return passwordHash{}, fmt.Errorf("digest must be %d bytes of unpadded base64", argon2DigestBytes)
	}
	return passwordHash{memory: memory, time: timeCost, threads: threads, salt: salt, digest: digest}, nil
}

func parseArgon2Parameters(encoded string) (uint32, uint32, uint8, error) {
	fields := strings.Split(encoded, ",")
	if len(fields) != 3 {
		return 0, 0, 0, fmt.Errorf("Argon2id parameters must contain exactly m,t,p")
	}
	values := map[string]uint64{}
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return 0, 0, 0, fmt.Errorf("invalid Argon2id parameter %q", field)
		}
		if _, exists := values[parts[0]]; exists {
			return 0, 0, 0, fmt.Errorf("duplicate Argon2id parameter %q", parts[0])
		}
		value, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid Argon2id parameter %q", field)
		}
		values[parts[0]] = value
	}
	memory, mok := values["m"]
	timeCost, tok := values["t"]
	threads, pok := values["p"]
	if !mok || !tok || !pok {
		return 0, 0, 0, fmt.Errorf("Argon2id parameters must contain m,t,p")
	}
	if memory > uint64(^uint32(0)) || timeCost > uint64(^uint32(0)) || threads > uint64(^uint8(0)) {
		return 0, 0, 0, fmt.Errorf("Argon2id parameters are out of range")
	}
	m, tc, p := uint32(memory), uint32(timeCost), uint8(threads)
	if err := validateArgon2Policy(m, tc, p); err != nil {
		return 0, 0, 0, err
	}
	return m, tc, p, nil
}

func validateArgon2Policy(memory, timeCost uint32, threads uint8) error {
	if memory < argon2MemoryKiB || memory > argon2MaxMemoryKiB {
		return fmt.Errorf("Argon2id memory must be between %d and %d KiB", argon2MemoryKiB, argon2MaxMemoryKiB)
	}
	if timeCost < argon2Time || timeCost > argon2MaxTime {
		return fmt.Errorf("Argon2id time must be between %d and %d", argon2Time, argon2MaxTime)
	}
	if threads < argon2MinThreads || threads > argon2MaxThreads {
		return fmt.Errorf("Argon2id threads must be between %d and %d", argon2MinThreads, argon2MaxThreads)
	}
	return nil
}

func verifyPassword(password []byte, verifier passwordHash) bool {
	derived := argon2.IDKey(password, verifier.salt, verifier.time, verifier.memory, verifier.threads, uint32(len(verifier.digest)))
	matched := subtle.ConstantTimeCompare(derived, verifier.digest) == 1
	clear(derived)
	return matched
}

func writeSecretFileExclusive(path string, secret []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("secret output path is required")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create secret file: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(secret); err != nil {
		return fmt.Errorf("write secret file: %w", err)
	}
	if _, err := file.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write secret newline: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("fsync secret file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close secret file: %w", err)
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open secret directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("fsync secret directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("close secret directory: %w", err)
	}
	keep = true
	return nil
}

func incrementCredentialVersion(account *accountstore.Account) error {
	if account == nil || account.CredentialVersion == ^uint64(0) {
		return fmt.Errorf("credential version cannot advance")
	}
	account.CredentialVersion++
	return nil
}

func randomAccountID() (string, error) {
	var random [16]byte
	if _, err := io.ReadFull(rand.Reader, random[:]); err != nil {
		return "", fmt.Errorf("read account id entropy: %w", err)
	}
	return "acct-" + base64.RawURLEncoding.EncodeToString(random[:]), nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "accountctl: "+format+"\n", args...)
	os.Exit(1)
}