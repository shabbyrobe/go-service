package servicemgr

import (
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

const (
	// HACK FEST: This needs to be high enough so that tests that rely on
	// timing don't fail because your computer was too slow
	tscale = 5 * time.Millisecond

	dto = 100 * tscale
)

func TestStart(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	// See notes in go-service/runner_test.go about startDelay
	s1 := &dummyService{runTime: 100 * tscale, startDelay: 2 * tscale}

	rdy := service.NewReadySignal()
	tt.MustOK(StartListen(nil, s1, rdy))
	tt.MustAssert(State(s1) == service.Starting)

	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustAssert(State(s1) == service.Started)

	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)
}

func TestStartWait(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	tt.MustOK(StartWaitListen(10*tscale, nil, s1))
	tt.MustAssert(State(s1) == service.Started)

	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)
}

func TestRegisterUnregister(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingFullListener(0)
	tt.MustOK(Register(s1).StartWaitListen(10*tscale, l))
	stats := listenerStats()
	tt.MustEqual(1, stats.listeners)
	tt.MustEqual(1, stats.listenersNHError)
	tt.MustEqual(1, stats.listenersState)
	tt.MustEqual(1, stats.retained)

	tt.MustEqual(1, len(Services(service.FindRegistered, 0)))

	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)

	tt.MustEqual(1, len(Services(service.FindRegistered, 0)))
	tt.MustOK(Unregister(s1))
	tt.MustEqual(0, len(Services(service.FindRegistered, 0)))
	tt.MustEqual(0, len(Services(service.FindUnregistered, 0)))

	stats = listenerStats()
	tt.MustEqual(0, stats.listeners)
	tt.MustEqual(0, stats.listenersNHError)
	tt.MustEqual(0, stats.listenersState)
	tt.MustEqual(0, stats.retained)
}

func TestListenerEndsOnHalt(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingListener(1)
	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, listenerStats().listeners)
	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)

	lerr := <-l.ends
	tt.MustOK(lerr.err)
	tt.MustEqual(s1, lerr.service)
}

func TestListenerShared(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}
	s2 := &dummyService{runTime: 100 * tscale}

	l := newTestingFullListener(2)
	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustOK(StartWaitListen(10*tscale, l, s2))
	tt.MustEqual(2, listenerStats().listeners)

	tt.MustOK(Shutdown(dto))
	tt.MustAssert(State(s1) == service.Halted)
	tt.MustAssert(State(s2) == service.Halted)

	lerr1 := <-l.ends
	tt.MustOK(lerr1.err)
	tt.MustEqual(s1, lerr1.service)

	lerr2 := <-l.ends
	tt.MustOK(lerr2.err)
	tt.MustEqual(s1, lerr2.service)
}

func TestListenerServices(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}
	s2 := &dummyService{runTime: 100 * tscale}

	tt.MustOK(StartWait(10*tscale, s1))
	tt.MustOK(StartWait(10*tscale, s2))
	tt.MustEqual(0, listenerStats().listeners)

	tt.MustEqual([]service.Service{s1, s2}, Services(service.AnyState, 0))

	tt.MustOK(Shutdown(dto))
	tt.MustAssert(State(s1) == service.Halted)
	tt.MustAssert(State(s2) == service.Halted)

	tt.MustEqual([]service.Service{}, Services(service.AnyState, 0))
}

func TestListenerUnregisteredServiceDoesNotLeakMemory(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingFullListener(1)
	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, listenerStats().listeners)

	tt.MustEqual([]service.Service{s1}, Services(service.AnyState, 0))

	tt.MustOK(Shutdown(dto))
	tt.MustAssert(State(s1) == service.Halted)
	tt.MustEqual(0, listenerStats().count)
	tt.MustEqual([]service.Service{}, Services(service.AnyState, 0))
}

func TestListenerUnregisterRunningService(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingFullListener(1)

	tt.MustEqual(0, listenerStats().retained)
	tt.MustOK(Register(s1))
	tt.MustEqual(1, listenerStats().retained)

	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, listenerStats().listeners)

	tt.MustEqual([]service.Service{s1}, Services(service.AnyState, 0))
	deferred, err := Unregister(s1)
	tt.MustOK(err)
	tt.MustAssert(deferred)

	tt.MustOK(Shutdown(dto))
	tt.MustAssert(State(s1) == service.Halted)

	// FIXME: this fails periodically because the listener is called in a
	// goroutine. we need the endWaiter from the listenerCollector in the
	// service package in here.
	tt.MustEqual(0, listenerStats().count)
	tt.MustEqual([]service.Service{}, Services(service.AnyState, 0))
}

func TestListenerUnregisterHaltedService(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingFullListener(1)

	tt.MustEqual(0, listenerStats().retained)
	tt.MustOK(Register(s1))
	tt.MustEqual(1, listenerStats().retained)

	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, listenerStats().retained)
	tt.MustEqual(1, listenerStats().count)

	tt.MustEqual([]service.Service{s1}, Services(service.AnyState, 0))
	tt.MustOK(Halt(dto, s1))

	stats := listenerStats()
	tt.MustEqual(1, stats.retained)
	tt.MustEqual(1, stats.count)

	deferred, err := Unregister(s1)
	tt.MustOK(err)
	tt.MustAssert(!deferred)
	tt.MustAssert(State(s1) == service.Halted)
	tt.MustEqual([]service.Service{}, Services(service.AnyState, 0))

	stats = listenerStats()
	tt.MustEqual(0, stats.retained)
	tt.MustEqual(0, stats.count)
}

func listenerStats() (stats struct {
	count            int
	listeners        int
	listenersNHError int
	listenersState   int
	retained         int
}) {
	l := getListener()
	l.lock.Lock()
	stats.listeners = len(l.listeners)
	stats.listenersNHError = len(l.listenersNHError)
	stats.listenersState = len(l.listenersState)
	stats.retained = len(l.retained)

	if stats.listeners > stats.count {
		stats.count = stats.listeners
	}
	if stats.listenersNHError > stats.count {
		stats.count = stats.listenersNHError
	}
	if stats.listenersState > stats.count {
		stats.count = stats.listenersState
	}
	if stats.retained > stats.count {
		stats.count = stats.retained
	}
	l.lock.Unlock()
	return
}
