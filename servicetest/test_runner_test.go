package servicetest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

// TODO:
// - Test every state transition using a state listener to control advancement
// - Verify nil contexts work for Start(), Halt() and Shutdown()
// - Multiple halts, multiple starts

const haltableStates = service.Starting | service.Started | service.Halting

func mustStateError(tt assert.T, err error, expected service.State, current service.State) {
	tt.Helper()
	tt.MustAssert(err != nil, "state error not found")

	expstr := fmt.Sprintf("expected %q; found %q", expected, current)
	tt.MustAssert(
		strings.Contains(err.Error(), expstr),
		fmt.Sprintf("assertion failed: expected %q, found %q", expstr, err.Error()))
}

func mustRecv(tt assert.T, c <-chan error, timeout time.Duration) error {
	tt.Helper()
	after := time.After(timeout)
	select {
	case err := <-c:
		return err
	case <-after:
		tt.Fatalf("waiter did not yield within timeout %v", timeout)
		return nil
	}
}

func mustNotRecv(tt assert.T, waiter chan struct{}) {
	close(waiter)

	tt.Helper()
	select {
	case _, ok := <-waiter:
		if ok {
			tt.Fatalf("waiter should not yield")
		}
	default:
	}
}

func TestRunnerStart(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner()

	for i := 0; i < 5; i++ {
		svc := service.New("", s1)
		tt.MustOK(service.StartTimeout(dto, r, svc))
		tt.MustAssert(r.State(svc) == service.Started)

		err := service.StartTimeout(dto, r, svc)
		tt.MustAssert(service.IsAlreadyRunning(err), err)

		tt.MustOK(service.HaltTimeout(dto, r, svc))
		tt.MustAssert(r.State(svc) == service.Halted)

		// Halting a second time should yield the same result:
		tt.MustOK(service.HaltTimeout(dto, r, svc))
		tt.MustAssert(r.State(svc) == service.Halted)
	}
}

func TestRunnerStates(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&BlockingService{}).Init())
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	sw1 := lc.StateWaiter(s1, 2)
	tt.MustOK(service.StartTimeout(dto, r, s1))
	tt.MustEqual(service.StateChange{s1, service.Halted, service.Starting}, *sw1.Take(dto))
	tt.MustEqual(service.StateChange{s1, service.Starting, service.Started}, *sw1.Take(dto))

	tt.MustOK(service.HaltTimeout(dto, r, s1))
	tt.MustEqual(service.StateChange{s1, service.Started, service.Halting}, *sw1.Take(dto))
	tt.MustEqual(service.StateChange{s1, service.Halting, service.Ended}, *sw1.Take(dto))
}

func TestRunnerHalt(t *testing.T) {
	tt := assert.WrapTB(t)

	sr1 := (&BlockingService{}).Init()
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	s1 := service.New("", sr1)

	err := service.StartTimeout(dto, r, s1)
	tt.MustOK(err)
	tt.MustAssert(r.State(s1) == service.Started)

	// an arbitrary number of calls to HaltTimeout should be OK:
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	tt.MustOK(service.HaltTimeout(dto, r, s1))

	// runTime must be long enough to ensure that EnsureHalt times out
	sr2 := (&UnhaltableService{}).Init()
	s2 := service.New("", sr2)
	e2 := lc.EndWaiter(s2, 1)
	tt.MustOK(service.StartTimeout(dto, r, s2))
	tt.MustEqual(context.DeadlineExceeded, service.HaltTimeout(1*time.Nanosecond, r, s2))
	close(sr2.halt)
	mustRecv(tt, e2.C(), dto)
}

func TestRunnerSameServiceMultipleRunners(t *testing.T) {
	tt := assert.WrapTB(t)

	sr1 := (&BlockingService{StartDelay: 2 * tscale}).Init()
	s1 := service.New("", sr1)
	r1 := service.NewRunner()
	r2 := service.NewRunner()

	tt.MustOK(r1.Start(nil, s1))
	tt.MustOK(r2.Start(nil, s1))
	tt.MustAssert(r1.State(s1) == service.Started)
	tt.MustAssert(r2.State(s1) == service.Started)
	tt.MustOK(service.HaltTimeout(dto, r1, s1))
	tt.MustOK(service.HaltTimeout(dto, r2, s1))
	tt.MustAssert(r1.State(s1) == service.Halted)
	tt.MustAssert(r2.State(s1) == service.Halted)
}

func TestRunnerStartMultiple(t *testing.T) {
	tt := assert.WrapTB(t)

	sr1 := (&BlockingService{}).Init()
	sr2 := (&BlockingService{}).Init()
	s1, s2 := service.New("", sr1), service.New("", sr2)
	r := service.NewRunner()

	tt.MustOK(r.Start(nil, s1, s2))
	tt.MustOK(service.HaltTimeout(dto, r, s1, s2))
	tt.MustAssert(r.State(s1) == service.Halted)
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerHaltMultiple(t *testing.T) {
	tt := assert.WrapTB(t)

	rn := service.RunnableFunc(func(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		time.Sleep(tscale * 10)
		return nil
	})

	s1, s2 := service.New("", rn), service.New("", rn)
	r := service.NewRunner()

	tt.MustOK(r.Start(nil, s1, s2))
	tt.MustOK(service.HaltTimeout(dto, r, s1, s2))
	tt.MustAssert(r.State(s1) == service.Halted)
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartServiceEnds(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&TimedService{}).Init()) // service should end immediately after it is started.

	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(nil, s1, nil))
	mustRecv(tt, ew.C(), dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))
}

func TestRunnerServiceEndedCausesContextToBeDone(t *testing.T) {
	tt := assert.WrapTB(t)

	// Don't try to receive from ctx.Done() until after the service has ended
	// in the runner:
	ended := make(chan struct{})

	doneYielded := make(chan error)

	s1 := service.New("", service.RunnableFunc(func(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		go func() {
			<-ended
			<-ctx.Done()
			close(doneYielded)
		}()
		return service.ErrServiceEnded
	}))

	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(nil, s1, nil))
	mustRecv(tt, ew.C(), dto)
	close(ended)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))

	mustRecv(tt, doneYielded, dto)
}

func TestRunnerStartErrorBeforeReady(t *testing.T) {
	tt := assert.WrapTB(t)

	serr := errors.New("1")
	s1 := service.New("", (&TimedService{StartFailure: serr}).Init())
	lc := NewListenerCollector()
	ew := lc.EndWaiter(s1, 1)

	rn := service.NewRunner(lc.RunnerOptions()...)

	err := service.StartTimeout(dto, rn, s1)
	tt.MustEqual(serr, cause(err))

	// We must receive an 'end' via the listener.
	mustRecv(tt, ew.C(), dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageReady, Err: serr}}, lc.Ends(s1))
}

func TestRunnerServiceEndsAfterReady(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("",
		(&TimedService{RunTime: 10 * time.Millisecond}).Init(),
	)
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(service.StartTimeout(dto, r, s1))
	mustRecv(tt, ew.C(), dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))
}

func TestRunnerServiceEndsBeforeReady(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("e1")
	s1 := service.New("", (&TimedService{StartFailure: e1}).Init()) // service ends as soon as it has started
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	tt.MustEqual(e1, cause(service.StartTimeout(dto, r, s1)))
}

func TestRunnerStartServiceThenStartSecondAfterFirstEnds(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&TimedService{}).Init()) // service ends as soon as it has started
	s2 := service.New("", (&BlockingService{}).Init())

	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	ew1 := lc.EndWaiter(s1, 1)
	ew2 := lc.EndWaiter(s2, 1)

	tt.MustOK(r.Start(nil, s1))
	mustRecv(tt, ew1.C(), dto) // unblocks when the first service ends

	tt.MustOK(r.Start(nil, s2))
	tt.MustOK(service.HaltTimeout(dto, r, s2))
	mustRecv(tt, ew2.C(), dto)
}

func TestRunnerReadyTimeout(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&BlockingService{StartDelay: 5 * tscale}).Init())
	r := service.NewRunner()

	err := service.StartTimeout(1*tscale, r, s1)
	tt.MustEqual(context.DeadlineExceeded, err)

	herr := service.HaltTimeout(dto, r, s1)
	tt.MustOK(herr)
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartHaltWhileInStartDelay(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&BlockingService{StartDelay: 2 * tscale}).Init())
	lc := NewListenerCollector()
	sw1 := lc.StateWaiter(s1, 2)
	ew1 := lc.EndWaiter(s1, 1)
	r := service.NewRunner(lc.RunnerOptions()...)

	go func() {
		// StartTimeout blocks until Started so we have to do it outside the
		// main test to check the Starting state.
		tt.MustOK(service.StartTimeout(dto, r, s1))
	}()

	tt.MustEqual(service.StateChange{s1, service.Halted, service.Starting}, *sw1.Take(dto))
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	mustRecv(tt, ew1.C(), dto)

	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartShutdownImmediately(t *testing.T) {
	tt := assert.WrapTB(t)

	r := service.NewRunner()
	s1 := service.New("", (&BlockingService{}).Init())

	tt.MustOK(r.Start(nil, s1))
	tt.MustAssert(r.State(s1) == service.Started)

	tt.MustOK(service.ShutdownTimeout(dto, r))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartDelay(t *testing.T) {
	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := service.New("", (&BlockingService{StartDelay: delay}).Init())
	r := service.NewRunner()

	tm := time.Now()
	tt.MustOK(service.StartTimeout(dto, r, s1))
	defer service.MustHaltTimeout(dto, r, s1)

	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltDelay(t *testing.T) {
	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := service.New("", (&BlockingService{HaltDelay: delay}).Init())
	r := service.NewRunner()

	tt.MustOK(service.StartTimeout(dto, r, s1))

	tm := time.Now()
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltingState(t *testing.T) {
	// This test attempts to catch a service in the act of 'halting' to ensure
	// that the halting state is correctly reported.
	//
	// It does so by starting a service that takes a certain amount of time to
	// halt, starting it, halting it, then polling its state.

	tt := assert.WrapTB(t)

	delay := 20 * tscale
	s1 := service.New("", (&BlockingService{HaltDelay: delay}).Init())
	r := service.NewRunner()

	tt.MustOK(service.StartTimeout(dto, r, s1))

	var stop int32
	var out = make(chan map[service.State]int)

	tm := time.Now()

	go func() {
		counts := map[service.State]int{}
		for {
			if atomic.LoadInt32(&stop) == 1 {
				out <- counts
				return
			}
			state := r.State(s1)
			counts[state]++
			time.Sleep(tscale)
		}
	}()
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	tt.MustAssert(time.Since(tm) > delay)
	tt.MustAssert(r.State(s1) == service.Halted)
	atomic.StoreInt32(&stop, 1)
	counts := <-out
	tt.MustAssert(counts[service.Halting] > 0)
}

func TestHaltableSleep(t *testing.T) {
	tt := assert.WrapTB(t)

	// We have to use MinHaltableSleep instead of tscale here
	// to make sure we use the channel-based version of the sleep
	// function
	sr1 := (&TimedService{RunTime: service.MinHaltableSleep}).Init()
	s1 := service.New("", sr1)
	r := service.NewRunner()

	tt.MustOK(service.StartTimeout(dto, r, s1))
	tm := time.Now()
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	since := time.Since(tm)
	limit := time.Duration(float64(service.MinHaltableSleep) * 0.9)
	tt.MustAssert(since < limit, "sleep time %s was not < %s", since, limit)

	sr1.UnhaltableSleep = true
	tt.MustOK(service.StartTimeout(dto, r, s1))
	tm = time.Now()
	tt.MustOK(service.HaltTimeout(dto, r, s1))
	since = time.Since(tm)

	// only test for 95% of the delay because the timers aren't perfect. sometimes
	// (though very, very rarely) we see test failures like this: "sleep time
	// 49.957051ms, expected 50ms"
	lim := time.Duration(float64(service.MinHaltableSleep) * 0.95)
	tt.MustAssert(since >= lim, "sleep time %s, expected %s", since, service.MinHaltableSleep)
}

func TestRunnerOnError(t *testing.T) {
	// FIXME: this test is brittle and terrible. it has been hacked on until
	// it passes but it should be rewritten.
	tt := assert.WrapTB(t)

	var s1StartTime, s2StartTime time.Duration = 4 * tscale, 6 * tscale
	var s1Tick, s2Tick time.Duration = 2 * tscale, 3 * tscale
	s1Expected, s2Expected := 4, 3
	var s1RunTime = s1StartTime + (s1Tick * time.Duration(s1Expected))
	var s2RunTime = s2StartTime + (s2Tick * time.Duration(s2Expected))
	var runTime = s1RunTime
	if s2RunTime > runTime {
		runTime = s2RunTime
	}

	sr1 := (&ErrorService{StartDelay: s1StartTime}).Init()
	sr2 := (&ErrorService{StartDelay: s2StartTime}).Init()
	s1 := service.New("", sr1)
	s2 := service.New("", sr2)

	go func() {
		t1 := time.NewTicker(s1Tick)
		t2 := time.NewTicker(s2Tick)
		s1Cnt, s2Cnt := 0, 0
		for s1Cnt < s1Expected || s2Cnt < s2Expected {
			select {
			case t := <-t1.C:
				if s1Cnt < s1Expected {
					sr1.errc <- fmt.Errorf("s1: %v", t)
					s1Cnt++
				}
			case t := <-t2.C:
				if s2Cnt < s2Expected {
					sr2.errc <- fmt.Errorf("s2: %v", t)
					s2Cnt++
				}
			}
		}
	}()

	lc := NewListenerCollector()
	ew1, ew2 := lc.ErrWaiter(s1, s1Expected), lc.ErrWaiter(s2, s2Expected)
	r := service.NewRunner(lc.RunnerOptions()...)
	tt.MustOK(service.StartTimeout(100*tscale, r, s1))
	tt.MustOK(service.StartTimeout(100*tscale, r, s2))

	s1Errs := ew1.TakeN(s1Expected, 10*runTime)
	s2Errs := ew2.TakeN(s2Expected, 10*runTime)
	tt.MustOK(service.ShutdownTimeout(dto, r))

	tt.MustAssert(len(s1Errs) >= s1Expected)
	tt.MustAssert(len(s2Errs) >= s2Expected)
}

func TestRunnerShutdown(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&BlockingService{}).Init())
	s2 := service.New("", (&BlockingService{}).Init())
	r := service.NewRunner()

	tt.MustOK(service.StartTimeout(dto, r, s1, s2))
	tt.MustEqual(2, len(r.Services(service.Started, 0, nil)))

	tt.MustOK(service.ShutdownTimeout(dto, r))
}

func TestRunnerServices(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("1", (&BlockingService{}).Init())
	s2 := service.New("2", (&BlockingService{}).Init())
	s3 := service.New("3", (&BlockingService{}).Init())
	r := service.NewRunner()

	tt.MustEqual(0, len(r.Services(service.AnyState, 0, nil)))

	// The runner does not retain halted services, so this will always return nothing.
	tt.MustEqual(0, len(r.Services(service.Halted, 0, nil)))

	tt.MustOK(service.StartTimeout(dto, r, s2))
	tt.MustOK(service.StartTimeout(dto, r, s3))

	assertServices := func(exp []service.ServiceInfo, query service.State, lim int) {
		tt.Helper()
		if len(exp) == 0 {
			tt.MustEqual(0, len(r.Services(query, lim, nil)))
		} else {
			svcs := r.Services(service.Started, lim, nil)
			sort.Slice(svcs, func(i, j int) bool { return svcs[i].Service.Name < svcs[j].Service.Name })
			tt.MustEqual(exp, svcs)
		}
	}

	{
		assertServices(nil, service.Halted, 0)
		exp := []service.ServiceInfo{
			{State: service.Started, Service: s2},
			{State: service.Started, Service: s3},
		}
		assertServices(exp, service.Started, 0)
	}

	{
		exp := []service.ServiceInfo{
			{State: service.Started, Service: s1},
			{State: service.Started, Service: s2},
			{State: service.Started, Service: s3},
		}
		tt.MustOK(service.StartTimeout(dto, r, s1))
		assertServices(exp, service.AnyState, 0)
		assertServices(exp, service.Started, 0)
		assertServices(nil, service.Halted, 0)
	}

	{
		tt.MustOK(service.HaltTimeout(dto, r, s1, s2, s3))
		assertServices(nil, service.AnyState, 0)
		assertServices(nil, service.Halted, 0)
		assertServices(nil, service.Started, 0)
	}
}

func TestRunnerServicesReusableMemory(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("1", (&BlockingService{}).Init())
	s2 := service.New("2", (&BlockingService{}).Init())
	r := service.NewRunner()
	defer service.MustShutdownTimeout(1*time.Second, r)

	tt.MustOK(service.StartTimeout(dto, r, s1))
	tt.MustOK(service.StartTimeout(dto, r, s2))

	into := make([]service.ServiceInfo, 0)
	out := r.Services(service.AnyState, 0, into)
	tt.MustEqual(2, cap(out))

	next := r.Services(service.AnyState, 0, out)
	tt.MustEqual(2, cap(next))
}

func TestRunnerServiceFunc(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", service.RunnableFunc(func(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}))

	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	sw1 := lc.StateWaiter(s1, 2)
	tt.MustOK(r.Start(nil, s1))
	tt.MustEqual(service.StateChange{s1, service.Halted, service.Starting}, *sw1.Take(dto))
	tt.MustEqual(service.StateChange{s1, service.Starting, service.Started}, *sw1.Take(dto))

	tt.MustOK(service.HaltTimeout(dto, r, s1))
}

func TestRunnerServiceWithHaltingGoroutines(t *testing.T) {
	tt := assert.WrapTB(t)

	var c uint32
	yep := make(chan struct{})

	ready := make(chan struct{})
	s1 := service.New("test", service.RunnableFunc(func(ctx service.Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			atomic.AddUint32(&c, 1)
			close(yep)
		}()
		<-ctx.Done()
		return nil
	}))

	lc := NewListenerCollector()
	sw := lc.StateWaiter(s1, 2)
	r := service.NewRunner(lc.RunnerOptions()...)
	go func() {
		if err := r.Start(nil, s1); err != nil {
			panic(err)
		}
	}()

	tt.MustEqual(service.StateChange{s1, service.Halted, service.Starting}, *sw.Take(dto))
	tt.MustEqual(service.Starting, r.State(s1))
	close(ready)

	tt.MustEqual(service.StateChange{s1, service.Starting, service.Started}, *sw.Take(dto))
	tt.MustEqual(service.Started, r.State(s1))

	tt.MustOK(service.HaltTimeout(dto, r, s1))

	select {
	case <-yep:
	case <-time.After(2 * time.Second):
		panic("timeout waiting for nested goroutine to stop")
	}
}

/*
func TestRunnerServiceWithHaltingGoroutinesOnEndError(t *testing.T) {
	tt := assert.WrapTB(t)

	var c uint32
	yep := make(chan struct{})

	ready := make(chan struct{})
	s1 := service.New("", service.RunnableFunc(func(ctx service.Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		go func() {
			// The service ending when the error is returned below should cause
			// this channel to be closed.
			<-ctx.Done()
			atomic.AddUint32(&c, 1)
			close(yep)
		}()

		// This should happen before <-ctx.Done() yields, but we should
		// not receive this error.
		return fmt.Errorf("nup")
	}))

	r := service.NewRunner()

	panic("need to respond to state listen events")

	tt.MustOK(r.Start(nil, s1))
	tt.MustEqual(service.Starting, r.State(s1))
	mustStateError(tt, r.Start(nil, s1), service.Halted, service.Starting)

	close(ready)

	// tt.MustOK(service.AwaitSignalTimeout(dto, rdy))

	select {
	case <-yep:
	case <-time.After(2 * time.Second):
		panic("timeout waiting for nested goroutine to stop")
	}
}
*/

func TestRunnerServiceHaltedNotRetained(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&BlockingService{}).Init())
	r := service.NewRunner()

	tt.MustOK(r.Start(nil, s1))
	service.MustHaltTimeout(dto*10, r, s1)

	services := r.Services(service.AnyState, 0, nil)
	tt.MustEqual(0, len(services))
}

func TestRunnerServiceEndedNotRetained(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := service.New("", (&TimedService{}).Init()) // should return immediately

	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(nil, s1))
	mustRecv(tt, ew.C(), dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))

	services := r.Services(service.AnyState, 0, nil)
	tt.MustEqual(0, len(services))
}

func TestRunnerHaltWhileStarting(t *testing.T) {
	// If we attempt to halt a service which is in the starting state, it should
	// halt the service and wait until the halt is complete.

	tt := assert.WrapTB(t)

	ready := make(chan struct{})

	s1 := service.New("test", service.RunnableFunc(func(ctx service.Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}))

	lc := NewListenerCollector()
	sw := lc.StateWaiter(s1, 2)
	r := service.NewRunner(lc.RunnerOptions()...)

	go func() {
		if err := r.Start(nil, s1); err != nil {
			panic(err)
		}
	}()

	tt.MustEqual(service.StateChange{s1, service.Halted, service.Starting}, *sw.Take(dto))
	tt.MustEqual(service.Starting, r.State(s1))

	hdone := make(chan error, 2)
	go func() { hdone <- r.Halt(nil, s1) }()
	go func() { hdone <- r.Halt(nil, s1) }()

	tt.MustEqual(service.StateChange{s1, service.Starting, service.Halting}, *sw.Take(dto))
	close(ready)

	tt.MustOK(<-hdone)
	s := time.Now()
	tt.MustOK(<-hdone)
	d := time.Since(s)
	tt.MustAssert(d < 1*time.Millisecond) // FIXME: Brittle timing test

	tt.MustEqual(0, len(r.Services(service.AnyState, 0, nil)))
}

func TestRunnerHaltWhileHalting(t *testing.T) {
	// If we attempt to halt a service which is in the halting state, it should
	// wait until the service is halted.

	tt := assert.WrapTB(t)

	haltDelay := 10 * tscale
	s1 := service.New("", (&BlockingService{HaltDelay: haltDelay}).Init())
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	tt.MustOK(service.StartTimeout(dto, r, s1))
	tt.MustAssert(r.State(s1) == service.Started)

	ts := make(chan time.Time, 2)
	go func() {
		if err := service.HaltTimeout(dto, r, s1); err != nil {
			panic(err)
		}
		ts <- time.Now()
	}()
	go func() {
		if err := service.HaltTimeout(dto, r, s1); err != nil {
			panic(err)
		}
		ts <- time.Now()
	}()

	t1 := <-ts
	t2 := <-ts
	diff := t2.Sub(t1)

	// Weak test... BlockingService sleeps for haltDelay and the two halts
	// should finish at roughly the same time, but the reliability of this
	// will be affected by tscale.
	tt.MustAssert(diff < haltDelay/10, "%s", diff)
}

func TestRunnerHaltOverlap(t *testing.T) {
	// The fuzzer unearthed some issues where services would find their way into
	// the "started" or "starting" states during the execution of Runner.Halt(),
	// and *between* the calls to Runner.Halting() and Runner.Halted().

	tt := assert.WrapTB(t)

	haltDelay := 10 * tscale
	s1 := service.New("", (&BlockingService{HaltDelay: haltDelay}).Init())
	lc := NewListenerCollector()
	r := service.NewRunner(lc.RunnerOptions()...)

	tt.MustOK(service.StartTimeout(dto, r, s1))
	tt.MustAssert(r.State(s1) == service.Started)

	ts := make(chan time.Time, 2)
	go func() {
		if err := service.HaltTimeout(dto, r, s1); err != nil {
			panic(err)
		}
		ts <- time.Now()
	}()
	go func() {
		if err := service.HaltTimeout(dto, r, s1); err != nil {
			panic(err)
		}
		ts <- time.Now()
	}()

	t1 := <-ts
	t2 := <-ts
	diff := t2.Sub(t1)

	// Weak test... BlockingService sleeps for haltDelay and the two halts
	// should finish at roughly the same time, but the reliability of this
	// will be affected by tscale.
	tt.MustAssert(diff < haltDelay/10)
}
