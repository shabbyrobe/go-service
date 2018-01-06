package service

import "time"

const MinHaltableSleep = 50 * time.Millisecond

// Context is passed to a Service's Run() method. It is used to signal
// that the service is ready, to receive the signal to halt or to relay
// non-fatal errors to the Runner's listener.
type Context interface {
	// Done returns a channel which will be closed when the service should
	// stop. All services should either include this channel in their select
	// loop, or regularly poll IsDone().
	// It is safe to add this channel to more than one select loop.
	Done() <-chan struct{}

	// IsDone returns true if the service has been instructed to halt by
	// its runner. All services should either regularly poll this, or
	// include Done() in their select loop.
	IsDone() bool

	// Ready MUST be called by all services when they have finished
	// their setup routines and are considered "Ready" to run.
	Ready() error

	// OnError is used to pass all non-fatal errors that do not cause the
	// service to halt prematurely up to the runner's listener.
	OnError(err error)
}

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

type (
	readyFunc func(service Service) error
	errFunc   func(service Service, err error)
)

func newContext(service Service, readyFunc readyFunc, errFunc errFunc, done chan struct{}) Context {
	return &context{
		service:   service,
		done:      done,
		readyFunc: readyFunc,
		errFunc:   errFunc,
	}
}

type context struct {
	service   Service
	readyFunc readyFunc
	errFunc   errFunc
	done      chan struct{}
}

func (c *context) Ready() error { return c.readyFunc(c.service) }

func (c *context) OnError(err error) { c.errFunc(c.service, err) }

func (c *context) Done() <-chan struct{} { return c.done }

func (c *context) IsDone() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}
