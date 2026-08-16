package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

func (o *sessionRecoveryDurableOutbox) Start() {
	if o == nil {
		return
	}
	o.startOnce.Do(func() {
		go o.run()
		o.signalWake()
	})
}

func (o *sessionRecoveryDurableOutbox) Retire() {
	if o == nil {
		return
	}
	o.stopOnce.Do(func() { close(o.stop) })
	select {
	case <-o.done:
	case <-time.After(3 * time.Second):
	}
	o.transportMu.Lock()
	transport := o.transport
	o.transport = nil
	o.transportMu.Unlock()
	if retirer, ok := transport.(sessionRecoveryDeliveryRetirer); ok {
		retirer.Retire()
	}
}

func (o *sessionRecoveryDurableOutbox) ReplaceTransport(next accountrecovery.DeliveryAdapter, provider *staticSessionRecoveryProvider) error {
	if o == nil || next == nil || provider == nil || next.Method() != sessionRecoveryHTTPAdapterMethod {
		return fmt.Errorf("%w: durable recovery outbox replacement requires %s delivery", errSessionLoginConfig, sessionRecoveryHTTPAdapterMethod)
	}
	o.transportMu.Lock()
	old := o.transport
	o.transport = next
	o.provider = provider
	o.transportMu.Unlock()
	if old != nil && old != next {
		if retirer, ok := old.(sessionRecoveryDeliveryRetirer); ok {
			retirer.Retire()
		}
	}
	o.signalWake()
	return nil
}

func (o *sessionRecoveryDurableOutbox) run() {
	defer close(o.done)
	for {
		o.processDue()
		delay := o.nextDelay()
		timer := time.NewTimer(delay)
		select {
		case <-o.stop:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-o.wake:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
		}
	}
}

func (o *sessionRecoveryDurableOutbox) processDue() {
	now := o.now().UTC()
	o.mu.Lock()
	o.pruneExpiredLocked(now)
	due := make([]string, 0, sessionRecoveryOutboxWorkerBatch)
	for requestID, record := range o.records {
		if record.DeliveryState != sessionRecoveryOutboxStatePending || !o.ready[requestID] {
			continue
		}
		next, err := parseSessionRecoveryOutboxTime(record.NextAttemptAt)
		if err != nil || !next.After(now) {
			due = append(due, requestID)
			if len(due) >= sessionRecoveryOutboxWorkerBatch {
				break
			}
		}
	}
	sort.Strings(due)
	o.mu.Unlock()
	for _, requestID := range due {
		select {
		case <-o.stop:
			return
		default:
		}
		o.attempt(requestID)
	}
}

func (o *sessionRecoveryDurableOutbox) attempt(requestID string) {
	now := o.now().UTC()
	o.mu.Lock()
	record, exists := o.records[requestID]
	if !exists || record.DeliveryState != sessionRecoveryOutboxStatePending {
		o.mu.Unlock()
		return
	}
	expiresAt, err := parseSessionRecoveryOutboxTime(record.ExpiresAt)
	if err != nil || !now.Before(expiresAt) {
		owner := o.owners[requestID]
		_ = o.deleteRecordLocked(requestID)
		o.mu.Unlock()
		if owner != nil {
			owner.Consume(context.Background(), requestID)
		}
		o.logf("recovery outbox: outcome=expired")
		return
	}
	nextAttemptAt, err := parseSessionRecoveryOutboxTime(record.NextAttemptAt)
	if err == nil && nextAttemptAt.After(now) {
		o.mu.Unlock()
		return
	}
	delivery := accountrecovery.Delivery{
		RequestID:   record.RequestID,
		Destination: record.Destination,
		Proof:       []byte(record.Proof),
		ExpiresAt:   expiresAt,
	}
	o.mu.Unlock()
	defer clear(delivery.Proof)

	o.transportMu.RLock()
	transport := o.transport
	if transport == nil {
		o.transportMu.RUnlock()
		return
	}
	err = transport.Deliver(context.Background(), delivery)
	o.transportMu.RUnlock()

	now = o.now().UTC()
	o.mu.Lock()
	record, exists = o.records[requestID]
	if !exists || record.DeliveryState != sessionRecoveryOutboxStatePending {
		o.mu.Unlock()
		return
	}
	owner := o.owners[requestID]
	if err == nil {
		record.DeliveryAttempts++
		record.DeliveryState = sessionRecoveryOutboxStateDelivered
		record.Destination = ""
		record.Proof = ""
		record.NextAttemptAt = ""
		if writeErr := o.writeRecordLocked(record); writeErr == nil {
			o.records[requestID] = record
			attempts := record.DeliveryAttempts
			o.mu.Unlock()
			o.logf("recovery outbox: outcome=delivered delivery_cycles=%d", attempts)
			return
		}
		o.mu.Unlock()
		return
	}
	if errors.Is(err, context.Canceled) {
		o.mu.Unlock()
		return
	}
	record.DeliveryAttempts++
	permanent := !errors.Is(err, accountrecovery.ErrDeliveryTransient)
	delay := o.retryDelay(record.DeliveryAttempts)
	next := now.Add(delay)
	exhausted := record.DeliveryAttempts >= o.maxDeliveryAttempts || !next.Before(expiresAt)
	if permanent || exhausted {
		record.DeliveryState = sessionRecoveryOutboxStateFailed
		record.Active = false
		record.Destination = ""
		record.Proof = ""
		record.NextAttemptAt = ""
		if writeErr := o.writeRecordLocked(record); writeErr != nil {
			// The durable pending record is still authoritative. Do not retire the
			// in-memory challenge until the terminal non-authorizing state is on
			// disk, otherwise a restart could resurrect the older active record.
			o.mu.Unlock()
			o.logf("recovery outbox: outcome=state_persist_failed")
			return
		}
		o.records[requestID] = record
		o.mu.Unlock()
		if owner != nil {
			owner.deactivateSessionRecoveryChallenge(requestID)
		}
		outcome := "permanent"
		if exhausted && !permanent {
			outcome = "exhausted"
		}
		o.logf("recovery outbox: outcome=%s delivery_cycles=%d", outcome, record.DeliveryAttempts)
		return
	}
	record.NextAttemptAt = next.Format(time.RFC3339Nano)
	if writeErr := o.writeRecordLocked(record); writeErr == nil {
		o.records[requestID] = record
	}
	o.mu.Unlock()
	o.logf("recovery outbox: outcome=retry_scheduled delivery_cycles=%d", record.DeliveryAttempts)
}

func (o *sessionRecoveryDurableOutbox) retryDelay(attempts int) time.Duration {
	if attempts <= 0 {
		return o.retryMin
	}
	delay := o.retryMin
	for i := 1; i < attempts; i++ {
		if delay >= o.retryMax/2 {
			return o.retryMax
		}
		delay *= 2
	}
	if delay > o.retryMax {
		return o.retryMax
	}
	return delay
}

func (o *sessionRecoveryDurableOutbox) nextDelay() time.Duration {
	now := o.now().UTC()
	o.mu.Lock()
	defer o.mu.Unlock()
	var earliest time.Time
	for requestID, record := range o.records {
		if record.DeliveryState != sessionRecoveryOutboxStatePending || !o.ready[requestID] {
			continue
		}
		next, err := parseSessionRecoveryOutboxTime(record.NextAttemptAt)
		if err != nil || !next.After(now) {
			return 0
		}
		if earliest.IsZero() || next.Before(earliest) {
			earliest = next
		}
	}
	if earliest.IsZero() {
		return time.Minute
	}
	delay := earliest.Sub(now)
	if delay < 0 {
		return 0
	}
	return delay
}

func (o *sessionRecoveryDurableOutbox) signalWake() {
	if o == nil {
		return
	}
	select {
	case o.wake <- struct{}{}:
	default:
	}
}
