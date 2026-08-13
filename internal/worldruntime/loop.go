package worldruntime

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidTickRate = errors.New("worldruntime: invalid tick rate")

type Loop struct {
	runtime      *Runtime
	tickDuration time.Duration
	tick         uint64
}

func NewLoop(runtime *Runtime, tickRateHz int) (*Loop, error) {
	if runtime == nil || tickRateHz <= 0 {
		return nil, ErrInvalidTickRate
	}
	return &Loop{runtime: runtime, tickDuration: time.Second / time.Duration(tickRateHz)}, nil
}

func (l *Loop) Step() StepReport {
	l.tick++
	return l.runtime.Step(l.tick, l.tickDuration)
}

func (l *Loop) Tick() uint64 { return l.tick }

func (l *Loop) Run(ctx context.Context) error {
	return l.RunObserved(ctx, nil)
}

// RunObserved 讓 composition root 可以集中記錄 command/tick/delivery errors，
// 而不把 logger 或 metrics 依賴塞進 worldruntime。
func (l *Loop) RunObserved(ctx context.Context, observer func(StepReport)) error {
	ticker := time.NewTicker(l.tickDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			report := l.Step()
			if observer != nil {
				observer(report)
			}
		}
	}
}
