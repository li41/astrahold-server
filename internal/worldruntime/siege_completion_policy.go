package worldruntime

import (
	"time"

	"github.com/li41/astrahold-server/internal/siege"
)

const siegeRoundScheduleCommandName = "schedule_next_siege_round"

// observeSiegeCompletionScheduling runs after the existing Reliable Siege replication pass.
// A zero pending count means every session active in that pass has accepted the current
// revision through TrySend; Protocol v7 has no Client acknowledgement and this policy does not
// pretend otherwise.
func (r *Runtime) observeSiegeCompletionScheduling(state siege.MatchState, activeSessions, pendingDeliveries int, report *StepReport) {
	if r == nil || state.Phase != siege.MatchPhaseCompleted {
		if r != nil {
			r.resetSiegeCompletionScheduling()
		}
		return
	}
	if r.dynamic == nil || r.config.SiegeCompletedMaxHold <= 0 {
		return
	}

	if r.siegeCompletedRevision != state.Revision {
		r.siegeCompletedRevision = state.Revision
		r.siegeCompletedElapsed = 0
		r.siegeRoundResetQueued = false
	} else {
		r.siegeCompletedElapsed = saturatingDurationAdd(r.siegeCompletedElapsed, r.siegeStepDelta, r.config.SiegeCompletedMaxHold)
	}

	if report != nil {
		report.Metrics.SiegeCompletedActiveSessions = activeSessions
		report.Metrics.SiegeCompletedPendingDeliveries = pendingDeliveries
		report.Metrics.SiegeCompletedElapsed = r.siegeCompletedElapsed
	}
	if r.siegeRoundResetQueued {
		return
	}

	minReached := r.siegeCompletedElapsed >= r.config.SiegeCompletedMinHold
	maxReached := r.siegeCompletedElapsed >= r.config.SiegeCompletedMaxHold
	allAccepted := pendingDeliveries == 0
	if !maxReached && (!minReached || !allAccepted) {
		return
	}

	if err := r.EnqueueStartNextSiegeRound(); err != nil {
		if report != nil {
			report.Metrics.SiegeRoundResetScheduleFailures++
			report.CommandErrors = append(report.CommandErrors, CommandError{Command: siegeRoundScheduleCommandName, Err: err})
		}
		return
	}
	r.siegeRoundResetQueued = true
	if report != nil {
		report.Metrics.SiegeRoundResetsScheduled++
		if maxReached && !allAccepted {
			report.Metrics.SiegeRoundResetsForcedByMaxHold++
		}
	}
}

func (r *Runtime) resetSiegeCompletionScheduling() {
	r.siegeCompletedRevision = 0
	r.siegeCompletedElapsed = 0
	r.siegeRoundResetQueued = false
}

func saturatingDurationAdd(current, delta, limit time.Duration) time.Duration {
	if delta <= 0 || current >= limit {
		return current
	}
	remaining := limit - current
	if delta >= remaining {
		return limit
	}
	return current + delta
}
