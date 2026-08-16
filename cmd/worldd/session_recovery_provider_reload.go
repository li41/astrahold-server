package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

const sessionRecoveryMaxRetiredProviderGenerations = 4

var errSessionRecoveryReloadRestartOnly = errors.New("worldd: recovery provider is restart-only")

type sessionRecoveryDeliveryRetirer interface {
	Retire()
}

type sessionRecoveryProviderRoute struct {
	provider   *staticSessionRecoveryProvider
	generation uint64
	expiresAt  time.Time
}

type sessionRecoveryProviderReloadResult struct {
	PreviousGeneration uint64
	Generation         uint64
	PreviousRevision   string
	Revision           string
	PreviousMethod     string
	Method             string
	RetainedChallenges int
	RetiredChallenges  int
}

// reloadableSessionRecoveryProvider serializes generation publication against
// Begin. A Begin already using the old generation is allowed to finish delivery
// and register its opaque request route before Replace publishes the new
// generation. Verify/Consume keep routing that challenge to the old verifier,
// while new Begin calls use only the newly published provider.
type reloadableSessionRecoveryProvider struct {
	generationMu sync.RWMutex
	routeMu      sync.Mutex
	current      *staticSessionRecoveryProvider
	generation   uint64
	routes       map[string]sessionRecoveryProviderRoute
	retiredOrder []uint64
	now          func() time.Time
}

func newReloadableSessionRecoveryProvider(initial *staticSessionRecoveryProvider, now func() time.Time) (*reloadableSessionRecoveryProvider, error) {
	if initial == nil || initial.delivery == nil {
		return nil, errSessionRecoveryReloadRestartOnly
	}
	if now == nil {
		now = time.Now
	}
	return &reloadableSessionRecoveryProvider{
		current:    initial,
		generation: 1,
		routes:     make(map[string]sessionRecoveryProviderRoute),
		now:        now,
	}, nil
}

func (r *reloadableSessionRecoveryProvider) Begin(ctx context.Context, subject accountrecovery.Subject) (accountrecovery.Challenge, error) {
	if r == nil {
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	r.generationMu.RLock()
	provider := r.current
	generation := r.generation
	if provider == nil {
		r.generationMu.RUnlock()
		return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
	}
	challenge, err := provider.Begin(ctx, subject)
	if err == nil && challenge.Valid() {
		r.routeMu.Lock()
		r.pruneExpiredRoutesLocked(r.now().UTC())
		if _, collision := r.routes[challenge.RequestID]; collision {
			r.routeMu.Unlock()
			provider.Consume(context.Background(), challenge.RequestID)
			r.generationMu.RUnlock()
			return accountrecovery.Challenge{}, accountrecovery.ErrUnavailable
		}
		r.routes[challenge.RequestID] = sessionRecoveryProviderRoute{
			provider:   provider,
			generation: generation,
			expiresAt:  challenge.ExpiresAt,
		}
		r.routeMu.Unlock()
	}
	r.generationMu.RUnlock()
	return challenge, err
}

func (r *reloadableSessionRecoveryProvider) Verify(ctx context.Context, requestID string, proof []byte) (accountrecovery.Grant, error) {
	if r == nil {
		return accountrecovery.Grant{}, accountrecovery.ErrRejected
	}
	r.routeMu.Lock()
	route, routed := r.routes[requestID]
	r.routeMu.Unlock()
	if routed {
		grant, err := route.provider.Verify(ctx, requestID, proof)
		if !r.now().UTC().Before(route.expiresAt) {
			r.removeRoute(requestID, route)
		}
		return grant, err
	}

	// Preserve the provider's unknown-request verification path so rejected
	// opaque IDs still use the same provider-side dummy verifier behavior.
	r.generationMu.RLock()
	provider := r.current
	if provider == nil {
		r.generationMu.RUnlock()
		return accountrecovery.Grant{}, accountrecovery.ErrRejected
	}
	grant, err := provider.Verify(ctx, requestID, proof)
	r.generationMu.RUnlock()
	return grant, err
}

func (r *reloadableSessionRecoveryProvider) Consume(ctx context.Context, requestID string) {
	if r == nil || requestID == "" {
		return
	}
	r.routeMu.Lock()
	route, routed := r.routes[requestID]
	if routed {
		delete(r.routes, requestID)
	}
	r.routeMu.Unlock()
	if routed {
		route.provider.Consume(ctx, requestID)
		return
	}
	r.generationMu.RLock()
	provider := r.current
	if provider != nil {
		provider.Consume(ctx, requestID)
	}
	r.generationMu.RUnlock()
}

func (r *reloadableSessionRecoveryProvider) Method() string {
	if r == nil {
		return ""
	}
	r.generationMu.RLock()
	defer r.generationMu.RUnlock()
	if r.current == nil {
		return ""
	}
	return r.current.Method()
}

func (r *reloadableSessionRecoveryProvider) Revision() string {
	if r == nil {
		return ""
	}
	r.generationMu.RLock()
	defer r.generationMu.RUnlock()
	if r.current == nil {
		return ""
	}
	return r.current.Revision()
}

func (r *reloadableSessionRecoveryProvider) Generation() uint64 {
	if r == nil {
		return 0
	}
	r.generationMu.RLock()
	defer r.generationMu.RUnlock()
	return r.generation
}

// Replace publishes a fully validated schema-v2 provider as one new runtime
// generation. It waits for old-generation Begin calls to finish. Existing
// challenge verifiers remain routed to their original provider, but proof-key
// and delivery-adapter credentials from the retired generation are cleared at
// cutover because Verify no longer needs them.
func (r *reloadableSessionRecoveryProvider) Replace(next *staticSessionRecoveryProvider) (sessionRecoveryProviderReloadResult, error) {
	if r == nil || next == nil || next.delivery == nil {
		if next != nil {
			next.retireDeliverySecrets()
		}
		return sessionRecoveryProviderReloadResult{}, errSessionRecoveryReloadRestartOnly
	}

	r.generationMu.Lock()
	defer r.generationMu.Unlock()
	if r.current == nil {
		next.retireDeliverySecrets()
		return sessionRecoveryProviderReloadResult{}, accountrecovery.ErrUnavailable
	}

	old := r.current
	oldGeneration := r.generation
	oldRevision := old.Revision()
	oldMethod := old.Method()

	r.routeMu.Lock()
	retiredChallenges := r.pruneExpiredRoutesLocked(r.now().UTC())
	retainedChallenges := r.countGenerationRoutesLocked(oldGeneration)
	if retainedChallenges > 0 {
		r.retiredOrder = append(r.retiredOrder, oldGeneration)
	}
	r.current = next
	r.generation = oldGeneration + 1
	old.retireDeliverySecrets()
	r.compactRetiredOrderLocked()
	for len(r.retiredOrder) > sessionRecoveryMaxRetiredProviderGenerations {
		oldest := r.retiredOrder[0]
		r.retiredOrder = r.retiredOrder[1:]
		retiredChallenges += r.retireGenerationRoutesLocked(oldest)
	}
	r.routeMu.Unlock()

	return sessionRecoveryProviderReloadResult{
		PreviousGeneration: oldGeneration,
		Generation:         r.generation,
		PreviousRevision:   oldRevision,
		Revision:           next.Revision(),
		PreviousMethod:     oldMethod,
		Method:             next.Method(),
		RetainedChallenges: retainedChallenges,
		RetiredChallenges:  retiredChallenges,
	}, nil
}

func (r *reloadableSessionRecoveryProvider) removeRoute(requestID string, route sessionRecoveryProviderRoute) {
	r.routeMu.Lock()
	current, exists := r.routes[requestID]
	if exists && current.provider == route.provider && current.generation == route.generation {
		delete(r.routes, requestID)
	}
	r.routeMu.Unlock()
	route.provider.Consume(context.Background(), requestID)
}

func (r *reloadableSessionRecoveryProvider) pruneExpiredRoutesLocked(now time.Time) int {
	retired := 0
	for requestID, route := range r.routes {
		if !now.Before(route.expiresAt) {
			delete(r.routes, requestID)
			route.provider.Consume(context.Background(), requestID)
			retired++
		}
	}
	r.compactRetiredOrderLocked()
	return retired
}

func (r *reloadableSessionRecoveryProvider) countGenerationRoutesLocked(generation uint64) int {
	count := 0
	for _, route := range r.routes {
		if route.generation == generation {
			count++
		}
	}
	return count
}

func (r *reloadableSessionRecoveryProvider) retireGenerationRoutesLocked(generation uint64) int {
	retired := 0
	for requestID, route := range r.routes {
		if route.generation != generation {
			continue
		}
		delete(r.routes, requestID)
		route.provider.Consume(context.Background(), requestID)
		retired++
	}
	return retired
}

func (r *reloadableSessionRecoveryProvider) compactRetiredOrderLocked() {
	if len(r.retiredOrder) == 0 {
		return
	}
	active := make(map[uint64]bool)
	for _, route := range r.routes {
		active[route.generation] = true
	}
	dst := r.retiredOrder[:0]
	seen := make(map[uint64]bool)
	for _, generation := range r.retiredOrder {
		if active[generation] && !seen[generation] {
			dst = append(dst, generation)
			seen[generation] = true
		}
	}
	r.retiredOrder = dst
}

// retireDeliverySecrets is called only after the generation barrier has waited
// for all old Begin/delivery calls to finish. Existing challenges keep their
// verifier digest and Subject binding, so the HMAC proof key and transport
// credentials are no longer needed for Verify/Consume.
func (p *staticSessionRecoveryProvider) retireDeliverySecrets() {
	if p == nil || p.delivery == nil {
		return
	}
	clear(p.proofKey[:])
	if retirer, ok := p.delivery.(sessionRecoveryDeliveryRetirer); ok {
		retirer.Retire()
	}
}

func wrapSessionRecoveryProviderForRuntime(provider *staticSessionRecoveryProvider) (accountrecovery.Provider, error) {
	if provider == nil || provider.delivery == nil {
		return provider, nil
	}
	return newReloadableSessionRecoveryProvider(provider, time.Now)
}

func sessionRecoveryReloadMetadata(provider accountrecovery.Provider) (mode string, generation uint64) {
	if reloadable, ok := provider.(*reloadableSessionRecoveryProvider); ok {
		return "sighup", reloadable.Generation()
	}
	if provider != nil {
		return "restart-only", 1
	}
	return "disabled", 0
}

func (r *sessionLoginRuntime) reloadRecoveryProvider() (sessionRecoveryProviderReloadResult, error) {
	if r == nil || r.recoveryProvider == nil {
		return sessionRecoveryProviderReloadResult{}, errSessionRecoveryReloadRestartOnly
	}
	reloadable, ok := r.recoveryProvider.(*reloadableSessionRecoveryProvider)
	if !ok {
		return sessionRecoveryProviderReloadResult{}, errSessionRecoveryReloadRestartOnly
	}
	path := strings.TrimSpace(*sessionRecoveryProviderFile)
	if path == "" {
		return sessionRecoveryProviderReloadResult{}, fmt.Errorf("%w: recovery provider file is empty", errSessionRecoveryReloadRestartOnly)
	}
	next, err := loadStaticSessionRecoveryProvider(
		path,
		*sessionRecoveryChallengeTTL,
		*sessionRecoveryChallengeMaxAttempts,
		time.Now,
		rand.Reader,
	)
	if err != nil {
		return sessionRecoveryProviderReloadResult{}, err
	}
	if next.delivery == nil {
		next.retireDeliverySecrets()
		return sessionRecoveryProviderReloadResult{}, fmt.Errorf("%w: schema-v2 delivered provider required", errSessionRecoveryReloadRestartOnly)
	}
	return reloadable.Replace(next)
}

var _ accountrecovery.Provider = (*reloadableSessionRecoveryProvider)(nil)
