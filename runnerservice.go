package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type runnerService struct {
	state  State
	retain bool

	startCtx context.Context
	haltCtx  context.Context

	service     *Service
	runner      *runner
	ready       Signal
	waiters     []Signal
	done        <-chan struct{}
	halt        chan struct{}
	readyCalled bool

	mu sync.Mutex
}

func newRunnerService(r *runner, svc *Service, ready Signal) *runnerService {
	rs := &runnerService{
		state:   Halted,
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

	if rs.state != Halted && rs.state != Ended {
		rs.mu.Unlock()
		return fmt.Errorf("expected halted or ended")
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

func (rs *runnerService) halting(ctx context.Context, done Signal) (rerr error) {
	rs.mu.Lock()
	if rs.state == Halted || rs.state == Ended {
		rs.mu.Unlock()
		done.Done(nil)
		return nil
	}

	rs.haltCtx = ctx
	rs.done = ctx.Done()

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
	rs.haltCtx = nil

	stage := StageReady
	if rs.readyCalled {
		stage = StageRun
	} else {
		rs.setReady()
	}

	rs.readyCalled = false
	for _, w := range rs.waiters {
		w.Done(err)
	}
	rs.waiters = nil

	rs.runner.ended(stage, rs.service, err)

	rs.mu.Unlock()

	return nil
}

func (rs *runnerService) Deadline() (deadline time.Time, ok bool) {
	rs.mu.Lock()
	if rs.startCtx != nil {
		deadline, ok = rs.startCtx.Deadline()
	} else if rs.haltCtx != nil {
		deadline, ok = rs.haltCtx.Deadline()
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
func (rs *runnerService) setReady() {
	// Note: this deliberately does not set the state to Started as the places
	// where it is used have different destination states.

	rs.startCtx = nil
	rs.readyCalled = true
	rs.done = rs.halt
	if rs.ready != nil {
		rs.ready.Done(nil)
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

	rs.setReady()
	rs.setState(Started)
	rs.mu.Unlock()

	return rerr
}

func (rs *runnerService) OnError(err error) {
	rs.mu.Lock()
	stage := StageReady
	if rs.readyCalled {
		stage = StageRun
	}
	runner, service := rs.runner, rs.service
	rs.mu.Unlock()

	// Warning: do not attempt to access rs below this point

	runner.raiseOnError(stage, service, err)
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
