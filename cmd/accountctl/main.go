package main

import (
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/accountstore"
)

const (
	passwordMinBytes       = 12
	passwordMaxBytes       = 256
	argon2MemoryKiB uint32 = 64 * 1024
	argon2Time      uint32 = 3
	argon2Threads   uint8  = 4
	argon2SaltBytes        = 16
	argon2DigestBytes      = 32
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: accountctl <init|create|set-password|disable|enable> [flags]")
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = runInit(os.Args[2:])
	case "create":
		err = runCreate(os.Args[2:])
	case "set-password":
		err = runSetPassword(os.Args[2:])
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
	path := flags.String("path", "", "Durable schema-v3 account store path")
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

func runCreate(args []string) error {
	flags := flag.NewFlagSet("create", flag.ContinueOnError)
	path := flags.String("path", "", "Durable schema-v3 account store path")
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
	path := flags.String("path", "", "Durable schema-v3 account store path")
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
		definition.Accounts[index].PasswordArgon2ID = phc
		definition.Accounts[index].CredentialVersion++
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

func runDisabled(args []string, disabled bool) error {
	name := "enable"
	if disabled {
		name = "disable"
	}
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	path := flags.String("path", "", "Durable schema-v3 account store path")
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
		account.CredentialVersion++
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

func hashPassword(password []byte) (string, error) {
	var salt [argon2SaltBytes]byte
	if _, err := io.ReadFull(rand.Reader, salt[:]); err != nil {
		return "", fmt.Errorf("read password salt: %w", err)
	}
	digest := argon2.IDKey(password, salt[:], argon2Time, argon2MemoryKiB, argon2Threads, argon2DigestBytes)
	defer clear(digest)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argon2MemoryKiB,
		argon2Time,
		argon2Threads,
		base64.RawStdEncoding.EncodeToString(salt[:]),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
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
