package service

import (
	"fmt"
	"sync"
	"time"
)

// Runner Starts, Halts and manages Services.
type Runner interface {
	State(svc Service) State

	// StartWait calls a Service's Run() method in a goroutine. It waits until
	// the service calls Context.Ready() before returning. It is a shorthand for
	// calling Start() with a ReadySignal, then waiting for the ReadySignal.
	//
	// If an error is returned and the service's status is not Halted or Complete,
	// you shoud attempt to Halt() the service. If the service does not successfully
	// halt, you MUST panic.
	//
	// Timeout must be > 0. If a timeout occurs, the error returned can be checked
	// using service.IsErrTimeout(). If StartWait times out and you do not have a
	// mechanism to recover and kill the Service, you MUST panic.
	//
	StartWait(timeout time.Duration, svc Service) error

	// Start a service in this runner.
	//
	// ReadySignal will be called when one of the following conditions is met:
	//
	//	- The service calls ctx.Ready()
	//	- The service returns before calling ctx.Ready()
	//	- The service is halted before calling ctx.Ready()
	//
	// ReadySignal is called with nil if ctx.Ready() was successfully called,
	// and an error in all other cases.
	//
	// ReadySignal may be nil.
	//
	Start(svc Service, ready ReadySignal) error

	// Halt a service started in this runner. The runner will retain a
	// reference to it until Unregister is called.
	//
	// Timeout must be > 0.
	Halt(timeout time.Duration, svc Service) error

	// HaltAll halts all services started in this runner. The runner will retain
	// references to the services until Unregister is called.
	//
	// If any service fails to halt, err will contain an error for each service
	// that failed, accessible by calling service.Errors(err). n will contain
	// the number of services successfully halted.
	//
	// Timeout must be > 0.
	//
	// If errlimit is > 0, HaltAll will stop after that many errors occur.
	HaltAll(timeout time.Duration, errlimit int) (n int, err error)

	// Services returns a list of services currently registered or running at
	// the time of the call. If State is provided, only services matching the
	// state are returned.
	//
	// Pass limit to restrict the number of returned results. If limit is <= 0,
	// all matching services are returned.
	Services(state StateQuery, limit int) []Service

	// Register instructs the runner to retain the service after it has ended.
	// Services that are not registered are not retained.
	// Register must return the runner upon which it was called.
	Register(svc Service) Runner

	// Unregister unregisters a service that has been registered in this runner.
	// If the service is running, it will not be unregistered immediately, but will
	// be Unregistered when it stops, either by halting or by erroring.
	//
	// If the unregister was deferred, this will be returned in the first return arg.
	Unregister(svc Service) (deferred bool, err error)
}

type Stage int

const (
	StageReady Stage = 1
	StageRun   Stage = 2
)

func EnsureHalt(r Runner, timeout time.Duration, s Service) error {
	err := r.Halt(timeout, s)
	if err == nil {
		return nil
	}

	if serr, ok := err.(*errState); ok && !serr.Current.IsRunning() {
		// Halted and halting are both ignored - if it's Halted, yep obviously
		// we're fine. If it's Halting, we should assume something else will
		// take responsibility for failing if the Halt doesn't complete and
		// return here.
		return nil
	} else if IsErrServiceUnknown(err) {
		// If the service is not retained, it will not be known when we try
		// to halt it. This is fine.
		return nil
	}

	return err
}

// MustEnsureHalt allows Runner.Halt() to be called in a defer, but only if
// it is acceptable to crash the server if the service does not Halt.
// EnsureHalt is used to prevent an error if the service is already halted.
func MustEnsureHalt(r Runner, timeout time.Duration, s Service) {
	if s == nil {
		return
	}
	if timeout <= 0 {
		panic(fmt.Errorf("service: MustHalt timeout must be > 0"))
	}
	if err := EnsureHalt(r, timeout, s); err != nil {
		panic(err)
	}
}

type runner struct {
	listener      Listener
	errListener   ErrorListener
	stateListener StateListener

	states     map[Service]*runnerService
	statesLock sync.RWMutex
}

func NewRunner(listener Listener) Runner {
	el, _ := listener.(ErrorListener)
	sl, _ := listener.(StateListener)

	return &runner{
		listener:      listener,
		errListener:   el,
		stateListener: sl,
		states:        make(map[Service]*runnerService),
	}
}

func (r *runner) Services(query StateQuery, limit int) []Service {
	r.statesLock.Lock()

	if limit <= 0 {
		limit = len(r.states)
	}

	// FIXME: possibly allocates way too much for the general case when
	// limit is not passed.
	out := make([]Service, 0, limit)

	n := 0
	for service, rs := range r.states {
		state, retain := rs.AllState()
		if query.Match(state, retain) {
			out = append(out, service)
			n++
			if n >= limit {
				break
			}
		}
	}

	r.statesLock.Unlock()

	return out
}

func (r *runner) StartWait(timeout time.Duration, service Service) error {
	if timeout <= 0 {
		return fmt.Errorf("service: start timeout must be > 0")
	}
	sr := NewReadySignal()
	if err := r.Start(service, sr); err != nil {
		return err
	}
	return WhenReady(timeout, sr)
}

func (r *runner) Start(service Service, ready ReadySignal) (err error) {
	if service == nil {
		return fmt.Errorf("nil service")
	}
	if err = r.starting(service, ready); err != nil {
		return err
	}

	rs := r.runnerService(service)
	ctx := newSvcContext(service, r.Ready, r.OnError, rs.done)

	go func() {
		// Careful! Anything you touch in the runnerService in here must take
		// care to synchronise.

		err := service.Run(ctx)

		// By the time we hit this point, the service should no longer be
		// running at all if it was started. A badly coded service may have
		// stray goroutines blocking on ctx.Done(), but if they are waiting
		// at a call to '<-ctx.Done()', they will be waiting on the old
		// channel, which should be closed when ended() calls shutdown().
		startingCalled, readyCalled := rs.Calls()
		wasStarted := err != nil

		ready := rs.ReadySignal()
		if wasStarted {
			if rerr := r.ended(service); rerr != nil {
				panic(rerr)
			}
		}

		rs.Halted()

		stage := StageRun
		if !readyCalled {
			stage = StageReady
		}

		if !readyCalled && startingCalled && ready != nil {
			// If the service ended while it was starting, Ready() will never
			// be called.
			ready.Done(&serviceError{name: service.ServiceName(), cause: err})
		}

		if r.listener != nil {
			go r.listener.OnServiceEnd(stage, service, WrapError(err, service))
		}

	}()

	return
}

func (r *runner) State(service Service) State {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	rs := r.states[service]
	if rs != nil {
		return rs.State()
	}
	return Halted
}

func (r *runner) runnerService(service Service) *runnerService {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	return r.states[service]
}

func (r *runner) Halt(timeout time.Duration, service Service) error {
	if timeout <= 0 {
		return fmt.Errorf("service: halt timeout must be > 0")
	}
	if service == nil {
		return fmt.Errorf("nil service")
	}

	fmt.Printf("halt halting %p\n", service)
	if err := r.Halting(service); err != nil {
		return err
	}

	rs := r.runnerService(service)
	fmt.Printf("halt pre-done %p\n", service)
	if rs == nil {
		panic(fmt.Errorf("runnerService should not be nil! %p", service))
	}
	rs.Done()

	after := time.After(timeout)
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

func (r *runner) HaltAll(timeout time.Duration, errlimit int) (n int, rerr error) {
	if timeout <= 0 {
		return 0, fmt.Errorf("service: halt timeout must be > 0")
	}

	services := r.Services(AnyState, 0)

	var errors []error

	for _, service := range services {
		if errlimit > 0 && len(errors) == errlimit {
			break
		}

		if err := r.Halting(service); err != nil {
			// It's OK if it has already halted - it may have ended while
			// we were iterating.
			if serr, ok := err.(*errState); ok && !serr.Current.IsRunning() {
				continue
			}

			// Similarly, if it's an unknown service, it may have ended and
			// been Unregistered in between Services() being called and
			// the service halting.
			if IsErrServiceUnknown(err) {
				continue
			}

			errors = append(errors, err)
			continue
		}

		rs := r.runnerService(service)
		rs.Done()

		after := time.After(timeout)
		select {
		case <-rs.halted:
		case <-after:
			errors = append(errors, errHaltTimeout(0))
			continue
		}
		if err := r.Halted(service); err != nil {
			errors = append(errors, err)
			continue
		}

		n++
	}

	if len(errors) > 0 {
		return n, &serviceErrors{errors: errors}
	}

	return n, nil
}

func (r *runner) starting(service Service, ready ReadySignal) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	var rs = r.states[service]
	if rs == nil {
		rs = newRunnerService()
		r.states[service] = rs
	}

	if _, err := rs.SetState(Starting); err != nil {
		return err
	}

	rs.resetStarting(ready)

	if r.stateListener != nil {
		go r.stateListener.OnServiceState(service, Starting)
	}
	return nil
}

func (r *runner) OnError(service Service, err error) {
	if r.errListener != nil {
		go r.errListener.OnServiceError(service, WrapError(err, service))
	}
}

func (r *runner) Ready(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()
	if r.states[service] == nil {
		return errServiceUnknown(0)
	}

	rs := r.states[service]
	rs.Ready()

	var serr *errState
	if _, err := rs.SetState(Started); err != nil {
		var ok bool
		if serr, ok = err.(*errState); ok {
			// State errors don't matter here -
			err = nil
		} else {
			return err
		}
	}
	if serr != nil && r.stateListener != nil {
		go r.stateListener.OnServiceState(service, Started)
	}
	return nil
}

func (r *runner) Halting(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	if r.states[service] == nil {
		return errServiceUnknown(0)
	}
	old, err := r.states[service].SetState(Halting)
	fmt.Println(old)
	if err != nil {
		return err
	}
	if r.stateListener != nil {
		go r.stateListener.OnServiceState(service, Halting)
	}
	return nil
}

func (r *runner) Halted(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]
	if rs == nil {
		// This should not be an error - Halting() should catch any situation
		// where this matters. If we are hitting this code, we may have simultaneous
		// calls to Halt() waiting for the service to finish.
		return nil
	}
	return r.shutdown(rs, service)
}

// ended is used to bring the state of the service to a Halted state
// if it ends before Halt is called.
func (r *runner) ended(service Service) error {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]

	if _, err := rs.SetState(Halting); IsErrNotRunning(err) {
		return nil
	} else if err != nil {
		return err
	}

	rs.Done()

	return r.shutdown(rs, service)
}

// shutdown assumes r.statesLock is acquired.
func (r *runner) shutdown(rs *runnerService, service Service) error {
	if _, err := rs.SetState(Halted); err != nil {
		return err
	}
	if !rs.Retain() {
		fmt.Printf("shutdown %p\n", service)
		delete(r.states, service)
	}
	if r.stateListener != nil {
		go r.stateListener.OnServiceState(service, Halted)
	}
	return nil
}

func (r *runner) Register(service Service) Runner {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]
	if rs == nil {
		rs = newRunnerService()
		r.states[service] = rs
	}
	rs.SetRetain(true)
	return r
}

func (r *runner) Unregister(service Service) (deferred bool, rerr error) {
	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rs := r.states[service]
	if rs == nil {
		return false, errServiceUnknown(0)
	}

	state := rs.state
	deferred = state != Halted

	if deferred {
		rs.SetRetain(false)
	} else {
		delete(r.states, service)
	}
	return deferred, nil
}
