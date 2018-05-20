package serviceutil

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/servicemgr"
	"go.uber.org/zap"
)

// TimedRestart is an experimental service.Wrapper that restarts a service that
// ends prematurely after a specific interval.
//
// It's intended to prototype a method for retrying failed connections or
// subprocesses.
//
// The big problem with this approach is the use of service.Standalone() -
// there's no way to catch lockups caused by services that don't halt properly.
//
type TimedRestart struct {
	service  service.Service
	limit    int
	timeout  time.Duration
	wait     time.Duration
	listener TimedRestartListener
	running  uint32
	starts   uint64

	suspended int32
	suspend   *sync.Cond
	ctx       service.RunContext
	lock      sync.Mutex
}

type TimedRestartListener interface {
	OnTimedRestartReady(start int)
	OnTimedRestartError(start int, err error)
}

// NewTimedRestart creates a TimedRestart service. If you want to log errors,
// pass in a listener, otherwise pass nil.
func NewTimedRestart(svc service.Service, timeout, wait time.Duration,
	listener TimedRestartListener, active bool) *TimedRestart {

	if svc == nil {
		panic("service was nil")
	}

	tr := &TimedRestart{
		service:  svc,
		timeout:  timeout,
		wait:     wait,
		listener: listener,
	}
	if !active {
		tr.suspended = 1
	}
	tr.suspend = sync.NewCond(&tr.lock)
	return tr
}

func (t *TimedRestart) Running() bool         { return atomic.LoadUint32(&t.running) == 1 }
func (t *TimedRestart) Starts() uint64        { return atomic.LoadUint64(&t.starts) }
func (t *TimedRestart) Suspended() (out bool) { return atomic.LoadInt32(&t.suspended) == 1 }

func (t *TimedRestart) ServiceName() service.Name { return t.service.ServiceName() }

// Suspend instructs the service to halt and to not restart.
// Developers note: this function should return quickly, not wait
// for Halt as there are things that depend on this that must not
// be blocked.
func (t *TimedRestart) Suspend(suspended bool) (changed bool) {
	t.lock.Lock()
	defer t.lock.Unlock()

	var sv int32
	if suspended {
		sv = 1
	}
	ov := atomic.SwapInt32(&t.suspended, sv)
	if ov == sv {
		return false
	}

	if suspended == true {
		if t.ctx != nil {
			// RunContext.Halt() does little more than close the done channel,
			// it doesn't wait until the service has actually halted.
			t.ctx.Halt()
			t.ctx = nil
		}
	} else {
		t.suspend.Broadcast()
	}

	return true
}

func (t *TimedRestart) suspendedWait() {
	t.lock.Lock()
	defer t.lock.Unlock()
	for atomic.LoadInt32(&t.suspended) == 1 {
		zap.L().Debug("timed restart suspended")
		t.suspend.Wait()
	}
	zap.L().Debug("timed restart active")
}

func (t *TimedRestart) Run(ctx service.Context) error {
	// This is an interesting one. If the service fails to start first go,
	// we can't exactly say we're "ready", but we can't hold everything
	// else up waiting for this thing's restart to finally succeed.
	//
	// At the moment, this assumes that if you want a restarting service,
	// you're content to be signalled that things are "ready to be started
	// until they start", not that things are "ready to receive and process
	// connections".
	//
	// It might be better to say "the first time we fail, or the first time
	// we receive a ready signal from the child service, we are ready", but
	// the above caveat still applies - you can't really guarantee readiness
	// without some other mechanism as the restarting mechanism makes actual
	// "readiness" something that can cease to be true. This does not apply to
	// normal services - readiness is a permanent state until halt or failure.
	//
	if err := ctx.Ready(); err != nil {
		return nil
	}

	ender := servicemgr.NewEndListener(1)

	contexts := make(chan timedRun, 1)

	go func() {
		for {
			run, ok := <-contexts
			if !ok {
				return
			}

			t.lock.Lock()
			t.ctx = run.rctx
			t.lock.Unlock()

			t.suspendedWait()

			atomic.StoreUint32(&t.running, 1)
			atomic.AddUint64(&t.starts, 1)
			err := t.service.Run(run.rctx)
			atomic.StoreUint32(&t.running, 0)
			ender.Send(err)

			// it's bad whether or not it's nil:
			if t.listener != nil {
				t.listener.OnTimedRestartError(run.start, errors.Wrap(err, "timed restart ended"))
			}
		}
	}()

	var rctx service.RunContext
	var failures = 0
	var start = 0
	defer func() {
		if rctx != nil {
			rctx.Halt()
		}
		close(contexts)
	}()

	for {
		start++
		curStart := start
		rctx = service.Standalone().WhenReady(func(svc service.Service) error {
			if t.listener != nil {
				go t.listener.OnTimedRestartReady(curStart)
			}
			return nil
		})

		contexts <- timedRun{start: curStart, rctx: rctx}

		select {
		case err := <-ender.Ends():
			failures++
			if t.limit > 0 && failures >= t.limit {
				t.listener.OnTimedRestartError(curStart, errors.Wrap(err, "timed restart failed"))
				return err
			}
			if halted := service.Sleep(ctx, t.wait); halted {
				// Watch out, I nearly missed a bug here where service.Sleep() didn't
				// properly check if the sleep was halted by ctx.Done() being tripped.
				// If I let that one slip through, this loop would've gone around for
				// another restart if you tried to halt while it was sleeping!
				return nil
			}

		case <-ctx.Done():
			return nil
		}
	}
}

type timedRun struct {
	start int
	rctx  service.RunContext
}
