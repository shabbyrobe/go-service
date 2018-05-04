package servicemgr

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"sync/atomic"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
)

func TestMain(m *testing.M) {
	beforeCount := pprof.Lookup("goroutine").Count()
	code := m.Run()

	if code == 0 {
		// See notes in service.TestMain
		time.Sleep(20 * time.Millisecond)

		after := pprof.Lookup("goroutine")
		afterCount := after.Count()

		diff := afterCount - beforeCount
		if diff > 0 {
			var buf bytes.Buffer
			after.WriteTo(&buf, 1)
			fmt.Fprintf(os.Stderr, "stray goroutines: %d\n%s\n", diff, buf.String())
			os.Exit(2)
		}
	}

	os.Exit(code)
}

type dummyService struct {
	name         service.Name
	startFailure error
	startDelay   time.Duration
	runFailure   error
	runTime      time.Duration
	haltDelay    time.Duration
	starts       int32
	halts        int32
}

func (d *dummyService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *dummyService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *dummyService) ServiceName() service.Name {
	if d.name == "" {
		// This is a nasty cheat, don't do it in any real code!
		return service.Name(fmt.Sprintf("dummyService-%p", d))
	}
	return d.name
}

func (d *dummyService) Run(ctx service.Context) error {
	atomic.AddInt32(&d.starts, 1)

	if d.startDelay > 0 {
		time.Sleep(d.startDelay)
	}
	if d.startFailure != nil {
		return d.startFailure
	}
	if err := ctx.Ready(); err != nil {
		return err
	}

	defer atomic.AddInt32(&d.halts, 1)

	if d.runTime > 0 {
		service.Sleep(ctx, d.runTime)
	}
	if service.IsDone(ctx) {
		if d.haltDelay > 0 {
			time.Sleep(d.haltDelay)
		}
		return nil
	} else {
		if d.runFailure == nil {
			return service.ErrServiceEnded
		}
		return d.runFailure
	}
}

type listenerErr struct {
	stage   service.Stage
	service service.Service
	err     service.Error
}

type listenerState struct {
	service service.Service
	state   service.State
}

type testingFullListener struct {
	*testingListener
	*testingNonHaltingErrorListener
	*testingStateListener
}

func newTestingFullListener(cap int) *testingFullListener {
	return &testingFullListener{
		testingListener:                newTestingListener(cap),
		testingNonHaltingErrorListener: newTestingNonHaltingErrorListener(cap),
		testingStateListener:           newTestingStateListener(cap),
	}
}

var (
	_ service.Listener      = &testingFullListener{}
	_ service.ErrorListener = &testingFullListener{}
	_ service.StateListener = &testingFullListener{}

	_ service.Listener = &testingListener{}
)

type testingListener struct {
	ends chan listenerErr
}

func newTestingListener(cap int) *testingListener {
	return &testingListener{
		ends: make(chan listenerErr, cap),
	}
}

func (t *testingListener) OnServiceEnd(stage service.Stage, service service.Service, err service.Error) {
	select {
	case t.ends <- listenerErr{stage: stage, service: service, err: err}:
	default:
	}
}

type testingNonHaltingErrorListener struct {
	*testingListener
	errors chan listenerErr
}

func newTestingNonHaltingErrorListener(cap int) *testingNonHaltingErrorListener {
	return &testingNonHaltingErrorListener{
		testingListener: newTestingListener(cap),
		errors:          make(chan listenerErr, cap),
	}
}

var _ service.ErrorListener = &testingNonHaltingErrorListener{}

func (t *testingNonHaltingErrorListener) OnServiceError(service service.Service, err service.Error) {
	select {
	case t.ends <- listenerErr{service: service, err: err}:
	default:
	}
}

type testingStateListener struct {
	*testingListener
	states chan listenerState
}

func newTestingStateListener(cap int) *testingStateListener {
	return &testingStateListener{
		testingListener: newTestingListener(cap),
		states:          make(chan listenerState, cap),
	}
}

var _ service.StateListener = &testingStateListener{}

func (t *testingStateListener) OnServiceState(service service.Service, state service.State) {
	select {
	case t.states <- listenerState{service: service, state: state}:
	default:
	}
}
