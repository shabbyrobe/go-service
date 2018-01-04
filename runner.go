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

	// StartWait is a shorthand for calling Start() then WhenReady()
	StartWait(s Service, timeout time.Duration) error

	// Start a service in this runner. The runner will retain a reference to it
	// until Unregister is called even if the service is Halted.
	Start(s Service) error

	// Halt a service started in this runner. The runner will retain a
	// reference to it until Unregister is called.
	Halt(s Service, timeout time.Duration) error

	// HaltAll halts all services started in this runner. The runner will retain
	// references to the services until Unregister is called.
	HaltAll(timeout time.Duration) error

	// Services returns a list of services currently registered or running at
	// the time of the call. If State is provided, only services matching the
	// state are returned.
	Services(state State)

	// Register instructs the runner to retain the service after it has ended.
	// Services that are not registered are not retained.
	// Register must return the runner upon which it was called.
	Register(s Service) Runner

	// Unregister unregisters a service that has been registered in this runner.
	// If the service is running, it will not be unregistered immediately, but will
	// be Unregistered when it stops, either by halting or by erroring.
	Unregister(s Service) error

	// WhenReady blocks until the service is ready or an error occurs.
	// It will unblock when one of the following conditions is met:
	//
	//  - The service's state is not Starting
	//	- The service calls ctx.Ready()
	//	- The service returns before calling ctx.Ready()
	//	- The service is halted before callig ctx.Ready()
	//	- The timeout elapses
	//	- The service is not known to the runner. This case does not return an
	//	  error because an unregistered service may halt before WhenReady can be
	//	  called.
	//
	// It returns nil if ctx.Ready() was successfully called, and an error in
	// all other cases.
	WhenReady(s Service, timeout time.Duration) error
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
	} else if IsErrServiceUnknown(err) {
		return nil
	}
	return err
}

// MustEnsureHalt allows Runner.Halt() to be called in a defer, but only if
// it is acceptable to crash the server if the service does not Halt.
// EnsureHalt is used to prevent an error if the service is already halted.
func MustEnsureHalt(r Runner, s Service, timeout time.Duration) {
	if s == nil {
		return
	}
	if timeout <= 0 {
		panic(fmt.Errorf("service: MustHalt timeout must be > 0"))
	}
	if err := EnsureHalt(r, s, timeout); err != nil {
		panic(err)
	}
}

func WhenAllReady(r Runner, timeout time.Duration, ss ...Service) error {
	var errs []error
	for _, s := range ss {
		if err := r.WhenReady(s, timeout); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return &serviceErrors{errors: errs}
	}
	return nil
}

type runnerState struct {
	changer        *stateChanger
	startingCalled int32
	readyCalled    int32
	retain         bool
	ready          chan error
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
	listener Listener

	states     map[Service]*runnerState
	statesLock sync.RWMutex
}

func NewRunner(listener Listener) Runner {
	return &runner{
		listener: listener,
		states:   make(map[Service]*runnerState),
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

	return r.WhenReady(service, timeout)
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

		ready := rs.ready
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
			ready <- &serviceError{name: service.ServiceName(), cause: err}
			close(ready)
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

	var svc = r.states[service]
	if svc == nil {
		svc = newRunnerState()
		r.states[service] = svc
	} else {
		svc.SetReadyCalled(false)
		svc.SetStartingCalled(false)
	}

	if err := svc.changer.SetStarting(nil); err != nil {
		return err
	}

	svc.SetStartingCalled(true)
	svc.ready = make(chan error, 1)
	svc.halt = make(chan struct{})
	svc.halted = make(chan struct{})

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

	rs := r.states[service]
	rs.SetReadyCalled(true)
	rs.ready <- nil
	close(rs.ready)

	var serr *errState
	if err := rs.changer.SetStarted(nil); err != nil {
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
	rs := r.states[service]
	if rs == nil {
		return errServiceUnknown(0)
	}
	if err := rs.changer.SetHalted(nil); err != nil {
		return err
	}
	if !rs.retain {
		delete(r.states, service)
	}
	if r.listener != nil {
		go r.listener.OnServiceState(service, Halting)
	}
	return nil
}

func (r *runner) Register(service Service) Runner {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]
	if rs == nil {
		rs = newRunnerState()
		r.states[service] = rs
	}
	rs.retain = true
	return r
}

func (r *runner) Unregister(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]
	if rs == nil {
		return errServiceUnknown(0)
	}

	state := rs.changer.State()
	if state != Halted {
		rs.retain = false
	} else {
		delete(r.states, service)
	}
	return nil
}

func (r *runner) WhenReady(service Service, limit time.Duration) error {
	errc := func() chan error {
		r.statesLock.Lock()
		defer r.statesLock.Unlock()

		rs := r.states[service]
		if rs == nil {
			return nil
		}
		return rs.ready
	}()

	if errc == nil {
		return nil
	}

	var wait <-chan time.Time
	if limit > 0 {
		wait = time.After(limit)
	}

	select {
	case <-wait:
		return errWaitTimeout(0)
	case err := <-errc:
		return err
	}
}

func newRunnerState() *runnerState {
	return &runnerState{
		changer: newStateChanger(),
	}
}
