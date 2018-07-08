package service

import (
	"context"
	"fmt"
	"sync"
)

// Runner Starts, Halts and manages Services.
type Runner interface {
	// Start a service in this runner asynchronously.
	//
	// This method is intended as a building-block; if you want to start a
	// service synchronously, see service.StartWait().
	//
	// An optional context can be provided via ctx; this allows cancellation to
	// be declared outside the Runner. You may provide a nil Context.
	//
	// An optional Signal can be provided to wait for the service to signal it
	// is "Ready", which will be called when one of the following conditions is
	// met:
	//
	//	- The service calls ctx.Ready()
	//	- The service returns before calling ctx.Ready()
	//	- The service is halted before calling ctx.Ready()
	//	- The provided context.Context is cancelled.
	//
	// Signal.Done() is called with nil if ctx.Ready() was successfully called,
	// and an error in all other cases.
	//
	// Signal may be nil.
	//
	Start(ctx context.Context, svc *Service, ready Signal) (Handle, error)

	// Halt a service started in this runner asynchronously.
	//
	// This method is intended as a building-block; if you want to start a
	// service synchronously, see service.HaltWait().
	//
	// An optional context can be provided via ctx; this allows cancellation to
	// be declared outside the Runner. You may provide a nil Context.
	//
	// If the context is cancelled before the service halts, you may have leaked
	// a goroutine; there is no way for a service lost in this way to be
	// recovered using this library, you will need to build in your own recovery
	// mechanisms if you want to handle this condition. In practice, a
	// cancelled 'halt' is probably a good time to panic(), but your specific
	// application may be able to tolerate some goroutine leaks until you can
	// fix the issue.
	//
	// An optional Signal can be provided to wait for the service to signal it
	// is done halting, which will be called when one of the following
	// conditions is met:
	//
	//	- The service calls ctx.Ready()
	//	- The service returns before calling ctx.Ready()
	//	- The service is halted before calling ctx.Ready()
	//	- The provided context.Context is cancelled.
	//
	// Signal.Done() is called with nil if the service was successfully halted,
	// and an error in all other cases.
	//
	// Signal may be nil.
	Halt(ctx context.Context, svc *Service, done Signal) error

	// Shutdown halts all services started in this runner and prevents new ones
	// from being started.
	//
	// If any service fails to halt, err will contain an error for each service
	// that failed, accessible by calling service.Errors(err). n will contain
	// the number of services successfully halted.
	//
	// An optional context can be provided via ctx; this allows cancellation to
	// be declared outside the Runner. You may provide a nil Context, but this is
	// not recommended as your application may block indefinitely.
	//
	// It is safe to call Shutdown multiple times.
	Shutdown(ctx context.Context) (err error)

	// Enable resumes a Shutdown runner.
	Enable() error

	// Suspend prevents new services from being started in this Runner, but
	// does not shut down existing services.
	Suspend() error

	State(svc *Service) (state State)

	// Services returns the list of services running at the time of the call.
	// time of the call. If StateQuery is provided, only the matching services
	// are returned.
	//
	// Pass limit to restrict the number of returned results. If limit is <= 0,
	// all matching services are returned.
	//
	// You can instruct States to allocate into an existing slice by passing
	// it in. You should replace it with the return value in case it needs
	// to grow.
	Services(state State, limit int, into []ServiceInfo) []ServiceInfo
}

type OnEnd func(stage Stage, service *Service, err error)
type OnError func(stage Stage, service *Service, err error)

type RunnerOption func(rn *runner)

func RunnerOnEnd(cb OnEnd) RunnerOption {
	return func(rn *runner) { rn.onEnd = cb }
}

func RunnerOnError(cb OnError) RunnerOption {
	return func(rn *runner) { rn.onError = cb }
}

type runner struct {
	onEnd   OnEnd
	onError OnError

	services map[*Service]*runnerService
	state    RunnerState
	lock     sync.RWMutex
}

var _ Runner = &runner{}

func NewRunner(opts ...RunnerOption) Runner {
	rn := &runner{
		services: make(map[*Service]*runnerService),
	}
	for _, o := range opts {
		o(rn)
	}
	return rn
}

func (r *runner) Enable() error {
	r.lock.Lock()
	r.state = RunnerEnabled
	r.lock.Unlock()
	return nil
}

func (r *runner) Suspend() error {
	r.lock.Lock()
	if r.state != RunnerEnabled {
		r.lock.Unlock()
		// FIXME: error that allows you to check if it's suspended or shut down:
		return fmt.Errorf("runner is not enabled")
	}
	r.state = RunnerSuspended
	r.lock.Unlock()
	return nil
}

func (r *runner) Shutdown(ctx context.Context) (rerr error) {
	r.lock.Lock()
	defer r.lock.Unlock()

	if r.state != RunnerEnabled && r.state != RunnerSuspended {
		// FIXME: error that allows you to check if it's suspended or shut down:
		return fmt.Errorf("runner is not enabled")
	}

	signal := NewMultiSignal(len(r.services))

	r.state = RunnerShutdown

	for _, rs := range r.services {
		if err := rs.halting(ctx, signal); err != nil {
			panic(err)
		}
	}

	select {
	case err := <-signal.Waiter():
		return err

	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *runner) Start(ctx context.Context, svc *Service, ready Signal) (h Handle, rerr error) {
	h = nilHandle
	if svc == nil {
		return h, fmt.Errorf("nil service")
	}

	var rs *runnerService
	{
		r.lock.Lock()
		if r.state != RunnerEnabled {
			r.lock.Unlock()

			// FIXME: error that allows you to check if it's suspended or shut down:
			return h, fmt.Errorf("runner is not enabled")
		}

		rs = r.services[svc]
		if rs == nil {
			rs = newRunnerService(r, svc, ready)
			r.services[svc] = rs
		}
		r.lock.Unlock()
	}

	if err := rs.starting(ctx); err != nil {
		return h, err
	}
	h = &handle{r: r, svc: svc}

	go func() {
		rerr := svc.Runnable.Run(rs)
		if err := rs.ended(rerr); err != nil {
			panic(err)
		}
	}()

	return h, nil
}

func (r *runner) Halt(ctx context.Context, svc *Service, done Signal) (rerr error) {
	if svc == nil {
		return fmt.Errorf("nil service")
	}

	r.lock.Lock()
	rs := r.services[svc]
	if rs == nil {
		r.lock.Unlock()
		return nil
	}
	r.lock.Unlock()

	return rs.halting(ctx, done)
}

func (r *runner) Services(query State, limit int, into []ServiceInfo) []ServiceInfo {
	r.lock.Lock()
	defer r.lock.Unlock()

	if limit <= 0 {
		limit = len(r.services)
	}

	if len(into) == 0 {
		into = make([]ServiceInfo, 0, limit)
	}

	n := 0
	for service, rs := range r.services {
		state := rs.State()
		if state.Match(query) {
			into = append(into, ServiceInfo{
				State:   state,
				Service: service,
			})
			n++
			if n >= limit {
				break
			}
		}
	}

	return into
}

func (r *runner) State(svc *Service) (state State) {
	r.lock.Lock()
	rs := r.services[svc]
	r.lock.Unlock()

	if rs != nil {
		state = rs.State()
	} else {
		state = Halted
	}
	return state
}

func (r *runner) raiseOnEnd(stage Stage, service *Service, err error) {
	r.lock.Lock()
	rend := r.onEnd
	r.lock.Unlock()

	if rend != nil {
		rend(stage, service, err)
	}
	if service.OnEnd != nil {
		service.OnEnd(stage, service, err)
	}
}

func (r *runner) raiseOnError(stage Stage, service *Service, err error) {
	r.lock.Lock()
	rerror := r.onError
	r.lock.Unlock()
	if rerror != nil {
		rerror(stage, service, err)
	}
}

type ServiceInfo struct {
	State      State
	Registered bool
	Service    *Service
}
