package service

import (
	"sync"
	"time"
)

type runnerService struct {
	state          State
	startingCalled bool
	readyCalled    bool
	retain         bool
	readySignal    ReadySignal
	lock           sync.Mutex

	doneClosed bool
	done       chan struct{}

	endLock   sync.Mutex
	endCalled bool
	endWaiter chan struct{}
	endErr    error
}

func newRunnerService() *runnerService {
	rs := &runnerService{
		state: Halted,
		done:  make(chan struct{}),
	}
	return rs
}

func (r *runnerService) resetStarting(ready ReadySignal) {
	r.lock.Lock()
	{
		r.startingCalled = true
		r.readyCalled = false
		r.readySignal = ready
		r.doneClosed = false
		r.done = make(chan struct{})

		r.endLock.Lock()
		{
			r.endErr = nil
			r.endCalled = false
			r.endWaiter = nil
		}
		r.endLock.Unlock()
	}
	r.lock.Unlock()
}

func (r *runnerService) State() (s State) {
	r.lock.Lock()
	s = r.state
	r.lock.Unlock()
	return s
}

func (r *runnerService) Starting(svc Service, l StateListener) (old State, rerr error) {
	r.lock.Lock()
	old, rerr = r.state.set(Starting)
	r.lock.Unlock()
	if rerr != nil && l != nil && old != Starting {
		go l.OnServiceState(svc, old, Starting)
	}
	return old, rerr
}

func (r *runnerService) Started(svc Service, l StateListener) (old State, rerr error) {
	r.lock.Lock()
	old, rerr = r.state.set(Started)
	r.lock.Unlock()
	if rerr != nil && l != nil && Started != old {
		go l.OnServiceState(svc, old, Started)
	}
	return old, rerr
}

func (r *runnerService) Halting(svc Service, l StateListener) (old State, rerr error) {
	r.lock.Lock()
	old, rerr = r.state.set(Halting)

	if rerr != nil {
		r.lock.Unlock()
		return old, rerr
	}

	if l != nil && old != Halting {
		go l.OnServiceState(svc, old, Halting)
	}

	if !r.doneClosed {
		r.doneClosed = true
		close(r.done)
	}

	if old != Halting {
		r.endLock.Lock()
		r.endCalled, r.endErr = false, nil
		r.endWaiter = make(chan struct{}, 1)
		r.endLock.Unlock()
	}

	r.lock.Unlock()

	return old, rerr
}

func (r *runnerService) Halted(svc Service, l StateListener, force bool, remove func(svc Service)) (old State, rerr error) {
	r.lock.Lock()
	retained := r.retain
	if !force {
		old, rerr = r.state.set(Halted)
	} else {
		old, r.state = r.state, Halted
	}
	r.lock.Unlock()

	if rerr != nil && l != nil && old != Halted {
		go l.OnServiceState(svc, old, Halted)
	}

	if !retained {
		remove(svc)
	}

	return old, rerr
}

func (r *runnerService) Ended(svc Service, err error, l Listener, sl StateListener, remove func(svc Service)) error {
	var state State
	var waiter chan struct{}
	var startingCalled, readyCalled bool
	var ready ReadySignal

	{
		r.lock.Lock()

		// If the service ended before Halt() was called, the Done() channel
		// will stay open unless we close it.
		if !r.doneClosed {
			r.doneClosed = true
			close(r.done)
		}

		state = r.state
		startingCalled, readyCalled = r.startingCalled, r.readyCalled
		ready = r.readySignal

		r.endLock.Lock()
		if r.endWaiter != nil {
			r.endErr = err
			r.endCalled = true
			waiter = r.endWaiter
		}
		r.endLock.Unlock()

		r.lock.Unlock()
	}

	// -- all locks freed below this point.

	stage := StageRun
	if !readyCalled {
		stage = StageReady
	}

	if !readyCalled && startingCalled && ready != nil {
		// If the service ended while it was starting, Ready() will never be called.
		r.Ready(&serviceError{name: svc.ServiceName(), cause: err})
	}

	if state == Halting {
		r.endLock.Lock()
		close(waiter)
		r.endWaiter = nil
		r.endErr = nil
		r.endLock.Unlock()
	} else {
		if _, err := r.Halted(svc, sl, true, remove); err != nil {
			return err
		}
	}

	if l != nil {
		go l.OnServiceEnd(stage, svc, WrapError(err, svc))
	}

	return nil
}

func (r *runnerService) EndWait(timeout time.Duration) error {
	r.endLock.Lock()
	endErr := r.endErr
	if r.endCalled {
		r.endLock.Unlock()
		return endErr
	}
	waiter := r.endWaiter
	r.endLock.Unlock()

	after := time.After(timeout)
	select {
	case <-waiter:
		return endErr
	case <-after:
		return errHaltTimeout(0)
	}
	return nil
}

func (r *runnerService) ReadySignal() (rs ReadySignal) {
	r.lock.Lock()
	rs = r.readySignal
	r.lock.Unlock()
	return
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

func (r *runnerService) Ready(err error) {
	r.lock.Lock()
	r.readyCalled = true
	if r.readySignal != nil {
		_ = r.readySignal.Done(err)
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
