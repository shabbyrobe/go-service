package service

import (
	"sync"
)

type runnerState struct {
	state          State
	startingCalled bool
	readyCalled    bool
	retain         bool
	readySignal    ReadySignal
	doneClosed     bool
	done           chan struct{}
	haltedClosed   bool
	halted         chan struct{}
	lock           sync.Mutex
}

func newRunnerState() *runnerState {
	return &runnerState{
		state: Halted,
	}
}

func (r *runnerState) resetStarting(ready ReadySignal) {
	r.lock.Lock()
	r.startingCalled = true
	r.readyCalled = false
	r.doneClosed = false
	r.done = make(chan struct{})
	r.haltedClosed = false
	r.halted = make(chan struct{})
	r.readySignal = ready
	r.lock.Unlock()
}

func (r *runnerState) ReadySignal() (rs ReadySignal) {
	r.lock.Lock()
	rs = r.readySignal
	r.lock.Unlock()
	return
}

func (r *runnerState) Done() {
	r.lock.Lock()
	if !r.doneClosed {
		r.doneClosed = true
		close(r.done)
	}
	r.lock.Unlock()
}

func (r *runnerState) Halted() {
	r.lock.Lock()
	if !r.haltedClosed {
		r.haltedClosed = true
		close(r.halted)
	}
	r.lock.Unlock()
}

func (r *runnerState) Retain() (retain bool) {
	r.lock.Lock()
	retain = r.retain
	r.lock.Unlock()
	return retain
}

func (r *runnerState) SetRetain(v bool) {
	r.lock.Lock()
	r.retain = v
	r.lock.Unlock()
}

func (r *runnerState) Calls() (starting, ready bool) {
	r.lock.Lock()
	starting, ready = r.startingCalled, r.readyCalled
	r.lock.Unlock()
	return
}

func (r *runnerState) Ready() {
	r.lock.Lock()
	r.readyCalled = true
	if r.readySignal != nil {
		_ = r.readySignal.Done(nil)
	}
	r.lock.Unlock()
}

func (r *runnerState) AllState() (s State, retain bool) {
	r.lock.Lock()
	s = r.state
	retain = r.retain
	r.lock.Unlock()
	return s, retain
}

func (r *runnerState) State() (s State) {
	r.lock.Lock()
	s = r.state
	r.lock.Unlock()
	return s
}
