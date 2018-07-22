package serviceutil

import (
	"fmt"
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type WaitCalc func() time.Duration

func WaitFixed(d time.Duration) WaitCalc {
	return func() time.Duration { return d }
}

// TimedRestart is an experimental service.Wrapper that restarts a service that
// ends prematurely after a specific interval.
//
// It's intended to prototype a method for retrying failed connections or
// subprocesses.
//
// The big problem with this approach is the use of service.Standalone() -
// there's no way to catch lockups caused by services that don't halt properly.
//
type TimedRestart struct {
	runnable service.Runnable

	limit   uint64
	timeout time.Duration
	wait    WaitCalc
	running uint32
	starts  uint64

	suspend   chan struct{}
	suspended int32

	onRestart func(start uint64, err error)
}

type TimedRestartOption func(tr *TimedRestart)

func TimedRestartActive(active bool) TimedRestartOption {
	return func(tr *TimedRestart) {
		if !active {
			tr.suspended = 1
		}
	}
}

func TimedRestartLimit(limit uint64) TimedRestartOption {
	return func(tr *TimedRestart) { tr.limit = limit }
}

func TimedRestartOnRestart(r func(start uint64, err error)) TimedRestartOption {
	return func(tr *TimedRestart) { tr.onRestart = r }
}

// NewTimedRestart creates a TimedRestart service. If you want to log errors,
// pass in a listener, otherwise pass nil.
func NewTimedRestart(runnable service.Runnable, timeout time.Duration, wait WaitCalc, options ...TimedRestartOption) *TimedRestart {
	if runnable == nil {
		panic("runnable was nil")
	}

	tr := &TimedRestart{
		runnable: runnable,
		timeout:  timeout,
		wait:     wait,
		suspend:  make(chan struct{}, 1),
	}
	for _, o := range options {
		o(tr)
	}
	return tr
}

func (t *TimedRestart) Running() bool         { return atomic.LoadUint32(&t.running) == 1 }
func (t *TimedRestart) Starts() uint64        { return atomic.LoadUint64(&t.starts) }
func (t *TimedRestart) Suspended() (out bool) { return atomic.LoadInt32(&t.suspended) == 1 }

// Suspend instructs the service to halt and to not restart.
func (t *TimedRestart) Suspend(suspended bool) (changed bool) {
	var sv int32
	if suspended {
		sv = 1
	}
	ov := atomic.SwapInt32(&t.suspended, sv)
	if ov == sv {
		return false
	}

	t.suspend <- struct{}{}

	return true
}

func (t *TimedRestart) Run(ctx service.Context) error {
	failer := service.NewFailureListener(1)
	runner := service.NewRunner(failer.ForRunner())

	// This is an interesting one. If the service fails to start first go,
	// we can't exactly say we're "ready", but we can't hold everything
	// else up waiting for this thing's restart to finally succeed.
	//
	// At the moment, this assumes that if you want a restarting service,
	// you're content to be signalled that things are "ready to be started
	// until they start", not that things are "ready to receive and process
	// connections".
	//
	// It might be better to say "the first time we fail, or the first time
	// we receive a ready signal from the child service, we are ready", but
	// the above caveat still applies - you can't really guarantee readiness
	// without some other mechanism as the restarting mechanism makes actual
	// "readiness" something that can cease to be true. This does not apply to
	// normal services - readiness is a permanent state until halt or failure.
	//
	if err := ctx.Ready(); err != nil {
		return nil
	}

	svc := service.New("", t.runnable)
	defer service.MustHaltTimeout(t.timeout, runner, svc)

	for {
		// Wait for suspended to be false:
		for atomic.LoadInt32(&t.suspended) == 1 {
			select {
			case <-t.suspend:
			case <-ctx.Done():
				return nil
			}
		}

		start := atomic.AddUint64(&t.starts, 1)
		err := runner.Start(ctx, svc)
		if err != nil {
			goto failure
		}
		atomic.StoreUint32(&t.running, 1)

		for atomic.LoadInt32(&t.suspended) == 0 {
			select {
			case <-t.suspend:
			case <-ctx.Done():
				return nil
			case err = <-failer.Failures():
				goto failure
			}
		}

		service.MustHaltTimeout(t.timeout, runner, svc)

	failure:
		atomic.StoreUint32(&t.running, 0)

		if t.onRestart != nil {
			t.onRestart(start, err)
		}

		if t.limit > 0 && start >= t.limit {
			return fmt.Errorf("retry limit exceeded")
		}

		wait := t.wait()
		if halted := service.Sleep(ctx, wait); halted {
			return nil
		}
	}
}
