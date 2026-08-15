package worldruntime

// EnqueueStartNextSiegeRound requests the next authoritative Siege round. The command is
// Server-only: schedulers/management may enqueue it, while the world owner performs the actual
// Gate/role/objective reset during Step. Protocol v7 exposes no Client reset command.
func (r *Runtime) EnqueueStartNextSiegeRound() error {
	if r == nil || r.queue == nil {
		return ErrSiegeUnavailable
	}
	return r.queue.tryPush(setBlockerCommand{startNextSiegeRound: true})
}

func (r *Runtime) applyStartNextSiegeRound(name string, report *StepReport) {
	if r != nil {
		// A scheduled request is now being consumed. If the reset fails, D.3D may retry on a
		// later Completed tick instead of leaving the policy permanently latched as queued.
		r.siegeRoundResetQueued = false
	}
	if r == nil || r.siege == nil || r.dynamic == nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: ErrSiegeUnavailable})
		return
	}
	changed, err := r.siege.StartNextRound(r.dynamic)
	if err != nil {
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: name, Err: err})
		return
	}
	if changed {
		// Gate HP and blocker state are part of Reliable WorldDynamicState. Match revision/team
		// changes independently force Reliable SiegeMatchState resend to every active session.
		r.bumpDynamicRevision()
	}
}
