package service

import (
	"sync"
)

type runnerService struct {
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

func newRunnerService() *runnerService {
	return &runnerService{
		state: Halted,
	}
}

func (r *runnerService) resetStarting(ready ReadySignal) {
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

func (r *runnerService) ReadySignal() (rs ReadySignal) {
	r.lock.Lock()
	rs = r.readySignal
	r.lock.Unlock()
	return
}

func (r *runnerService) Done() {
	r.lock.Lock()
	if !r.doneClosed {
		r.doneClosed = true
		close(r.done)
	}
	r.lock.Unlock()
}

func (r *runnerService) Halted() {
	r.lock.Lock()
	if !r.haltedClosed {
		r.haltedClosed = true
		close(r.halted)
	}
	r.lock.Unlock()
}

func (r *runnerService) Retain() (retain bool) {
	r.lock.Lock()
	retain = r.retain
	r.lock.Unlock()
	return retain
}

func (r *runnerService) SetRetain(v bool) {
	r.lock.Lock()
	r.retain = v
	r.lock.Unlock()
}

func (r *runnerService) Calls() (starting, ready bool) {
	r.lock.Lock()
	starting, ready = r.startingCalled, r.readyCalled
	r.lock.Unlock()
	return
}

func (r *runnerService) Ready() {
	r.lock.Lock()
	r.readyCalled = true
	if r.readySignal != nil {
		_ = r.readySignal.Done(nil)
	}
	r.lock.Unlock()
}

func (r *runnerService) AllState() (s State, retain bool) {
	r.lock.Lock()
	s = r.state
	retain = r.retain
	r.lock.Unlock()
	return s, retain
}

func (r *runnerService) State() (s State) {
	r.lock.Lock()
	s = r.state
	r.lock.Unlock()
	return s
}

func (r *runnerService) SetState(s State) (old State, rerr error) {
	r.lock.Lock()
	old, rerr = r.state.set(s)
	r.lock.Unlock()
	return old, rerr
}
