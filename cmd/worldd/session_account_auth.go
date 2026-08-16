package main

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

const (
	sessionPasswordSchemaVersion       uint16 = 2
	sessionPasswordArgon2Version              = 19
	sessionPasswordArgon2MinMemoryKiB  uint32 = 64 * 1024
	sessionPasswordArgon2MaxMemoryKiB  uint32 = 128 * 1024
	sessionPasswordArgon2MinTime       uint32 = 3
	sessionPasswordArgon2MaxTime       uint32 = 10
	sessionPasswordArgon2MinThreads    uint8  = 1
	sessionPasswordArgon2MaxThreads    uint8  = 8
	sessionPasswordArgon2SaltMinBytes         = 16
	sessionPasswordArgon2SaltMaxBytes         = 64
	sessionPasswordArgon2DigestBytes          = 32
	sessionPasswordMaxConcurrentChecks        = 4
)

type sessionAccountAuthenticator interface {
	Authenticate(context.Context, string, []byte) (sessioncredential.Grant, error)
	Revision() string
	Method() string
}

type sessionAccountGenerationValidator interface {
	StoreRevision() uint64
	GenerationActive(string, string) bool
}

type sessionAccountAuthRuntime struct {
	mu            sync.RWMutex
	authenticator sessionAccountAuthenticator
}

func newSessionAccountAuthRuntime(authenticator sessionAccountAuthenticator) (*sessionAccountAuthRuntime, error) {
	if authenticator == nil || strings.TrimSpace(authenticator.Revision()) == "" || strings.TrimSpace(authenticator.Method()) == "" {
		return nil, errSessionLoginConfig
	}
	return &sessionAccountAuthRuntime{authenticator: authenticator}, nil
}

func (r *sessionAccountAuthRuntime) snapshot() sessionAccountAuthenticator {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.authenticator
}

func (r *sessionAccountAuthRuntime) Authenticate(ctx context.Context, loginID string, secret []byte) (sessioncredential.Grant, error) {
	authenticator := r.snapshot()
	if authenticator == nil {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	return authenticator.Authenticate(ctx, loginID, secret)
}

func (r *sessionAccountAuthRuntime) Revision() string {
	authenticator := r.snapshot()
	if authenticator == nil {
		return ""
	}
	return authenticator.Revision()
}

func (r *sessionAccountAuthRuntime) Method() string {
	authenticator := r.snapshot()
	if authenticator == nil {
		return ""
	}
	return authenticator.Method()
}

func (r *sessionAccountAuthRuntime) replace(authenticator sessionAccountAuthenticator) {
	if r == nil || authenticator == nil {
		return
	}
	r.mu.Lock()
	r.authenticator = authenticator
	r.mu.Unlock()
}

func (r *sessionAccountAuthRuntime) grantCurrent(grant sessioncredential.Grant) bool {
	if !grant.Valid() {
		return false
	}
	if grant.AuthenticationSubject == "" && grant.AuthenticationGeneration == "" {
		return true
	}
	if grant.AuthenticationSubject == "" || grant.AuthenticationGeneration == "" {
		return false
	}
	authenticator := r.snapshot()
	validator, ok := authenticator.(sessionAccountGenerationValidator)
	if !ok {
		return false
	}
	return validator.GenerationActive(grant.AuthenticationSubject, grant.AuthenticationGeneration)
}

func (r *sessionAccountAuthRuntime) reloadMetadata() (string, uint64, bool) {
	authenticator := r.snapshot()
	validator, ok := authenticator.(sessionAccountGenerationValidator)
	if !ok || authenticator == nil || validator.StoreRevision() == 0 {
		return "", 0, false
	}
	return authenticator.Revision(), validator.StoreRevision(), true
}

func (a *staticSessionLoginAuthenticator) Revision() string {
	if a == nil {
		return ""
	}
	return a.revision
}

func (a *staticSessionLoginAuthenticator) Method() string {
	return "sha256-high-entropy"
}

type sessionPasswordLoginDefinition struct {
	SchemaVersion uint16                        `json:"schema_version"`
	Revision      string                        `json:"revision"`
	Accounts      []sessionPasswordLoginAccount `json:"accounts"`
}

type sessionPasswordLoginAccount struct {
	LoginID             string `json:"login_id"`
	PasswordArgon2ID    string `json:"password_argon2id"`
	CharacterID         string `json:"character_id"`
	AllowActiveTakeover bool   `json:"allow_active_takeover,omitempty"`
}

type argon2idPasswordHash struct {
	memory  uint32
	time    uint32
	threads uint8
	salt    []byte
	digest  []byte
}

type argon2idSessionLoginAccount struct {
	password argon2idPasswordHash
	grant    sessioncredential.Grant
}

type argon2idDeriveFunc func([]byte, []byte, uint32, uint32, uint8, uint32) []byte

type argon2idSessionLoginAuthenticator struct {
	revision string
	accounts map[string]argon2idSessionLoginAccount
	dummy    argon2idPasswordHash
	slots    chan struct{}
	derive   argon2idDeriveFunc
}

func loadSessionAccountAuthenticator(path string) (sessionAccountAuthenticator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read session login config %q: %w", path, err)
	}
	var header struct {
		SchemaVersion uint16 `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("%w: decode schema_version: %v", errSessionLoginConfig, err)
	}
	switch header.SchemaVersion {
	case sessionLoginSchemaVersion:
		return loadStaticSessionLoginAuthenticator(path)
	case sessionPasswordSchemaVersion:
		return loadArgon2idSessionLoginAuthenticatorData(data)
	case sessionDurableAccountLegacySchemaVersion, sessionDurableAccountSchemaVersion:
		return loadDurableSessionLoginAuthenticator(path)
	default:
		return nil, fmt.Errorf("%w: unsupported schema_version %d", errSessionLoginConfig, header.SchemaVersion)
	}
}

func loadArgon2idSessionLoginAuthenticatorData(data []byte) (*argon2idSessionLoginAuthenticator, error) {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var definition sessionPasswordLoginDefinition
	if err := decoder.Decode(&definition); err != nil {
		return nil, fmt.Errorf("%w: decode password accounts: %v", errSessionLoginConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("%w: trailing JSON value", errSessionLoginConfig)
		}
		return nil, fmt.Errorf("%w: trailing data: %v", errSessionLoginConfig, err)
	}
	return newArgon2idSessionLoginAuthenticator(definition)
}

func newArgon2idSessionLoginAuthenticator(definition sessionPasswordLoginDefinition) (*argon2idSessionLoginAuthenticator, error) {
	if definition.SchemaVersion != sessionPasswordSchemaVersion || strings.TrimSpace(definition.Revision) == "" || len(definition.Accounts) == 0 {
		return nil, errSessionLoginConfig
	}
	accounts := make(map[string]argon2idSessionLoginAccount, len(definition.Accounts))
	var dummy argon2idPasswordHash
	for index, item := range definition.Accounts {
		loginID := strings.TrimSpace(item.LoginID)
		if loginID == "" || loginID != item.LoginID || len(loginID) > sessionLoginMaxIDBytes {
			return nil, fmt.Errorf("%w: account[%d] login_id must be 1..%d trimmed bytes", errSessionLoginConfig, index, sessionLoginMaxIDBytes)
		}
		if _, exists := accounts[loginID]; exists {
			return nil, fmt.Errorf("%w: duplicate login_id %q", errSessionLoginConfig, loginID)
		}
		password, err := parseArgon2idPasswordHash(item.PasswordArgon2ID)
		if err != nil {
			return nil, fmt.Errorf("%w: account[%d] password_argon2id: %v", errSessionLoginConfig, index, err)
		}
		if index == 0 {
			dummy = cloneArgon2idPasswordHash(password)
		} else if !sameArgon2idPolicy(dummy, password) {
			return nil, fmt.Errorf("%w: account[%d] Argon2id cost policy differs from account[0]", errSessionLoginConfig, index)
		}
		binding, err := characteridentity.NewTrusted(item.CharacterID)
		if err != nil {
			return nil, fmt.Errorf("%w: account[%d] character_id: %v", errSessionLoginConfig, index, err)
		}
		accounts[loginID] = argon2idSessionLoginAccount{
			password: password,
			grant: sessioncredential.Grant{
				Identity:            binding,
				AllowActiveTakeover: item.AllowActiveTakeover,
			},
		}
	}
	return &argon2idSessionLoginAuthenticator{
		revision: definition.Revision,
		accounts: accounts,
		dummy:    dummy,
		slots:    make(chan struct{}, sessionPasswordMaxConcurrentChecks),
		derive:   argon2.IDKey,
	}, nil
}

func (a *argon2idSessionLoginAuthenticator) Revision() string {
	if a == nil {
		return ""
	}
	return a.revision
}

func (a *argon2idSessionLoginAuthenticator) Method() string {
	return "argon2id-password"
}

func (a *argon2idSessionLoginAuthenticator) Authenticate(ctx context.Context, loginID string, secret []byte) (sessioncredential.Grant, error) {
	if a == nil || len(a.accounts) == 0 || a.derive == nil || loginID == "" || len(loginID) > sessionLoginMaxIDBytes || len(secret) == 0 || len(secret) > sessionLoginMaxSecretBytes {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	case <-ctx.Done():
		return sessioncredential.Grant{}, ctx.Err()
	}

	account, exists := a.accounts[loginID]
	verifier := a.dummy
	if exists {
		verifier = account.password
	}
	derived := a.derive(secret, verifier.salt, verifier.time, verifier.memory, verifier.threads, uint32(len(verifier.digest)))
	matched := subtle.ConstantTimeCompare(derived, verifier.digest) == 1
	clear(derived)

	select {
	case <-ctx.Done():
		return sessioncredential.Grant{}, ctx.Err()
	default:
	}
	if !exists || !matched || !account.grant.Valid() {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	return account.grant, nil
}

func parseArgon2idPasswordHash(encoded string) (argon2idPasswordHash, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(sessionPasswordArgon2Version) {
		return argon2idPasswordHash{}, fmt.Errorf("must use $argon2id$v=19$m=...,t=...,p=...$salt$digest")
	}
	memory, timeCost, threads, err := parseArgon2idParameters(parts[3])
	if err != nil {
		return argon2idPasswordHash{}, err
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) < sessionPasswordArgon2SaltMinBytes || len(salt) > sessionPasswordArgon2SaltMaxBytes {
		return argon2idPasswordHash{}, fmt.Errorf("salt must be %d..%d bytes of unpadded base64", sessionPasswordArgon2SaltMinBytes, sessionPasswordArgon2SaltMaxBytes)
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(digest) != sessionPasswordArgon2DigestBytes {
		return argon2idPasswordHash{}, fmt.Errorf("digest must be %d bytes of unpadded base64", sessionPasswordArgon2DigestBytes)
	}
	return argon2idPasswordHash{memory: memory, time: timeCost, threads: threads, salt: salt, digest: digest}, nil
}

func parseArgon2idParameters(encoded string) (uint32, uint32, uint8, error) {
	var memory uint64
	var timeCost uint64
	var threads uint64
	seen := map[string]bool{}
	fields := strings.Split(encoded, ",")
	if len(fields) != 3 {
		return 0, 0, 0, fmt.Errorf("Argon2id parameters must contain exactly m,t,p")
	}
	for _, field := range fields {
		pair := strings.SplitN(field, "=", 2)
		if len(pair) != 2 || seen[pair[0]] {
			return 0, 0, 0, fmt.Errorf("invalid Argon2id parameter %q", field)
		}
		seen[pair[0]] = true
		value, err := strconv.ParseUint(pair[1], 10, 32)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("invalid Argon2id parameter %q", field)
		}
		switch pair[0] {
		case "m":
			memory = value
		case "t":
			timeCost = value
		case "p":
			threads = value
		default:
			return 0, 0, 0, fmt.Errorf("unknown Argon2id parameter %q", pair[0])
		}
	}
	if memory < uint64(sessionPasswordArgon2MinMemoryKiB) || memory > uint64(sessionPasswordArgon2MaxMemoryKiB) {
		return 0, 0, 0, fmt.Errorf("m must be between %d and %d KiB", sessionPasswordArgon2MinMemoryKiB, sessionPasswordArgon2MaxMemoryKiB)
	}
	if timeCost < uint64(sessionPasswordArgon2MinTime) || timeCost > uint64(sessionPasswordArgon2MaxTime) {
		return 0, 0, 0, fmt.Errorf("t must be between %d and %d", sessionPasswordArgon2MinTime, sessionPasswordArgon2MaxTime)
	}
	if threads < uint64(sessionPasswordArgon2MinThreads) || threads > uint64(sessionPasswordArgon2MaxThreads) {
		return 0, 0, 0, fmt.Errorf("p must be between %d and %d", sessionPasswordArgon2MinThreads, sessionPasswordArgon2MaxThreads)
	}
	return uint32(memory), uint32(timeCost), uint8(threads), nil
}

func sameArgon2idPolicy(left, right argon2idPasswordHash) bool {
	return left.memory == right.memory && left.time == right.time && left.threads == right.threads && len(left.digest) == len(right.digest)
}

func cloneArgon2idPasswordHash(source argon2idPasswordHash) argon2idPasswordHash {
	return argon2idPasswordHash{
		memory:  source.memory,
		time:    source.time,
		threads: source.threads,
		salt:    append([]byte(nil), source.salt...),
		digest:  append([]byte(nil), source.digest...),
	}
}
