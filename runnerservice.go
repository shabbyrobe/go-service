package service

import (
	"context"
	"time"

	// "github.com/shabbyrobe/golib/synctools"
	"github.com/shabbyrobe/golib/synctools"
)

type runnerService struct {
	state  State
	retain bool

	startCtx context.Context

	service     *Service
	runner      *runner
	ready       Signal
	stage       Stage
	waiters     []Signal
	done        <-chan struct{}
	halt        chan struct{}
	readyCalled bool

	mu synctools.LoggingMutex
}

func newRunnerService(r *runner, svc *Service, ready Signal) *runnerService {
	rs := &runnerService{
		state:   Halted,
		stage:   StageReady,
		ready:   ready,
		runner:  r,
		service: svc,
		halt:    make(chan struct{}),
	}

	rs.done = rs.halt
	return rs
}

func (rs *runnerService) State() (state State) {
	rs.mu.Lock()
	state = rs.state
	rs.mu.Unlock()
	return state
}

func (rs *runnerService) starting(ctx context.Context) error {
	rs.mu.Lock()

	current := rs.state
	if current != Halted && current != Ended {
		rs.mu.Unlock()
		return &errState{Current: current, Expected: Halted | Ended, To: Starting}
	}

	rs.startCtx = ctx
	if ctx != nil {
		rs.done = join(rs.halt, ctx.Done())
	} else {
		rs.done = rs.halt
	}
	rs.setState(Starting)

	rs.mu.Unlock()
	return nil
}

func (rs *runnerService) halting(done Signal) (rerr error) {
	rs.mu.Lock()
	if rs.state == Halted || rs.state == Ended {
		rs.mu.Unlock()
		done.Done(nil)
		return nil
	}

	if done != nil {
		rs.waiters = append(rs.waiters, done)
	}

	if rs.state != Halting {
		rs.setState(Halting)
		close(rs.halt)
	}

	rs.mu.Unlock()

	return nil
}

func (rs *runnerService) ended(err error) error {
	rs.mu.Lock()
	rs.setState(Ended)
	rs.done = nil

	// This is a strange looking bit of code; we have to separate
	// "ready errors" from "halt errors".
	//
	// - If a service ends, regardless of whether it was a "ready error" or a
	//   "halt error", we want it reported to the listener as the "reason why
	//   the service ended".
	//
	// - If the service fails before it is "ready", the error should be sent
	//   to the "ready" signal *only*, not the "halt" signal.
	//
	// - This is relevant if "halt" is called after a service fails before it's
	//   ready with a context timeout.
	//
	herr := err
	if rs.stage == StageReady {
		rs.setReady(err)
		herr = nil
	}

	// This MUST happen before the waiters are notified. If not, then the
	// service won't be deleted from Runner.services before Halt() returns,
	// which can cause Start() to return a "service already running" error
	// even if the calls to Start and Halt are sequential.
	rs.runner.ended(rs.stage, rs.service, err)

	rs.readyCalled = false
	for _, w := range rs.waiters {
		w.Done(herr)
	}
	rs.waiters = nil

	rs.mu.Unlock()

	return nil
}

func (rs *runnerService) Deadline() (deadline time.Time, ok bool) {
	rs.mu.Lock()
	if rs.startCtx != nil {
		deadline, ok = rs.startCtx.Deadline()
	}
	rs.mu.Unlock()
	return time.Time{}, false
}

// Err implements context.Context.Err().
func (rs *runnerService) Err() (rerr error) {
	rs.mu.Lock()
	if rs.state == Ended {
		rerr = context.Canceled
	}
	rs.mu.Unlock()
	return rerr
}

// Value implements context.Context.Value, which you probably shouldn't use if
// you can avoid it:
// https://medium.com/@cep21/how-to-correctly-use-context-context-in-go-1-7-8f2c0fafdf39
func (rs *runnerService) Value(key interface{}) (out interface{}) {
	rs.mu.Lock()
	if rs.startCtx != nil {
		out = rs.startCtx.Value(key)
	}
	rs.mu.Unlock()
	return out
}

// setReady expects rs.mu to be locked.
func (rs *runnerService) setReady(err error) {
	// Note: this deliberately does not set the state to Started as the places
	// where setReady is used have different destination states.

	rs.startCtx = nil
	rs.readyCalled = true
	rs.done = rs.halt
	if err == nil {
		rs.stage = StageRun
	}
	if rs.ready != nil {
		rs.ready.Done(err)
		rs.ready = nil
	}
}

// setState expects rs.mu to be locked.
func (rs *runnerService) setState(state State) {
	old := rs.state
	rs.state = state
	rs.runner.raiseOnState(rs.service, old, rs.state)
}

func (rs *runnerService) Ready() (rerr error) {
	rs.mu.Lock()
	if rs.startCtx != nil {
		rerr = rs.startCtx.Err()
	}

	rs.setReady(rerr)
	rs.setState(Started)
	rs.mu.Unlock()

	return rerr
}

func (rs *runnerService) OnError(err error) {
	rs.mu.Lock()
	runner, service, stage := rs.runner, rs.service, rs.stage
	rs.mu.Unlock()

	// Warning: do not attempt to access rs below this point

	runner.raiseOnError(stage, service, err)
}

func (rs *runnerService) ShouldHalt() (v bool) {
	rs.mu.Lock()
	v = rs.state == Halting || rs.state == Halted || rs.state == Ended
	rs.mu.Unlock()
	return v
}

func (rs *runnerService) Done() <-chan struct{} {
	return rs.done
}

func join(done chan struct{}, ctxDone <-chan struct{}) chan struct{} {
	if ctxDone == nil {
		return done
	}

	o := make(chan struct{})
	go func() {
		select {
		case <-done:
		case <-ctxDone:
		}
		close(o)
	}()
	return o
}
