package worldruntime

// stepFenceCommand is consumed by commandQueue.drain as a hard Step boundary.
// Commands queued after the fence cannot enter the same drained command batch.
type stepFenceCommand struct {
	reached chan struct{}
}

func (stepFenceCommand) name() string { return "step_fence" }

// EnqueueStepFence inserts a world-command queue boundary and returns a signal that closes
// when one Step has drained every command ahead of the fence. Commands enqueued after the
// signal are therefore guaranteed to wait for a later Step. The fence does not mutate gameplay
// state and does not bypass the single authoritative world-owner command path.
func (r *Runtime) EnqueueStepFence() (<-chan struct{}, error) {
	reached := make(chan struct{})
	if err := r.queue.tryPush(stepFenceCommand{reached: reached}); err != nil {
		return nil, err
	}
	return reached, nil
}
