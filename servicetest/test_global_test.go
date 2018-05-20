package servicetest

import (
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/servicemgr"
	"github.com/shabbyrobe/golib/assert"
)

func TestGlobalStart(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	// See notes in test_runner_test.go about startDelay
	s1 := (&TimedService{RunTime: 100 * tscale, StartDelay: 2 * tscale}).Init()

	rdy := service.NewReadySignal()
	tt.MustOK(servicemgr.StartListen(nil, s1, rdy))
	tt.MustAssert(servicemgr.State(s1) == service.Starting)

	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustAssert(servicemgr.State(s1) == service.Started)

	tt.MustOK(servicemgr.Halt(dto, s1))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
}

func TestGlobalStartWait(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()

	tt.MustOK(servicemgr.StartWaitListen(10*tscale, nil, s1))
	tt.MustAssert(servicemgr.State(s1) == service.Started)

	tt.MustOK(servicemgr.Halt(dto, s1))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
}

func TestGlobalRegisterUnregister(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()

	l := NewNullListenerFull()
	tt.MustOK(servicemgr.Register(s1).StartWaitListen(10*tscale, l))
	stats := servicemgr.Stats()
	tt.MustEqual(1, stats.Listeners)
	tt.MustEqual(1, stats.ListenersError)
	tt.MustEqual(1, stats.ListenersState)
	tt.MustEqual(1, stats.Retained)

	tt.MustEqual(1, len(servicemgr.Services(service.FindRegistered, 0)))

	tt.MustOK(servicemgr.Halt(dto, s1))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)

	tt.MustEqual(1, len(servicemgr.Services(service.FindRegistered, 0)))
	tt.MustOK(servicemgr.Unregister(s1))
	tt.MustEqual(0, len(servicemgr.Services(service.FindRegistered, 0)))
	tt.MustEqual(0, len(servicemgr.Services(service.FindUnregistered, 0)))

	stats = servicemgr.Stats()
	tt.MustEqual(0, stats.Listeners)
	tt.MustEqual(0, stats.ListenersError)
	tt.MustEqual(0, stats.ListenersState)
	tt.MustEqual(0, stats.Retained)
}

func TestGlobalListenerEndsOnHalt(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()

	l := NewListenerCollector()
	w := l.EndWaiter(s1, 1)
	tt.MustOK(servicemgr.StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, servicemgr.Stats().Listeners)

	tt.MustOK(servicemgr.Halt(dto, s1))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
	tt.MustOK(w.Take(1 * time.Second))
}

func TestGlobalListenerShared(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()
	s2 := (&TimedService{RunTime: 100 * tscale}).Init()

	l := NewListenerCollector()
	w1 := l.EndWaiter(s1, 1)
	w2 := l.EndWaiter(s2, 1)

	tt.MustOK(servicemgr.StartWaitListen(10*tscale, l, s1))
	tt.MustOK(servicemgr.StartWaitListen(10*tscale, l, s2))
	tt.MustEqual(2, servicemgr.Stats().Listeners)

	tt.MustOK(servicemgr.Shutdown(dto))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
	tt.MustAssert(servicemgr.State(s2) == service.Halted)

	tt.MustOK(w1.Take(1 * time.Second))
	tt.MustOK(w2.Take(1 * time.Second))
}

func TestGlobalListenerServices(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()
	s2 := (&TimedService{RunTime: 100 * tscale}).Init()

	tt.MustOK(servicemgr.StartWait(10*tscale, s1))
	tt.MustOK(servicemgr.StartWait(10*tscale, s2))
	tt.MustEqual(0, servicemgr.Stats().Listeners)

	tt.MustEqual([]service.Service{s1, s2}, servicemgr.Services(service.AnyState, 0))

	tt.MustOK(servicemgr.Shutdown(dto))
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
	tt.MustAssert(servicemgr.State(s2) == service.Halted)

	tt.MustEqual([]service.Service{}, servicemgr.Services(service.AnyState, 0))
}

func TestGlobalListenerUnregisteredServiceDoesNotLeakMemory(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()
	lc := NewListenerCollector()
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(servicemgr.StartWaitListen(10*tscale, lc, s1))
	tt.MustEqual(1, servicemgr.Stats().Listeners)

	tt.MustEqual([]service.Service{s1}, servicemgr.Services(service.AnyState, 0))

	tt.MustOK(servicemgr.Shutdown(dto))
	ew.Take(1 * time.Second)

	tt.MustAssert(servicemgr.State(s1) == service.Halted)
	tt.MustEqual(0, servicemgr.Stats().Listeners)
	tt.MustEqual([]service.Service{}, servicemgr.Services(service.AnyState, 0))
}

func TestGlobalListenerUnregisterRunningService(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()
	lc := NewListenerCollector()
	ew := lc.EndWaiter(s1, 1)

	tt.MustEqual(0, servicemgr.Stats().Retained)
	tt.MustOK(servicemgr.Register(s1))
	tt.MustEqual(1, servicemgr.Stats().Retained)

	tt.MustOK(servicemgr.StartWaitListen(10*tscale, lc, s1))
	tt.MustEqual(1, servicemgr.Stats().Listeners)

	tt.MustEqual([]service.Service{s1}, servicemgr.Services(service.AnyState, 0))
	deferred, err := servicemgr.Unregister(s1)
	tt.MustOK(err)
	tt.MustAssert(deferred)

	tt.MustOK(servicemgr.Shutdown(dto))
	tt.MustOK(ew.Take(1 * time.Second))

	tt.MustAssert(servicemgr.State(s1) == service.Halted)

	tt.MustEqual(0, servicemgr.Stats().Listeners)
	tt.MustEqual([]service.Service{}, servicemgr.Services(service.AnyState, 0))
}

func TestGlobalListenerUnregisterHaltedService(t *testing.T) {
	defer servicemgr.Reset()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 100 * tscale}).Init()

	l := NewNullListenerFull()

	tt.MustEqual(0, servicemgr.Stats().Retained)
	tt.MustOK(servicemgr.Register(s1))
	tt.MustEqual(1, servicemgr.Stats().Retained)

	tt.MustOK(servicemgr.StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, servicemgr.Stats().Retained)
	tt.MustEqual(1, servicemgr.Stats().Listeners)

	tt.MustEqual([]service.Service{s1}, servicemgr.Services(service.AnyState, 0))
	tt.MustOK(servicemgr.Halt(dto, s1))

	stats := servicemgr.Stats()
	tt.MustEqual(1, stats.Retained)
	tt.MustEqual(1, stats.Listeners)

	deferred, err := servicemgr.Unregister(s1)
	tt.MustOK(err)
	tt.MustAssert(!deferred)
	tt.MustAssert(servicemgr.State(s1) == service.Halted)
	tt.MustEqual([]service.Service{}, servicemgr.Services(service.AnyState, 0))

	stats = servicemgr.Stats()
	tt.MustEqual(0, stats.Retained)
	tt.MustEqual(0, stats.Listeners)
}
