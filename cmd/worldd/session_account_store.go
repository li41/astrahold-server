package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"github.com/li41/astrahold-server/internal/accountstore"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/sessioncredential"
)

const (
	sessionDurableAccountLegacySchemaVersion = accountstore.LegacySchemaVersion
	sessionDurableAccountSchemaVersion       = accountstore.SchemaVersion
)

type durableSessionLoginAccount struct {
	password argon2idPasswordHash
	grant    sessioncredential.Grant
	disabled bool
}

type durableSessionLoginAuthenticator struct {
	storeRevision uint64
	accounts      map[string]durableSessionLoginAccount
	generations   map[string]string
	dummy         argon2idPasswordHash
	slots         chan struct{}
	derive        argon2idDeriveFunc
}

func loadDurableSessionLoginAuthenticator(path string) (*durableSessionLoginAuthenticator, error) {
	definition, err := accountstore.Load(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errSessionLoginConfig, err)
	}
	return newDurableSessionLoginAuthenticator(definition)
}

func newDurableSessionLoginAuthenticator(definition accountstore.Definition) (*durableSessionLoginAuthenticator, error) {
	if (definition.SchemaVersion != sessionDurableAccountLegacySchemaVersion && definition.SchemaVersion != sessionDurableAccountSchemaVersion) || definition.Revision == 0 || len(definition.Accounts) == 0 {
		return nil, errSessionLoginConfig
	}
	accounts := make(map[string]durableSessionLoginAccount, len(definition.Accounts))
	generations := make(map[string]string, len(definition.Accounts))
	var dummy argon2idPasswordHash
	for index, item := range definition.Accounts {
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
		generation := durableAccountGeneration(item)
		grant := sessioncredential.Grant{
			Identity:                 binding,
			AllowActiveTakeover:      item.AllowActiveTakeover,
			AuthenticationSubject:    item.AccountID,
			AuthenticationGeneration: generation,
		}
		accounts[item.LoginID] = durableSessionLoginAccount{
			password: password,
			grant:    grant,
			disabled: item.DisabledAt != "",
		}
		if item.DisabledAt == "" {
			generations[item.AccountID] = generation
		}
	}
	return &durableSessionLoginAuthenticator{
		storeRevision: definition.Revision,
		accounts:      accounts,
		generations:   generations,
		dummy:         dummy,
		slots:         make(chan struct{}, sessionPasswordMaxConcurrentChecks),
		derive:        argon2.IDKey,
	}, nil
}

func (a *durableSessionLoginAuthenticator) Revision() string {
	if a == nil {
		return ""
	}
	return fmt.Sprintf("durable-%d", a.storeRevision)
}

func (a *durableSessionLoginAuthenticator) Method() string {
	return "argon2id-durable"
}

func (a *durableSessionLoginAuthenticator) StoreRevision() uint64 {
	if a == nil {
		return 0
	}
	return a.storeRevision
}

func (a *durableSessionLoginAuthenticator) GenerationActive(subject, generation string) bool {
	if a == nil || subject == "" || generation == "" {
		return false
	}
	return a.generations[subject] == generation
}

func (a *durableSessionLoginAuthenticator) Authenticate(ctx context.Context, loginID string, secret []byte) (sessioncredential.Grant, error) {
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
	matched := subtleCompareAndClear(derived, verifier.digest)

	select {
	case <-ctx.Done():
		return sessioncredential.Grant{}, ctx.Err()
	default:
	}
	if !exists || !matched || account.disabled || !account.grant.Valid() {
		return sessioncredential.Grant{}, errSessionLoginRejected
	}
	return account.grant, nil
}

func subtleCompareAndClear(derived, expected []byte) bool {
	matched := len(derived) == len(expected)
	var diff byte
	if matched {
		for index := range derived {
			diff |= derived[index] ^ expected[index]
		}
	}
	clear(derived)
	return matched && diff == 0
}

func durableAccountGeneration(account accountstore.Account) string {
	hasher := sha256.New()
	writeString := func(value string) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
	writeString(account.AccountID)
	writeString(account.LoginID)
	writeString(account.PasswordArgon2ID)
	writeString(account.CharacterID)
	writeString(fmt.Sprintf("%d", account.CredentialVersion))
	if account.AllowActiveTakeover {
		writeString("takeover:1")
	} else {
		writeString("takeover:0")
	}
	return "account-v1:" + hex.EncodeToString(hasher.Sum(nil))
}

type sessionAccountReloadResult struct {
	PreviousRevision string
	Revision         string
	RemovedBearers   int
	RetiredPeers     int
}

func (r *sessionLoginRuntime) reloadDurableAccounts(now time.Time) (sessionAccountReloadResult, error) {
	if r == nil || r.accountAuth == nil || r.provider == nil || strings.TrimSpace(r.accountPath) == "" || r.replaceScopes == nil {
		return sessionAccountReloadResult{}, errSessionLoginRuntimeUnavailable
	}
	previousRevision, previousStoreRevision, ok := r.accountAuth.reloadMetadata()
	if !ok {
		return sessionAccountReloadResult{}, fmt.Errorf("%w: account provider is restart-only", errSessionLoginConfig)
	}
	nextAuthenticator, err := loadSessionAccountAuthenticator(r.accountPath)
	if err != nil {
		return sessionAccountReloadResult{}, err
	}
	nextDurable, ok := nextAuthenticator.(*durableSessionLoginAuthenticator)
	if !ok {
		return sessionAccountReloadResult{}, fmt.Errorf("%w: reload requires durable schema_version %d or %d", errSessionLoginConfig, sessionDurableAccountLegacySchemaVersion, sessionDurableAccountSchemaVersion)
	}
	if nextDurable.StoreRevision() <= previousStoreRevision {
		return sessionAccountReloadResult{}, fmt.Errorf("%w: durable account revision must advance: previous=%d next=%d", errSessionLoginConfig, previousStoreRevision, nextDurable.StoreRevision())
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	_, currentStoreRevision, ok := r.accountAuth.reloadMetadata()
	if !ok || currentStoreRevision != previousStoreRevision {
		return sessionAccountReloadResult{}, fmt.Errorf("%w: account provider changed during reload", errSessionLoginConfig)
	}
	current := r.provider.snapshot()
	nextRevision := r.revision + 1
	nextProvider := cloneIssuedSessionCredentialProvider(current, nextRevision, r.now)
	pruneIssuedSessionCredentials(nextProvider, now)
	removed := 0
	for digest, entry := range nextProvider.credentials {
		subject := entry.grant.AuthenticationSubject
		generation := entry.grant.AuthenticationGeneration
		if subject == "" || generation == "" || nextDurable.GenerationActive(subject, generation) {
			continue
		}
		delete(nextProvider.credentials, digest)
		if entry.credentialID != "" {
			delete(nextProvider.credentialsByID, entry.credentialID)
		}
		removed++
	}

	// Fence removed account generations before publishing the replacement
	// account authenticator. In-flight old-password authentication may finish,
	// but Issue re-validates the account generation under r.mu and therefore
	// cannot mint a bearer after this reload commits.
	retired := r.replaceScopes(activeTrustedCharacterAuthenticationScopes(nextProvider, now))
	r.provider.replace(nextProvider)
	r.revision = nextRevision
	r.accountAuth.replace(nextAuthenticator)
	return sessionAccountReloadResult{
		PreviousRevision: previousRevision,
		Revision:         nextAuthenticator.Revision(),
		RemovedBearers:   removed,
		RetiredPeers:     retired,
	}, nil
}
