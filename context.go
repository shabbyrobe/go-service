package service

import (
	"context"
	"time"
)

// MinHaltableSleep specifies the minimum amount of time that you must
// pass to service.Sleep() if you want the Sleep() to be cancellable
// from a context. Calls to service.Sleep() with a duration smaller than
// this will simply call time.Sleep().
const MinHaltableSleep = 50 * time.Millisecond

/*
Context is passed to a Service's Run() method. It is used to signal that the
service is ready, to receive the signal to halt, or to relay non-fatal
errors to the Runner's listener.

All services must either include Context.Done() in their select loop, or
regularly poll ctx.ShouldHalt() if they don't make use of one.

service.Context is a context.Context, so you can use it anywhere you would
expect to be able to use a context.Context and the thing you are using will
be signalled when your service is halted:

	func (s *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}

		rqCtx, cancel := context.WithDeadline(ctx, time.Now().Add(2 * time.Second))
		defer cancel()

		client := &http.Client{}
		rq, err := http.NewRequest("GET", "http://example.com", nil)
		// ...

		// The child context passed to the request will be "Done" either when
		// the service is halted, or the deadline arrives, so this request will
		// be aborted by the service being Halted:
		rq = rq.WithContext(rqCtx)
		rs, err := client.Do(rq)
		// ...

		<-ctx.Done()

		return nil
	}

*/
type Context interface {
	context.Context

	// Ready MUST be called by all services when they have finished
	// their setup routines and are considered "Ready" to run. If
	// Ready() returns an error, it MUST be immediately returned.
	Ready() error

	// ShouldHalt is used in situations where you can not block listening
	// to <-Done(), but instead must poll to check if Run() should halt.
	// Checking <-Done() in a select{} is preferred, but not always practical.
	ShouldHalt() bool

	// OnError is used to pass all non-fatal errors that do not cause the
	// service to halt prematurely up to the runner's listener.
	OnError(err error)

	// Runner allows you to access the Runner from which the invocation of
	// Run() originated. This lets you start child services in the same runner.
	// It is safe to call any method of this from inside a Runnable.
	Runner() Runner
}

// Sleep allows a Runnable to perform an interruptible sleep - it will return
// early if the Service is halted.
func Sleep(ctx context.Context, d time.Duration) (halted bool) {
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
