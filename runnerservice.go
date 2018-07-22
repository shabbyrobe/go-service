package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	// "github.com/shabbyrobe/golib/synctools"
)

type runnerService struct {
	id      uint64
	service *Service // safe to access unlocked
	runner  *runner  // safe to access unlocked

	state      State
	startCtx   context.Context
	ready      Signal
	stage      Stage
	waiters    []Signal
	done       <-chan struct{}
	halt       chan struct{}
	joinedDone *joinedDone

	readyCalled bool

	mu sync.Mutex
	// mu synctools.LoggingMutex
}

func newRunnerService(id uint64, r *runner, svc *Service, ready Signal) *runnerService {
	rs := &runnerService{
		id:      id,
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
		cd := ctx.Done()
		if cd != nil {
			rs.joinedDone = joinDone(rs.halt, cd)
			rs.done = rs.joinedDone.out
		}
	}

	rs.setState(Starting)

	rs.mu.Unlock()
	return nil
}

func (rs *runnerService) halting(done Signal) (rerr error) {
	rs.mu.Lock()
	if rs.state == NoState || rs.state == Halted || rs.state == Ended {
		rs.mu.Unlock()
		if done != nil {
			done.Done(nil)
		}
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

// setReady expects rs.mu to be locked.
func (rs *runnerService) setReady(err error) {
	// Note: this deliberately does not set the state to Started as the places
	// where setReady is used have different destination states.

	rs.startCtx = nil
	rs.readyCalled = true

	if rs.joinedDone != nil {
		rs.joinedDone.setReady()
	}

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
	rs.runner.mu.Lock()
	rs.mu.Lock()

	if rs.startCtx != nil {
		rerr = rs.startCtx.Err()
	}

	rs.setReady(rerr)
	if rs.state == Starting {
		rs.setState(Started)
	}

	rs.mu.Unlock()
	rs.runner.mu.Unlock()

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
	rs.mu.Lock()
	done := rs.done
	rs.mu.Unlock()
	return done
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

type joinedDone struct {
	halt           chan struct{}
	startCtx       <-chan struct{}
	out            chan struct{}
	ignoreStartCtx int32
}

func (j *joinedDone) setReady() {
	atomic.StoreInt32(&j.ignoreStartCtx, 1)
}

func joinDone(halt chan struct{}, startCtx <-chan struct{}) *joinedDone {
	out := make(chan struct{})
	jd := &joinedDone{
		halt:     halt,
		startCtx: startCtx,
		out:      out,
	}

	go func() {
		select {
		case <-halt:
			close(out)
		case <-startCtx:
			if atomic.LoadInt32(&jd.ignoreStartCtx) == 1 {
				<-halt
			}

			// Clear channel to allow the context.Context to be garbage collected:
			jd.startCtx = nil

			close(out)
		}
	}()

	return jd
}
