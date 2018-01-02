package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Runner Starts, Halts and manages Services.
type Runner interface {
	State(s Service) State
	StartWait(s Service, timeout time.Duration) error

	// Start a service in this runner. The runner will retain a reference to it
	// until Unregister is called even if the service is Halted.
	Start(s Service) error

	Halt(s Service, timeout time.Duration) error
	HaltAll(timeout time.Duration) error

	// List of services currently registered at the time of the call.
	// If State is provided, only services matching the state are returned.
	Services(state State) []Service

	// If you start a service, the runner will retain a reference to it until
	// Unregister is called.
	Unregister(s Service) error

	// Wait returns a channel which will emit an error if one occurs during
	// startup, an error if the timeout duration elapses before Context.Ready()
	// is called, or nil if the service has Started().
	// If there is nothing to Wait for (i.e. the internal wait group's counter
	// is 0), the channel will return nil immediately.
	WhenReady(timeout time.Duration) <-chan error
}

// Listener allows you to respond to events raised by the Runner in the
// code that owns the Runner, like premature service failure.
//
// Listeners should not be shared between Runners.
//
type Listener interface {
	// OnServiceError should be called when an error occurs in your running service
	// that does not cause the service to End; the service MUST continue
	// running after this error occurs.
	//
	// This is basically where you send errors that don't have an immediately
	// obvious method of handling, that don't terminate the service, but you
	// don't want to swallow entirely. Essentially it defers the decision for
	// what to do about the error to the parent context.
	//
	// Errors should be wrapped using service.WrapError(err, yourSvc) so
	// context information can be applied.
	OnServiceError(service Service, err Error)

	// OnServiceEnd is called when your service ends. If the service responded
	// because it was Halted, err will be nil, otherwise err MUST be set.
	OnServiceEnd(service Service, err Error)

	OnServiceState(service Service, state State)
}

func EnsureHalt(r Runner, s Service, timeout time.Duration) error {
	err := r.Halt(s, timeout)
	if err == nil {
		return nil
	}
	if serr, ok := err.(*errState); ok && serr.Current == Halted {
		return nil
	}
	return err
}

// MustEnsureHalt allows Runner.Halt() to be called in a defer, but only if
// it is acceptable to crash the server if the service does not Halt.
// EnsureHalt is used to prevent an error if the service is already halted.
func MustEnsureHalt(r Runner, service Service, timeout time.Duration) {
	if service == nil {
		return
	}
	if timeout <= 0 {
		panic(fmt.Errorf("service: MustHalt timeout must be > 0"))
	}
	if err := EnsureHalt(r, service, timeout); err != nil {
		panic(err)
	}
}

type runnerState struct {
	changer        *StateChanger
	startingCalled int32
	readyCalled    int32
	halt           chan struct{}
	halted         chan struct{}
}

func (r *runnerState) StartingCalled() bool { return atomic.LoadInt32(&r.startingCalled) == 1 }
func (r *runnerState) SetStartingCalled(v bool) {
	var vi int32
	if v {
		vi = 1
	}
	atomic.StoreInt32(&r.startingCalled, vi)
}

func (r *runnerState) ReadyCalled() bool { return atomic.LoadInt32(&r.readyCalled) == 1 }
func (r *runnerState) SetReadyCalled(v bool) {
	var vi int32
	if v {
		vi = 1
	}
	atomic.StoreInt32(&r.readyCalled, vi)
}

type runner struct {
	// sync.WaitGroup is not adequate for this job as we may call wg.Add() before
	// all wg.Wait() calls have returned.
	wg *errQueue

	listener Listener

	states     map[Service]*runnerState
	statesLock sync.RWMutex
}

func NewRunner(listener Listener) Runner {
	return &runner{
		listener: listener,
		states:   make(map[Service]*runnerState),
		wg:       newErrQueue(),
	}
}

func (r *runner) Services(state State) []Service {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	out := make([]Service, 0, len(r.states))
	for service, rs := range r.states {
		if state == AnyState || state&rs.changer.State() != 0 {
			out = append(out, service)
		}
	}

	return out
}

// StartWait calls a Service's Run() method in a goroutine. It waits until
// the service calls Context.Ready() before returning.
//
// If an error is returned and the service's status is not Halted or Complete,
// you shoud attempt to Halt() the service. If the service does not successfully
// halt, you MUST panic.
//
func (r *runner) StartWait(service Service, timeout time.Duration) (err error) {
	if timeout <= 0 {
		return fmt.Errorf("service: start timeout must be > 0")
	}
	if err := r.Start(service); err != nil {
		return err
	}

	select {
	case err = <-r.WhenReady(timeout):
	}

	return
}

func (r *runner) Start(service Service) (err error) {
	if err = r.Starting(service); err != nil {
		return err
	}

	rs := r.runnerState(service)
	ctx := newContext(service, r.Ready, r.OnError, rs.halt)

	go func() {
		err := service.Run(ctx)
		startingCalled, readyCalled := rs.StartingCalled(), rs.ReadyCalled()
		wasStarted := err != nil

		if wasStarted {
			if rerr := r.ended(service); rerr != nil {
				panic(rerr)
			}
		}

		close(rs.halted)
		if r.listener != nil {
			go r.listener.OnServiceEnd(service, WrapError(err, service))
		}

		if !readyCalled && startingCalled {
			// If the service ended while it was starting, Ready() will never
			// be called.
			r.wg.Put(&serviceError{name: service.ServiceName(), cause: err})
		}
	}()

	return
}

func (r *runner) State(service Service) State {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	rs := r.states[service]
	if rs != nil {
		return rs.changer.State()
	}
	return Halted
}

func (r *runner) runnerState(service Service) *runnerState {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	return r.states[service]
}

func (r *runner) Halt(service Service, timeout time.Duration) error {
	if err := r.Halting(service); err != nil {
		return err
	}

	rs := r.runnerState(service)
	if rs == nil {
		panic("runnerState should not be nil!")
	}
	close(rs.halt)

	after := Timeout(timeout)
	select {
	case <-rs.halted:
	case <-after:
		return errHaltTimeout(0)
	}

	if err := r.Halted(service); err != nil {
		return err
	}
	return nil
}

func (r *runner) HaltAll(timeout time.Duration) error {
	services := r.Services(AnyState)

	for _, service := range services {
		if err := r.Halting(service); err != nil {
			// It's OK if it has already halted - it may have ended while
			// we were iterating.
			if serr, ok := err.(*errState); ok && !serr.Current.IsRunning() {
				continue
			}
			return WrapError(err, service)
		}
		rs := r.runnerState(service)
		close(rs.halt)

		after := Timeout(timeout)
		select {
		case <-rs.halted:
		case <-after:
			return WrapError(errHaltTimeout(0), service)
		}
		if err := r.Halted(service); err != nil {
			return WrapError(err, service)
		}
	}

	return nil
}

func (r *runner) Starting(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	if r.states[service] == nil {
		r.states[service] = &runnerState{
			changer: NewStateChanger(),
		}
	} else {
		r.states[service].SetReadyCalled(false)
		r.states[service].SetStartingCalled(false)
	}

	svc := r.states[service]
	if err := svc.changer.SetStarting(nil); err != nil {
		return err
	}
	r.states[service].SetStartingCalled(true)
	svc.halt = make(chan struct{})
	svc.halted = make(chan struct{})

	r.wg.Add(1)

	if r.listener != nil {
		go r.listener.OnServiceState(service, Starting)
	}
	return nil
}

func (r *runner) OnError(service Service, err error) {
	if r.listener != nil {
		r.listener.OnServiceError(service, WrapError(err, service))
	}
}

func (r *runner) Ready(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	if r.states[service] == nil {
		return errServiceUnknown(0)
	}

	r.states[service].SetReadyCalled(true)
	r.wg.Put(nil)

	var serr *errState
	if err := r.states[service].changer.SetStarted(nil); err != nil {
		var ok bool
		if serr, ok = err.(*errState); ok {
			// State errors don't matter here -
			err = nil
		} else {
			return err
		}
	}
	if serr != nil {
		if r.listener != nil {
			go r.listener.OnServiceState(service, Started)
		}
	}
	return nil
}

// ended is used to bring the state of the service to a Halted state
// if it ends before Halt is called.
func (r *runner) ended(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	if err := r.states[service].changer.SetHalting(nil); IsErrNotRunning(err) {
		return nil
	} else if err != nil {
		return err
	}

	if err := r.states[service].changer.SetHalted(nil); err != nil {
		return err
	}

	if r.listener != nil {
		go r.listener.OnServiceState(service, Halted)
	}

	return nil
}

func (r *runner) Halting(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	return r.halting(service)
}

func (r *runner) halting(service Service) error {
	if r.states[service] == nil {
		return errServiceUnknown(0)
	}
	if err := r.states[service].changer.SetHalting(nil); err != nil {
		return err
	}
	if r.listener != nil {
		go r.listener.OnServiceState(service, Halting)
	}
	return nil
}

func (r *runner) Halted(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	return r.halted(service)
}

func (r *runner) halted(service Service) error {
	if r.states[service] == nil {
		return errServiceUnknown(0)
	}
	if err := r.states[service].changer.SetHalted(nil); err != nil {
		return err
	}
	if r.listener != nil {
		go r.listener.OnServiceState(service, Halting)
	}
	return nil
}

func (r *runner) Unregister(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	if r.states[service] == nil {
		return errServiceUnknown(0)
	}

	state := r.states[service].changer.State()
	if state != Halted {
		return &errState{Halted, Halted, state}
	}
	delete(r.states, service)
	return nil
}

func (r *runner) WhenReady(limit time.Duration) <-chan error {
	var wait <-chan time.Time
	var stop chan struct{}

	if limit > 0 {
		wait = time.After(limit)
		stop = make(chan struct{})
	}

	out := make(chan error, 1)
	var closed int32
	go func() {
		var err error

		errs := r.wg.Wait()
		if len(errs) == 1 {
			err = errs[0]
		} else if len(errs) > 1 {
			err = &serviceErrors{errors: errs}
		}

		if atomic.CompareAndSwapInt32(&closed, 0, 1) {
			if stop != nil {
				close(stop)
			}
			out <- err
			close(out)
		}
	}()

	if wait != nil {
		go func() {
			select {
			case <-stop:
				// This cleans up the goroutine if we return before the timeout
			case <-wait:
				if atomic.CompareAndSwapInt32(&closed, 0, 1) {
					atomic.StoreInt32(&closed, 1)
					out <- errWaitTimeout(0)
					close(out)
				}
			}
		}()
	}

	return out
}
