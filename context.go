package service

import (
	"context"
	"sync/atomic"
	"time"
)

const MinHaltableSleep = 50 * time.Millisecond

// Context is passed to a Service's Run() method. It is used to signal that the
// service is ready, to receive the signal to halt, or to relay non-fatal
// errors to the Runner's listener.
//
// All services must either include Context.Done() in their select loop, or
// regularly poll service.IsDone(ctx) if they don't make use of one.
//
type Context interface {
	context.Context

	// Ready MUST be called by all services when they have finished
	// their setup routines and are considered "Ready" to run.
	Ready() error

	// OnError is used to pass all non-fatal errors that do not cause the
	// service to halt prematurely up to the runner's listener.
	OnError(err error)
}

var _ context.Context = &svcContext{}

// Sleep allows a service to perform an interruptible sleep - it will
// return early if the service is halted.
func Sleep(ctx Context, d time.Duration) (halted bool) {
	if d < MinHaltableSleep {
		time.Sleep(d)
		select {
		case <-ctx.Done():
			return true
		default:
			return false
		}
	}
	select {
	case <-time.After(d):
		return false
	case <-ctx.Done():
		return true
	}
}

type RunContext interface {
	Context

	// Halt stops the context. It must be safe to call Halt() more than once.
	// Halt() does not block until the service is halted.
	Halt()

	WhenReady(func(svc Service) error) RunContext
	WhenError(func(svc Service, err error)) RunContext
}

type (
	readyFunc func(service Service) error
	errFunc   func(service Service, err error)
)

var emptyErrFunc = func(service Service, err error) {}
var emptyReadyFunc = func(service Service) error { return nil }

type runContext struct {
	svcContext
	halted int32
}

func (f *runContext) Halt() {
	if atomic.CompareAndSwapInt32(&f.halted, 0, 1) {
		close(f.done)
	}
}

func (f *runContext) WhenReady(readyFunc func(svc Service) error) RunContext {
	if readyFunc != nil {
		f.readyFunc = readyFunc
	} else {
		f.readyFunc = emptyReadyFunc
	}
	return f
}

func (f *runContext) WhenError(errFunc func(svc Service, err error)) RunContext {
	if errFunc != nil {
		f.errFunc = errFunc
	} else {
		f.errFunc = emptyErrFunc
	}
	return f
}

// Standalone returns a RunContext you can pass to Service.Run() if you
// want to run the service outside a Runner.
//
// If you want to capture ready signals, see RunContext.WhenReady(). To capture
// non-halting error signals, see RunContext.WhenError().
//
func Standalone() RunContext {
	ctx := &runContext{
		svcContext: svcContext{
			done:      make(chan struct{}),
			readyFunc: emptyReadyFunc,
			errFunc:   emptyErrFunc,
		},
	}
	return ctx
}

type svcContext struct {
	service   Service
	readyFunc readyFunc
	errFunc   errFunc
	done      chan struct{}
}

func newSvcContext(service Service, readyFunc readyFunc, errFunc errFunc, done chan struct{}) Context {
	return &svcContext{
		service:   service,
		done:      done,
		readyFunc: readyFunc,
		errFunc:   errFunc,
	}
}

func (c *svcContext) Deadline() (deadline time.Time, ok bool) { return }

func (c *svcContext) Err() error {
	if IsDone(c) {
		return context.Canceled
	}
	return nil
}

func (c *svcContext) Value(key interface{}) interface{} { return nil }

func (c *svcContext) Ready() error { return c.readyFunc(c.service) }

func (c *svcContext) OnError(err error) { c.errFunc(c.service, err) }

func (c *svcContext) Done() <-chan struct{} { return c.done }

// IsDone returns true if the context has been cancelled.
//
// If you have a service which does not use a for/select block, you can
// poll the context with this method.
func IsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
