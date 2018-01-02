package service

import "time"

const MinHaltableSleep = 50 * time.Millisecond

// Context is passed to a Service's Run() method. It is used to signal
// that the service is ready, to receive the signal to halt or to relay
// non-fatal errors to the Runner's listener.
type Context interface {
	Halt() <-chan struct{}
	Halted() bool
	Ready() error

	// OnError is used to report a non-fatal error that does not cause
	// the service to halt prematurely to the Listener.
	OnError(err error)
}

// Sleep allows a service to perform an interruptible sleep - it will
// return early if the service is halted.
func Sleep(ctx Context, d time.Duration) (halted bool) {
	if d < MinHaltableSleep {
		time.Sleep(d)
		select {
		case <-ctx.Halt():
			return true
		default:
			return false
		}
	}
	select {
	case <-time.After(d):
		return false
	case <-ctx.Halt():
		return true
	}
}

type (
	readyFunc func(service Service) error
	errFunc   func(service Service, err error)
)

func newContext(service Service, readyFunc readyFunc, errFunc errFunc, halter chan struct{}) Context {
	return &context{
		service:   service,
		halt:      halter,
		readyFunc: readyFunc,
		errFunc:   errFunc,
	}
}

type context struct {
	service   Service
	readyFunc readyFunc
	errFunc   errFunc
	halt      chan struct{}
}

func (c *context) Ready() error { return c.readyFunc(c.service) }

func (c *context) OnError(err error) { c.errFunc(c.service, err) }

func (c *context) Halt() <-chan struct{} { return c.halt }

func (c *context) Halted() bool {
	select {
	case <-c.halt:
		return true
	default:
		return false
	}
}
