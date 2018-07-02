package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Runner Starts, Halts and manages Services.
type Runner interface {
	State(svc Service) State

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

	// Shutdown halts all services started in this runner and prevents new ones from
	// being started.
	//
	// If any service fails to halt, err will contain an error for each service
	// that failed, accessible by calling service.Errors(err). n will contain
	// the number of services successfully halted.
	//
	// Timeout must be > 0.
	//
	// If errlimit is > 0, Shutdown will stop after that many errors occur.
	//
	// It is safe to call Shutdown multiple times.
	Shutdown(timeout time.Duration, errlimit int) (n int, err error)

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

// StartWait calls a Service's Run() method in a goroutine. It waits until
// the service calls Context.Ready() before returning. It is a shorthand for
// calling Start() with a ReadySignal, then waiting for the ReadySignal.
//
// If an error is returned and the service's status is not Halted or Complete,
// you shoud attempt to Halt() the service. If the service does not successfully
// halt, you should panic as resources have been lost.
//
// Timeout must be > 0. If a timeout occurs, the error returned can be checked
// using service.IsErrTimeout(). If StartWait times out and you do not have a
// mechanism to recover and kill the Service, you MUST panic.
//
func StartWait(runner Runner, timeout time.Duration, service Service) error {
	if timeout <= 0 {
		return fmt.Errorf("service: start timeout must be > 0")
	}
	sr := NewReadySignal()
	if err := runner.Start(service, sr); err != nil {
		return err
	}
	return WhenReady(timeout, sr)
}

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

	suspended int32 // boolean; access using atomic.LoadInt32

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
	defer r.statesLock.Unlock()

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

	return out
}

func (r *runner) Start(svc Service, ready ReadySignal) (rerr error) {
	if svc == nil {
		return fmt.Errorf("nil service")
	}
	rs, err := r.starting(svc, ready)
	if err != nil {
		return err
	}

	ctx := newSvcContext(svc, r.Ready, r.OnError, rs.done)

	go func() {
		rerr := svc.Run(ctx)
		if err := rs.Ended(svc, rerr, r.listener, r.stateListener, r.removeService); err != nil {
			panic(err)
		}
	}()

	return
}

func (r *runner) State(svc Service) (state State) {
	r.statesLock.Lock()
	rs := r.states[svc]
	if rs != nil {
		state = rs.State()
	} else {
		state = Halted
	}
	r.statesLock.Unlock()
	return state
}

func (r *runner) runnerService(svc Service) (rs *runnerService) {
	r.statesLock.Lock()
	rs = r.states[svc]
	r.statesLock.Unlock()
	return rs
}

func (r *runner) Halt(timeout time.Duration, svc Service) error {
	if timeout <= 0 {
		return fmt.Errorf("service: halt timeout must be > 0")
	}
	if svc == nil {
		return fmt.Errorf("nil service")
	}

	rs := r.runnerService(svc)
	if rs == nil {
		return errServiceUnknown(0)
	}

	fromState, err := rs.Halting(svc, r.stateListener)
	if err != nil {
		return err
	}

	initiator := fromState != Halting

	if err := rs.EndWait(timeout); err != nil {
		return err
	}

	if initiator {
		if _, err := rs.Halted(svc, r.stateListener, false, r.removeService); err != nil {
			return err
		}
	}

	return nil
}

func (r *runner) removeService(svc Service) {
	r.statesLock.Lock()
	delete(r.states, svc)
	r.statesLock.Unlock()
}

func (r *runner) Suspend() (success bool) {
	return atomic.CompareAndSwapInt32(&r.suspended, 0, 1)
}

func (r *runner) Suspended() bool {
	return atomic.LoadInt32(&r.suspended) == 1
}

func (r *runner) Shutdown(timeout time.Duration, errlimit int) (n int, rerr error) {
	if !r.Suspend() {
		return 0, nil
	}

	if timeout <= 0 {
		return 0, fmt.Errorf("service: halt timeout must be > 0")
	}

	services := r.Services(AnyState, 0)

	var errors []error

	for _, service := range services {
		if errlimit > 0 && len(errors) == errlimit {
			break
		}

		if err := r.Halt(timeout, service); err != nil {
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

			errors = append(errors, &serviceError{cause: err, name: service.ServiceName()})
			continue
		}

		n++
	}

	if len(errors) > 0 {
		return n, &serviceErrors{errors: errors}
	}

	return n, nil
}

func (r *runner) starting(svc Service, ready ReadySignal) (rs *runnerService, rerr error) {
	if atomic.LoadInt32(&r.suspended) == 1 {
		return nil, errRunnerSuspended(1)
	}

	r.statesLock.Lock()
	defer r.statesLock.Unlock()

	rsNew := false
	rs = r.states[svc]
	if rs == nil {
		rsNew = true
		rs = newRunnerService()
	}

	if _, err := rs.Starting(svc, r.stateListener); err != nil {
		return nil, err
	}
	if rsNew {
		r.states[svc] = rs
	}
	rs.resetStarting(ready)

	return rs, nil
}

func (r *runner) OnError(service Service, err error) {
	if r.errListener != nil {
		go r.errListener.OnServiceError(service, WrapError(err, service))
	}
}

func (r *runner) Ready(svc Service) error {
	rs := r.runnerService(svc)
	if rs == nil {
		return errServiceUnknown(0)
	}

	rs.Ready(nil)

	if _, err := rs.Started(svc, r.stateListener); err != nil {
		if _, ok := err.(*errState); ok {
			// State errors don't matter here
			err = nil
		} else {
			return err
		}
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
