package service

import (
	"context"
	"time"
)

const MinHaltableSleep = 50 * time.Millisecond

/*
Context is passed to a Service's Run() method. It is used to signal that the
service is ready, to receive the signal to halt, or to relay non-fatal
errors to the Runner's listener.

All services must either include Context.Done() in their select loop, or
regularly poll service.IsDone(ctx) if they don't make use of one.

service.Context is a context.Context, so you can use it anywhere you would
expect to be able to use a context.Context:

	func (s *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}

		dctx, cancel := context.WithDeadline(ctx, time.Now().Add(2 * time.Second))
		defer cancel()

		// This service will be "Done" either when the service is halted,
		// or the deadline arrives (though in the latter case, the service
		// will be considered to have ended prematurely)
		<-dctx.Done()

		return nil
	}

*/
type Context interface {
	context.Context

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
	// MinHaltableSleep is a performance hack. It's probably not a
	// one-size-fits all constant but it'll do for now.
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
