package worldruntime

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

var (
	ErrMapExitSourceNotIsolated       = errors.New("worldruntime: map exit source is not isolated")
	ErrMapExitPolicyUnavailable       = errors.New("worldruntime: map exit policy unavailable")
	ErrMapExitInvalidDestination      = errors.New("worldruntime: invalid map exit destination")
	ErrMapExitInvalidRestriction      = errors.New("worldruntime: invalid map exit item restriction")
	ErrMapExitRestrictedItem          = errors.New("worldruntime: map exit blocked by restricted item")
	ErrMapExitRequiresTrustedIdentity = errors.New("worldruntime: map exit requires trusted durable identity")
	ErrMapExitPersistenceUnavailable  = errors.New("worldruntime: map exit persistence unavailable")
	ErrMapExitSnapshotUnavailable     = errors.New("worldruntime: map exit snapshot unavailable")
)

type MapExitRequest struct {
	DestinationWorld     characterstate.WorldRef
	DestinationTransform world.Transform
}

func (r *Runtime) EnqueueMapExit(id session.ID, request MapExitRequest) error {
	return r.EnqueueMapExitOwned(id, SessionOwnershipFence{}, request)
}

func (r *Runtime) EnqueueMapExitOwned(id session.ID, ownership SessionOwnershipFence, request MapExitRequest) error {
	owned := request
	return r.queue.tryPush(leaveCommand{id: id, ownership: ownership, mapExit: &owned})
}

func (r *Runtime) applyMapExit(name string, c leaveCommand, report *StepReport) {
	request := c.mapExit
	if request == nil {
		return
	}
	sourceMapID := gameplayworld.MapID(r.characterStateWorld.MapID)
	if sourceMapID != gameplayworld.MapIDGMRoom || r.characterStateWorld.WorldID != "gm-room" {
		r.recordMapExitError(name, c.id, ErrMapExitSourceNotIsolated, report)
		return
	}
	policy, ok := gameplayworld.MapExitPolicyFor(sourceMapID)
	if !ok {
		r.recordMapExitError(name, c.id, ErrMapExitPolicyUnavailable, report)
		return
	}
	r.applyMapExitWithPolicy(name, c, policy, report)
}

func (r *Runtime) applyMapExitWithPolicy(name string, c leaveCommand, policy gameplayworld.MapExitPolicy, report *StepReport) {
	request := c.mapExit
	if request == nil {
		return
	}
	if err := validateMapExitDestination(r.characterStateWorld, *request); err != nil {
		r.recordMapExitError(name, c.id, err, report)
		return
	}
	s, ok := r.sessions.Get(c.id)
	if !ok {
		r.recordMapExitError(name, c.id, session.ErrSessionNotFound, report)
		return
	}
	if s.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
		r.recordMapExitError(name, c.id, ErrMapExitRequiresTrustedIdentity, report)
		return
	}
	if r.characterStateOutbox == nil {
		r.recordMapExitError(name, c.id, ErrMapExitPersistenceUnavailable, report)
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	restricted, err := firstRestrictedItemArchetype(inv, policy.RestrictedItemArchetypeIDs)
	if err != nil {
		r.recordMapExitError(name, c.id, err, report)
		return
	}
	if restricted != "" {
		r.recordMapExitError(name, c.id, fmt.Errorf("%w: %s", ErrMapExitRestrictedItem, restricted), report)
		return
	}
	binding, snapshot, ok := r.captureCharacterStateSnapshot(c.id, s.EntityID, nil)
	if !ok {
		r.recordMapExitError(name, c.id, ErrMapExitSnapshotUnavailable, report)
		return
	}
	snapshot.World = request.DestinationWorld
	snapshot.Position = request.DestinationTransform.Position
	snapshot.Yaw = request.DestinationTransform.Yaw
	removed, err := r.sessions.Remove(c.id)
	if err != nil {
		r.recordMapExitError(name, c.id, err, report)
		return
	}
	if _, err := r.characterStateOutbox.Enqueue(binding, snapshot); err != nil {
		if rollbackErr := r.sessions.Add(removed); rollbackErr != nil {
			panic(fmt.Sprintf("worldruntime: map exit session rollback failed: %v", rollbackErr))
		}
		r.recordMapExitError(name, c.id, err, report)
		if report != nil {
			report.Metrics.CharacterStateSaveIntentFailures++
		}
		return
	}
	if report != nil {
		report.Metrics.CharacterStateSaveIntentsEnqueued++
	}
	r.removeSiegeParticipant(removed)
	r.cleanupRemovedSession(removed, report)
}

func validateMapExitDestination(source characterstate.WorldRef, request MapExitRequest) error {
	destination := request.DestinationWorld
	mapID := strings.TrimSpace(destination.MapID)
	if mapID == "" || mapID != destination.MapID || destination.MapID == source.MapID {
		return ErrMapExitInvalidDestination
	}
	identity := protocol.WorldIdentity{WorldID: destination.WorldID, Revision: destination.Revision, GameplaySHA256: destination.GameplaySHA256}
	if !identity.Valid() {
		return ErrMapExitInvalidDestination
	}
	for _, value := range []float32{request.DestinationTransform.Position.X, request.DestinationTransform.Position.Y, request.DestinationTransform.Position.Z, request.DestinationTransform.Yaw} {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return ErrMapExitInvalidDestination
		}
	}
	return nil
}

func firstRestrictedItemArchetype(inv *inventory.Inventory, restrictedIDs []string) (string, error) {
	if len(restrictedIDs) == 0 {
		return "", nil
	}
	restricted := make(map[string]struct{}, len(restrictedIDs))
	for _, raw := range restrictedIDs {
		id := strings.TrimSpace(raw)
		if id == "" || id != raw {
			return "", ErrMapExitInvalidRestriction
		}
		restricted[id] = struct{}{}
	}
	if inv == nil {
		return "", nil
	}
	for _, stack := range inv.Snapshot() {
		if _, blocked := restricted[stack.ArchetypeID]; blocked {
			return stack.ArchetypeID, nil
		}
	}
	for _, instance := range inv.InstanceSnapshot() {
		if _, blocked := restricted[instance.ItemArchetypeID]; blocked {
			return instance.ItemArchetypeID, nil
		}
	}
	for _, equipped := range inv.EquippedArchetypeSnapshot() {
		if _, blocked := restricted[equipped.ArchetypeID]; blocked {
			return equipped.ArchetypeID, nil
		}
	}
	for _, equipped := range inv.EquippedInstanceSnapshot() {
		if _, blocked := restricted[equipped.Item.ItemArchetypeID]; blocked {
			return equipped.Item.ItemArchetypeID, nil
		}
	}
	return "", nil
}

func (r *Runtime) recordMapExitError(name string, id session.ID, err error, report *StepReport) {
	if report == nil {
		return
	}
	report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, SessionID: id, Err: err})
}
