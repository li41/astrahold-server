package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

const (
	sessionRecoveryOutboxMethod                            = "durable-outbox-v1"
	sessionRecoveryOutboxRecordSchemaVersion        uint16 = 1
	sessionRecoveryOutboxDefaultMaxRecords                 = 1024
	sessionRecoveryOutboxMaxRecords                        = 4096
	sessionRecoveryOutboxDefaultMaxDeliveryAttempts        = 8
	sessionRecoveryOutboxMaxDeliveryAttempts               = 64
	sessionRecoveryOutboxDefaultRetryMin                   = time.Second
	sessionRecoveryOutboxDefaultRetryMax                   = 30 * time.Second
	sessionRecoveryOutboxMinRetry                          = 100 * time.Millisecond
	sessionRecoveryOutboxMaxRetry                          = 5 * time.Minute
	sessionRecoveryOutboxMaxRecordBytes                    = 8192
	sessionRecoveryOutboxWorkerBatch                       = 32
)

const (
	sessionRecoveryOutboxStatePending   = "pending"
	sessionRecoveryOutboxStateDelivered = "delivered"
	sessionRecoveryOutboxStateFailed    = "failed"
)

var (
	sessionRecoveryOutboxDir = flag.String(
		"session-recovery-outbox-dir",
		"",
		"Optional owner-only directory for durable schema-v2 HTTPS recovery delivery replay and challenge restart recovery",
	)
	sessionRecoveryOutboxMaxRecordsFlag = flag.Int(
		"session-recovery-outbox-max-records",
		sessionRecoveryOutboxDefaultMaxRecords,
		"Maximum live durable recovery outbox/challenge records (1..4096)",
	)
	sessionRecoveryOutboxMaxDeliveryAttemptsFlag = flag.Int(
		"session-recovery-outbox-max-delivery-attempts",
		sessionRecoveryOutboxDefaultMaxDeliveryAttempts,
		"Maximum completed durable delivery retry cycles before a challenge becomes non-authorizing (1..64)",
	)
	sessionRecoveryOutboxRetryMinFlag = flag.Duration(
		"session-recovery-outbox-retry-min",
		sessionRecoveryOutboxDefaultRetryMin,
		"Initial durable recovery delivery retry interval (100ms..5m)",
	)
	sessionRecoveryOutboxRetryMaxFlag = flag.Duration(
		"session-recovery-outbox-retry-max",
		sessionRecoveryOutboxDefaultRetryMax,
		"Maximum durable recovery delivery retry interval (>= retry-min, <=5m)",
	)
)

type sessionRecoveryOutboxConfig struct {
	Dir                 string
	MaxRecords          int
	MaxDeliveryAttempts int
	RetryMin            time.Duration
	RetryMax            time.Duration
}

type sessionRecoveryOutboxChallengeSnapshot struct {
	RequestID            string
	LoginID              string
	AccountID            string
	CredentialVersion    uint64
	VerifierSHA256       string
	Active               bool
	ExpiresAt            time.Time
	VerificationAttempts int
}

type sessionRecoveryOutboxRecord struct {
	SchemaVersion        uint16 `json:"schema_version"`
	DeliveryID           string `json:"delivery_id"`
	RequestID            string `json:"request_id"`
	LoginID              string `json:"login_id"`
	AccountID            string `json:"account_id"`
	CredentialVersion    uint64 `json:"credential_version"`
	VerifierSHA256       string `json:"verifier_sha256"`
	Active               bool   `json:"active"`
	ExpiresAt            string `json:"expires_at"`
	VerificationAttempts int    `json:"verification_attempts"`
	DeliveryState        string `json:"delivery_state"`
	Destination          string `json:"destination,omitempty"`
	Proof                string `json:"proof,omitempty"`
	DeliveryAttempts     int    `json:"delivery_attempts"`
	NextAttemptAt        string `json:"next_attempt_at,omitempty"`
}

type sessionRecoverySharedDelivery interface {
	sharedRecoveryDelivery()
}

type sessionRecoveryDurableOutbox struct {
	dir                 string
	maxRecords          int
	maxDeliveryAttempts int
	retryMin            time.Duration
	retryMax            time.Duration
	now                 func() time.Time
	logf                func(string, ...any)

	transportMu sync.RWMutex
	transport   accountrecovery.DeliveryAdapter
	provider    *staticSessionRecoveryProvider

	mu      sync.Mutex
	records map[string]sessionRecoveryOutboxRecord
	owners  map[string]*staticSessionRecoveryProvider
	ready   map[string]bool

	wake      chan struct{}
	stop      chan struct{}
	done      chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
}

func sessionRecoveryOutboxConfigFromFlags() (sessionRecoveryOutboxConfig, bool, error) {
	dir := strings.TrimSpace(*sessionRecoveryOutboxDir)
	if dir == "" {
		return sessionRecoveryOutboxConfig{}, false, nil
	}
	config := sessionRecoveryOutboxConfig{
		Dir:                 dir,
		MaxRecords:          *sessionRecoveryOutboxMaxRecordsFlag,
		MaxDeliveryAttempts: *sessionRecoveryOutboxMaxDeliveryAttemptsFlag,
		RetryMin:            *sessionRecoveryOutboxRetryMinFlag,
		RetryMax:            *sessionRecoveryOutboxRetryMaxFlag,
	}
	if config.MaxRecords < 1 || config.MaxRecords > sessionRecoveryOutboxMaxRecords {
		return sessionRecoveryOutboxConfig{}, true, fmt.Errorf("%w: session-recovery-outbox-max-records must be between 1 and %d", errSessionLoginConfig, sessionRecoveryOutboxMaxRecords)
	}
	if config.MaxDeliveryAttempts < 1 || config.MaxDeliveryAttempts > sessionRecoveryOutboxMaxDeliveryAttempts {
		return sessionRecoveryOutboxConfig{}, true, fmt.Errorf("%w: session-recovery-outbox-max-delivery-attempts must be between 1 and %d", errSessionLoginConfig, sessionRecoveryOutboxMaxDeliveryAttempts)
	}
	if config.RetryMin < sessionRecoveryOutboxMinRetry || config.RetryMin > sessionRecoveryOutboxMaxRetry {
		return sessionRecoveryOutboxConfig{}, true, fmt.Errorf("%w: session-recovery-outbox-retry-min must be between %s and %s", errSessionLoginConfig, sessionRecoveryOutboxMinRetry, sessionRecoveryOutboxMaxRetry)
	}
	if config.RetryMax < config.RetryMin || config.RetryMax > sessionRecoveryOutboxMaxRetry {
		return sessionRecoveryOutboxConfig{}, true, fmt.Errorf("%w: session-recovery-outbox-retry-max must be between retry-min and %s", errSessionLoginConfig, sessionRecoveryOutboxMaxRetry)
	}
	return config, true, nil
}

func configureSessionRecoveryDurableOutbox(provider *staticSessionRecoveryProvider) error {
	config, requested, err := sessionRecoveryOutboxConfigFromFlags()
	if err != nil || !requested {
		return err
	}
	if provider == nil || provider.delivery == nil || provider.delivery.Method() != sessionRecoveryHTTPAdapterMethod {
		return fmt.Errorf("%w: durable recovery outbox requires schema-v2 %s delivery", errSessionLoginConfig, sessionRecoveryHTTPAdapterMethod)
	}
	outbox, err := newSessionRecoveryDurableOutbox(config, provider, provider.delivery, time.Now)
	if err != nil {
		return err
	}
	provider.delivery = outbox
	outbox.Start()
	return nil
}

func newSessionRecoveryDurableOutbox(
	config sessionRecoveryOutboxConfig,
	provider *staticSessionRecoveryProvider,
	transport accountrecovery.DeliveryAdapter,
	now func() time.Time,
) (*sessionRecoveryDurableOutbox, error) {
	if provider == nil || provider.delivery == nil || transport == nil || transport.Method() != sessionRecoveryHTTPAdapterMethod {
		return nil, fmt.Errorf("%w: durable recovery outbox requires an HTTPS delivery provider", errSessionLoginConfig)
	}
	if now == nil {
		now = time.Now
	}
	if err := validateSessionRecoveryOutboxRoot(config.Dir); err != nil {
		return nil, err
	}
	outbox := &sessionRecoveryDurableOutbox{
		dir:                 config.Dir,
		maxRecords:          config.MaxRecords,
		maxDeliveryAttempts: config.MaxDeliveryAttempts,
		retryMin:            config.RetryMin,
		retryMax:            config.RetryMax,
		now:                 now,
		logf:                log.Printf,
		transport:           transport,
		provider:            provider,
		records:             make(map[string]sessionRecoveryOutboxRecord),
		owners:              make(map[string]*staticSessionRecoveryProvider),
		ready:               make(map[string]bool),
		wake:                make(chan struct{}, 1),
		stop:                make(chan struct{}),
		done:                make(chan struct{}),
	}
	if err := outbox.loadAndRestore(provider); err != nil {
		return nil, err
	}
	return outbox, nil
}

func validateSessionRecoveryOutboxRoot(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("%w: stat recovery outbox directory: %v", errSessionLoginConfig, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 || info.Mode().Perm()&0o700 != 0o700 {
		return fmt.Errorf("%w: recovery outbox directory must be a real owner-only readable/writable/searchable directory", errSessionLoginConfig)
	}
	return nil
}

func (o *sessionRecoveryDurableOutbox) Method() string {
	if o == nil {
		return ""
	}
	o.transportMu.RLock()
	defer o.transportMu.RUnlock()
	if o.transport == nil {
		return ""
	}
	return o.transport.Method()
}

func (o *sessionRecoveryDurableOutbox) Revision() string {
	if o == nil {
		return ""
	}
	o.transportMu.RLock()
	defer o.transportMu.RUnlock()
	if o.transport == nil {
		return ""
	}
	return o.transport.Revision()
}

func (*sessionRecoveryDurableOutbox) sharedRecoveryDelivery() {}

func (o *sessionRecoveryDurableOutbox) Deliver(ctx context.Context, delivery accountrecovery.Delivery) error {
	if o == nil || !delivery.Valid() {
		return accountrecovery.ErrDeliveryPermanent
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	o.transportMu.RLock()
	provider := o.provider
	o.transportMu.RUnlock()
	snapshot, ok := provider.sessionRecoveryOutboxSnapshot(delivery.RequestID)
	digest := sha256.Sum256(delivery.Proof)
	if !ok || snapshot.ExpiresAt.UTC() != delivery.ExpiresAt.UTC() || snapshot.VerifierSHA256 != hex.EncodeToString(digest[:]) {
		return accountrecovery.ErrDeliveryPermanent
	}

	now := o.now().UTC()
	if !now.Before(delivery.ExpiresAt.UTC()) {
		return accountrecovery.ErrDeliveryPermanent
	}
	record := sessionRecoveryOutboxRecord{
		SchemaVersion:        sessionRecoveryOutboxRecordSchemaVersion,
		DeliveryID:           recoveryDeliveryID(delivery.RequestID),
		RequestID:            delivery.RequestID,
		LoginID:              snapshot.LoginID,
		AccountID:            snapshot.AccountID,
		CredentialVersion:    snapshot.CredentialVersion,
		VerifierSHA256:       snapshot.VerifierSHA256,
		Active:               true,
		ExpiresAt:            delivery.ExpiresAt.UTC().Format(time.RFC3339Nano),
		VerificationAttempts: snapshot.VerificationAttempts,
		DeliveryState:        sessionRecoveryOutboxStatePending,
		Destination:          delivery.Destination,
		Proof:                string(delivery.Proof),
		NextAttemptAt:        now.Format(time.RFC3339Nano),
	}
	if err := o.validateRecord(record); err != nil {
		return accountrecovery.ErrDeliveryPermanent
	}

	o.mu.Lock()
	o.pruneExpiredLocked(now)
	if existing, exists := o.records[record.RequestID]; exists {
		if sameSessionRecoveryOutboxDelivery(existing, record) {
			o.owners[record.RequestID] = provider
			o.mu.Unlock()
			o.signalWake()
			return nil
		}
		o.mu.Unlock()
		return accountrecovery.ErrDeliveryPermanent
	}
	if len(o.records) >= o.maxRecords {
		pending := o.pendingCountLocked()
		o.mu.Unlock()
		o.logf("recovery outbox: outcome=backpressure records=%d pending=%d max_records=%d", o.recordCount(), pending, o.maxRecords)
		return accountrecovery.ErrDeliveryTransient
	}
	if err := o.writeRecordLocked(record); err != nil {
		o.mu.Unlock()
		return fmt.Errorf("%w: durable recovery outbox enqueue: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	o.records[record.RequestID] = record
	o.owners[record.RequestID] = provider
	o.ready[record.RequestID] = false
	records := len(o.records)
	pending := o.pendingCountLocked()
	o.mu.Unlock()
	o.logf("recovery outbox: outcome=enqueued records=%d pending=%d", records, pending)
	return nil
}

func (o *sessionRecoveryDurableOutbox) PublishChallenge(requestID string) {
	if o == nil || requestID == "" {
		return
	}
	o.mu.Lock()
	record, exists := o.records[requestID]
	if exists && record.DeliveryState == sessionRecoveryOutboxStatePending {
		o.ready[requestID] = true
	}
	o.mu.Unlock()
	if exists {
		o.signalWake()
	}
}

func (o *sessionRecoveryDurableOutbox) PersistChallenge(provider *staticSessionRecoveryProvider, requestID string) error {
	if o == nil || provider == nil || requestID == "" {
		return nil
	}
	snapshot, exists := provider.sessionRecoveryOutboxSnapshot(requestID)
	if !exists {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	record, exists := o.records[requestID]
	if !exists {
		return nil
	}
	record.VerificationAttempts = snapshot.VerificationAttempts
	if record.DeliveryState != sessionRecoveryOutboxStateFailed {
		record.Active = snapshot.Active
	}
	if err := o.writeRecordLocked(record); err != nil {
		return err
	}
	o.records[requestID] = record
	return nil
}

func (o *sessionRecoveryDurableOutbox) DeleteChallenge(requestID string) error {
	if o == nil || requestID == "" {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.deleteRecordLocked(requestID)
}
