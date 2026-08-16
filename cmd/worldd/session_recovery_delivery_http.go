package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

const (
	sessionRecoveryHTTPAdapterMethod                = "https-json-v1"
	sessionRecoveryHTTPPayloadSchemaVersion  uint16 = 1
	sessionRecoveryHTTPMinTimeout                   = 100 * time.Millisecond
	sessionRecoveryHTTPMaxTimeout                   = 2 * time.Second
	sessionRecoveryHTTPDefaultTimeout               = time.Second
	sessionRecoveryHTTPMaxAttempts                  = 3
	sessionRecoveryHTTPDefaultMaxAttempts           = 3
	sessionRecoveryHTTPMaxRetryBackoff              = 500 * time.Millisecond
	sessionRecoveryHTTPDefaultRetryBackoff          = 200 * time.Millisecond
	sessionRecoveryHTTPMaxCredentialBytes           = 4096
	sessionRecoveryHTTPMaxResponseDrainBytes        = 4096
)

type httpRecoveryDeliveryAdapter struct {
	endpoint       *url.URL
	revision       string
	credential     []byte
	client         *http.Client
	requestTimeout time.Duration
	maxAttempts    int
	retryBackoff   time.Duration
	logf           func(string, ...any)
}

type httpRecoveryDeliveryPayload struct {
	SchemaVersion uint16 `json:"schema_version"`
	DeliveryID    string `json:"delivery_id"`
	Destination   string `json:"destination"`
	Proof         string `json:"proof"`
	ExpiresAt     string `json:"expires_at"`
}

func newHTTPRecoveryDeliveryAdapter(endpoint, credentialFile, caFile, revision string, requestTimeout time.Duration, maxAttempts int, retryBackoff time.Duration) (*httpRecoveryDeliveryAdapter, error) {
	parsed, err := validateRecoveryDeliveryEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	rawRevision := revision
	revision = strings.TrimSpace(revision)
	if revision == "" || revision != rawRevision {
		return nil, fmt.Errorf("%w: https recovery delivery revision must be non-empty", errSessionLoginConfig)
	}
	if requestTimeout == 0 {
		requestTimeout = sessionRecoveryHTTPDefaultTimeout
	}
	if requestTimeout < sessionRecoveryHTTPMinTimeout || requestTimeout > sessionRecoveryHTTPMaxTimeout {
		return nil, fmt.Errorf("%w: https recovery delivery request_timeout must be between %s and %s", errSessionLoginConfig, sessionRecoveryHTTPMinTimeout, sessionRecoveryHTTPMaxTimeout)
	}
	if maxAttempts == 0 {
		maxAttempts = sessionRecoveryHTTPDefaultMaxAttempts
	}
	if maxAttempts < 1 || maxAttempts > sessionRecoveryHTTPMaxAttempts {
		return nil, fmt.Errorf("%w: https recovery delivery max_attempts must be between 1 and %d", errSessionLoginConfig, sessionRecoveryHTTPMaxAttempts)
	}
	if retryBackoff < 0 || retryBackoff > sessionRecoveryHTTPMaxRetryBackoff {
		return nil, fmt.Errorf("%w: https recovery delivery retry_backoff must be between 0 and %s", errSessionLoginConfig, sessionRecoveryHTTPMaxRetryBackoff)
	}
	credential, err := loadRecoveryDeliveryCredential(credentialFile)
	if err != nil {
		return nil, err
	}
	roots, err := loadRecoveryDeliveryRoots(caFile)
	if err != nil {
		clear(credential)
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}
	client := &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &httpRecoveryDeliveryAdapter{
		endpoint:       parsed,
		revision:       revision,
		credential:     credential,
		client:         client,
		requestTimeout: requestTimeout,
		maxAttempts:    maxAttempts,
		retryBackoff:   retryBackoff,
		logf:           log.Printf,
	}, nil
}

func validateRecoveryDeliveryEndpoint(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: https recovery delivery endpoint must be non-empty", errSessionLoginConfig)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("%w: https recovery delivery endpoint must be an absolute https URL without userinfo or fragment", errSessionLoginConfig)
	}
	return parsed, nil
}

func loadRecoveryDeliveryCredential(path string) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%w: https recovery delivery credential_file must be non-empty", errSessionLoginConfig)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("%w: stat https recovery delivery credential file: %v", errSessionLoginConfig, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: https recovery delivery credential file must be a regular owner-only file", errSessionLoginConfig)
	}
	if info.Size() <= 0 || info.Size() > sessionRecoveryHTTPMaxCredentialBytes+2 {
		return nil, fmt.Errorf("%w: https recovery delivery credential file size is invalid", errSessionLoginConfig)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read https recovery delivery credential file: %v", errSessionLoginConfig, err)
	}
	trimmed := bytes.TrimSuffix(data, []byte("\n"))
	trimmed = bytes.TrimSuffix(trimmed, []byte("\r"))
	if len(trimmed) < 16 || len(trimmed) > sessionRecoveryHTTPMaxCredentialBytes || !validRecoveryDeliveryCredential(trimmed) {
		clear(data)
		return nil, fmt.Errorf("%w: https recovery delivery credential must be 16..%d visible ASCII bytes", errSessionLoginConfig, sessionRecoveryHTTPMaxCredentialBytes)
	}
	credential := append([]byte(nil), trimmed...)
	clear(data)
	return credential, nil
}

func validRecoveryDeliveryCredential(value []byte) bool {
	for _, b := range value {
		if b < 0x21 || b > 0x7e {
			return false
		}
	}
	return true
}

func loadRecoveryDeliveryRoots(path string) (*x509.CertPool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read https recovery delivery ca_file: %v", errSessionLoginConfig, err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if ok := roots.AppendCertsFromPEM(data); !ok {
		return nil, fmt.Errorf("%w: https recovery delivery ca_file contains no certificates", errSessionLoginConfig)
	}
	return roots, nil
}

func (a *httpRecoveryDeliveryAdapter) Method() string { return sessionRecoveryHTTPAdapterMethod }
func (a *httpRecoveryDeliveryAdapter) Revision() string {
	if a == nil {
		return ""
	}
	return a.revision
}

func (a *httpRecoveryDeliveryAdapter) Deliver(ctx context.Context, delivery accountrecovery.Delivery) error {
	if a == nil || a.endpoint == nil || a.client == nil || len(a.credential) == 0 || !delivery.Valid() || len(delivery.Proof) == 0 {
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
	deliveryID := recoveryDeliveryID(delivery.RequestID)
	payload, err := json.Marshal(httpRecoveryDeliveryPayload{
		SchemaVersion: sessionRecoveryHTTPPayloadSchemaVersion,
		DeliveryID:    deliveryID,
		Destination:   delivery.Destination,
		Proof:         string(delivery.Proof),
		ExpiresAt:     delivery.ExpiresAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return accountrecovery.ErrDeliveryPermanent
	}
	defer clear(payload)

	lastStatus := 0
	for attempt := 1; attempt <= a.maxAttempts; attempt++ {
		status, transient, err := a.deliverAttempt(ctx, deliveryID, payload)
		lastStatus = status
		if err == nil && !transient {
			a.logOutcome("success", attempt, status)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !errors.Is(err, accountrecovery.ErrDeliveryTransient) {
			a.logOutcome("permanent", attempt, status)
			return err
		}
		if !transient {
			a.logOutcome("permanent", attempt, status)
			if err != nil {
				return err
			}
			return accountrecovery.ErrDeliveryPermanent
		}
		if attempt == a.maxAttempts {
			a.logOutcome("transient", attempt, status)
			return accountrecovery.ErrDeliveryTransient
		}
		delay := a.retryDelay(attempt)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	a.logOutcome("transient", a.maxAttempts, lastStatus)
	return accountrecovery.ErrDeliveryTransient
}

func (a *httpRecoveryDeliveryAdapter) deliverAttempt(parent context.Context, deliveryID string, payload []byte) (int, bool, error) {
	attemptCtx, cancel := context.WithTimeout(parent, a.requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, a.endpoint.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, false, accountrecovery.ErrDeliveryPermanent
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+string(a.credential))
	request.Header.Set("Idempotency-Key", deliveryID)
	request.Header.Set("X-Astrahold-Delivery-ID", deliveryID)
	request.Header.Set("User-Agent", "Astrahold-Recovery-Delivery/1")
	response, err := a.client.Do(request)
	if err != nil {
		if parent.Err() != nil {
			return 0, false, parent.Err()
		}
		return 0, true, accountrecovery.ErrDeliveryTransient
	}
	_, _ = io.CopyN(io.Discard, response.Body, sessionRecoveryHTTPMaxResponseDrainBytes+1)
	_ = response.Body.Close()
	status := response.StatusCode
	if status >= 200 && status <= 299 {
		return status, false, nil
	}
	if status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500 {
		return status, true, accountrecovery.ErrDeliveryTransient
	}
	return status, false, accountrecovery.ErrDeliveryPermanent
}

func (a *httpRecoveryDeliveryAdapter) retryDelay(attempt int) time.Duration {
	if a == nil || a.retryBackoff <= 0 || attempt <= 0 {
		return 0
	}
	delay := a.retryBackoff
	for i := 1; i < attempt; i++ {
		if delay >= sessionRecoveryHTTPMaxRetryBackoff/2 {
			return sessionRecoveryHTTPMaxRetryBackoff
		}
		delay *= 2
	}
	if delay > sessionRecoveryHTTPMaxRetryBackoff {
		return sessionRecoveryHTTPMaxRetryBackoff
	}
	return delay
}

func (a *httpRecoveryDeliveryAdapter) logOutcome(outcome string, attempts, status int) {
	if a == nil || a.logf == nil {
		return
	}
	class := "transport"
	if status > 0 {
		class = fmt.Sprintf("%dxx", status/100)
	}
	a.logf("recovery delivery: adapter=%s revision=%s outcome=%s attempts=%d status_class=%s", a.Method(), a.Revision(), outcome, attempts, class)
}

func recoveryDeliveryID(requestID string) string {
	digest := sha256.Sum256([]byte("astrahold-recovery-delivery-id-v1\x00" + requestID))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

var _ accountrecovery.DeliveryAdapter = (*httpRecoveryDeliveryAdapter)(nil)
