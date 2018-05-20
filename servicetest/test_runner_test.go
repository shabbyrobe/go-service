package servicetest

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

const haltableStates = service.Starting | service.Started | service.Halting

func mustStateError(tt assert.T, err error, expected service.State, current service.State) {
	tt.Helper()
	tt.MustAssert(err != nil, "state error not found")

	expstr := fmt.Sprintf("state error: expected %q; found %q", expected, current)
	tt.MustAssert(
		strings.Contains(err.Error(), expstr),
		fmt.Sprintf("assertion failed: expected %q, found %q", expstr, err.Error()))
}

func mustStateErrorUnknown(tt assert.T, err error) {
	tt.Helper()
	tt.MustAssert(err != nil, "state error not found")
	tt.MustAssert(service.IsErrServiceUnknown(err), err)
}

func mustRecv(tt assert.T, waiter Waiter, timeout time.Duration) {
	tt.Helper()
	after := time.After(timeout)
	select {
	case <-waiter.C():
	case <-after:
		tt.Fatalf("waiter did not yield within timeout %v", timeout)
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

func TestRunnerStartWait(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	for i := 0; i < 5; i++ {
		tt.MustOK(service.StartWait(r, dto, s1))
		tt.MustAssert(r.State(s1) == service.Started)

		mustStateError(tt, service.StartWait(r, dto, s1), service.Halted, service.Started)

		tt.MustOK(r.Halt(dto, s1))
		tt.MustAssert(r.State(s1) == service.Halted)

		herr := r.Halt(dto, s1)
		tt.MustAssert(service.IsErrServiceUnknown(herr), herr)
	}
}

func TestRunnerStart(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// FIXME: Must delay start enough to allow Starting state to be checked.
	// This is a brittle test - we could use OnServiceState to help block
	// the service until Starting has been checked.
	s1 := (&BlockingService{StartDelay: 2 * tscale}).Init()
	r := service.NewRunner(NewNullListenerFull())

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustAssert(r.State(s1) == service.Starting)
	mustStateError(tt, r.Start(s1, rdy), service.Halted, service.Starting)

	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustAssert(r.State(s1) == service.Started)

	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestEnsureHalt(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	lc := NewListenerCollector()
	r := service.NewRunner(lc)

	tt.MustOK(service.StartWait(r, dto, s1))
	tt.MustAssert(r.State(s1) == service.Started)

	// Unlike runner.Halt(), an arbitrary number of calls to EnsureHalted
	// should be OK
	tt.MustOK(service.EnsureHalt(r, dto, s1))
	tt.MustOK(service.EnsureHalt(r, dto, s1))
	tt.MustOK(service.EnsureHalt(r, dto, s1))

	herr := r.Halt(dto, s1)
	tt.MustAssert(service.IsErrServiceUnknown(herr), herr)

	// runTime must be long enough to ensure that EnsureHalt times out
	s2 := (&UnhaltableService{}).Init()
	e2 := lc.EndWaiter(s2, 1)
	tt.MustOK(service.StartWait(r, dto, s2))
	err := service.EnsureHalt(r, 1*time.Nanosecond, s2) // 1ns is the shortest possible timeout; 0 means wait forever
	tt.MustAssert(service.IsErrHaltTimeout(err), err)
	close(s2.halt)
	mustRecv(tt, e2, dto)
}

func TestRunnerSameServiceMultipleRunners(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{StartDelay: 2 * tscale}).Init()
	r1 := service.NewRunner(NewNullListenerFull())
	r2 := service.NewRunner(NewNullListenerFull())

	rdy1, rdy2 := service.NewReadySignal(), service.NewReadySignal()
	tt.MustOK(r1.Start(s1, rdy1))
	tt.MustOK(r2.Start(s1, rdy2))
	tt.MustAssert(r1.State(s1) == service.Starting)
	tt.MustAssert(r2.State(s1) == service.Starting)
	tt.MustOK(service.WhenReady(dto, rdy1))
	tt.MustOK(service.WhenReady(dto, rdy2))
	tt.MustAssert(r1.State(s1) == service.Started)
	tt.MustAssert(r2.State(s1) == service.Started)
	tt.MustOK(r1.Halt(dto, s1))
	tt.MustOK(r2.Halt(dto, s1))
	tt.MustAssert(r1.State(s1) == service.Halted)
	tt.MustAssert(r2.State(s1) == service.Halted)
}

func TestRunnerStartMultiple(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	mr := service.NewMultiReadySignal(2)
	tt.MustOK(r.Start(s1, mr))
	tt.MustOK(r.Start(s2, mr))
	tt.MustOK(service.WhenReady(dto, mr))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustOK(r.Halt(dto, s2))
}

func TestRunnerStartServiceEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{}).Init() // service should end immediately after it is started.

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(s1, nil))
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))
}

func TestRunnerStartWaitErrorBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("1")
	s1 := (&TimedService{StartFailure: serr}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustEqual(serr, cause(service.StartWait(r, dto, s1)))
}

func TestRunnerStartErrorBeforeReadyIsReturnedByWhenReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("fail")
	s1 := (&TimedService{StartFailure: serr}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	endc := lc.EndWaiter(s1, 1)

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	result := cause(service.WhenReady(dto, rdy))
	tt.MustAssert(result != nil)
	tt.MustEqual(serr, result)

	endc.Take(1 * time.Second)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageReady, Err: serr}}, lc.Ends(s1))
}

func TestRunnerStartFailureBeforeReadyPassedToSignal(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("start failure")
	s1 := (&TimedService{StartFailure: serr}).Init()
	lc := NewListenerCollector()
	rn := service.NewRunner(lc)
	ew := lc.EndWaiter(s1, 1)

	rdy := service.NewReadySignal()
	tt.MustOK(rn.Start(s1, rdy))
	tt.MustEqual(serr, cause(service.WhenReady(dto, rdy)))

	// The start error should be passed to the listener as well
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageReady, Err: serr}}, lc.Ends(s1))
}

func TestRunnerStartFailureBeforeReadyPassedToListenerWhenSignalIsNil(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("start failure")

	s1 := (&TimedService{StartFailure: serr}).Init()
	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(s1, nil))
	mustRecv(tt, ew, dto)
	tt.MustEqual(serr, lc.Ends(s1)[0].Err)
}

func TestRunnerStartWaitServiceEndsAfterReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 10 * time.Millisecond}).Init() // should return immediately
	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(service.StartWait(r, dto, s1))
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))
}

func TestRunnerStartWaitServiceEndsBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	e1 := errors.New("e1")
	s1 := (&TimedService{StartFailure: e1}).Init() // should return immediately
	lc := NewListenerCollector()
	r := service.NewRunner(lc)

	tt.MustEqual(e1, cause(service.StartWait(r, dto, s1)))
}

func TestRunnerStartFirstRegisteredThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{}).Init() // should return immediately
	s2 := (&BlockingService{}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	_ = r.Register(s1)

	ew1 := lc.EndWaiter(s1, 1)
	ew2 := lc.EndWaiter(s2, 1)

	mr := service.NewMultiReadySignal(2)
	tt.MustOK(r.Start(s1, mr))
	mustRecv(tt, ew1, dto)

	tt.MustOK(r.Start(s2, mr))
	tt.MustOK(service.WhenReady(1*time.Second, mr))
	mustStateError(tt, r.Halt(dto, s1), haltableStates, service.Halted)

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerStartFirstUnregisteredThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{}).Init() // should return immediately
	s2 := (&BlockingService{}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)

	ew1 := lc.EndWaiter(s1, 1)
	ew2 := lc.EndWaiter(s2, 1)

	mr := service.NewMultiReadySignal(2)
	tt.MustOK(r.Start(s1, mr))
	mustRecv(tt, ew1, dto)

	tt.MustOK(r.Start(s2, mr))
	tt.MustOK(service.WhenReady(1*time.Second, mr))
	mustStateErrorUnknown(tt, r.Halt(dto, s1))

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerStartWaitFirstRegisteredThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We need to make the dummy service run for a little while to make
	// sure that the End happens after the Ready
	s1 := (&TimedService{RunTime: 2 * tscale}).Init()

	s2 := (&BlockingService{}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	_ = r.Register(s1)

	ew1 := lc.EndWaiter(s1, 1)
	ew2 := lc.EndWaiter(s2, 1)

	tt.MustOK(service.StartWait(r, dto, s1))
	mustRecv(tt, ew1, dto)

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s2, rdy))
	tt.MustOK(service.WhenReady(dto, rdy))
	mustStateError(tt, r.Halt(dto, s1), haltableStates, service.Halted)

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerStartWaitFirstUnregisteredThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We need to make the dummy service run for a little while to make
	// sure that the End happens after the Ready
	s1 := (&TimedService{RunTime: 2 * tscale}).Init()

	s2 := (&BlockingService{}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)

	ew1 := lc.EndWaiter(s1, 1)
	ew2 := lc.EndWaiter(s2, 1)

	tt.MustOK(service.StartWait(r, dto, s1))
	mustRecv(tt, ew1, dto)

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s2, rdy))
	tt.MustOK(service.WhenReady(dto, rdy))
	mustStateErrorUnknown(tt, r.Halt(dto, s1))

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerReadyTimeout(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{StartDelay: 5 * tscale}).Init()
	r := service.NewRunner(NewNullListenerFull())

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustAssert(r.State(s1) == service.Starting)
	tt.MustAssert(service.IsErrWaitTimeout(service.WhenReady(1*tscale, rdy)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartHaltWhileInStartDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{StartDelay: 2 * tscale}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(r.Start(s1, nil))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartHaltImmediately(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := service.NewRunner(NewNullListenerFull())

	s1 := (&BlockingService{}).Init()
	tt.MustOK(r.Start(s1, nil))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartHaltImmediatelyWithReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := service.NewRunner(NewNullListenerFull())

	s1 := (&BlockingService{}).Init()
	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartShutdownImmediately(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := service.NewRunner(NewNullListenerFull())

	s1 := (&BlockingService{}).Init()
	tt.MustOK(r.Start(s1, nil))
	tt.MustOK(r.Shutdown(dto, 0))
	tt.MustAssert(r.State(s1) == service.Halted)
}

func TestRunnerStartDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := (&BlockingService{StartDelay: delay}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tm := time.Now()
	tt.MustOK(service.StartWait(r, dto, s1))
	defer tt.MustOK(r.Halt(dto, s1))

	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := (&BlockingService{HaltDelay: delay}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r, dto, s1))

	tm := time.Now()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) > delay)
}

/*
func TestRunnerHaltingState(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 20 * tscale
	s1 := (&BlockingService{HaltDelay: delay}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r, dto, s1))

	var stop int32
	var out = make(chan map[State]int)

	tm := time.Now()

	go func() {
		counts := map[State]int{}
		for {
			if atomic.LoadInt32(&stop) == 1 {
				out <- counts
				return
			}
			state := r.State(s1)
			if err := r.service.WhenReady(dto, s1); err != nil {
				panic(err)
			}
			counts[state]++
			time.Sleep(tscale)
		}
	}()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) > delay)
	tt.MustAssert(r.State(s1) == service.Halted)
	atomic.StoreInt32(&stop, 1)
	counts := <-out
	tt.MustAssert(counts[Halting] > 0)
}
*/

func TestRunnerRegisterMultiple(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r.Register(s1).Register(s1), dto, s1))
	tt.MustEqual(1, len(r.Services(service.FindRegistered, 0)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
	tt.MustEqual(service.Halted, r.State(s1))
	tt.MustEqual(1, len(r.Services(service.FindRegistered, 0)))

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual(0, len(r.Services(service.FindRegistered, 0)))
	_, err := r.Unregister(s1)
	tt.MustAssert(service.IsErrServiceUnknown(err))
}

func TestRunnerUnregister(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r.Register(s1), dto, s1))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == service.Halted)
	tt.MustEqual(service.Halted, r.State(s1))

	tt.MustOK(r.Unregister(s1))
	_, err := r.Unregister(s1)
	tt.MustAssert(service.IsErrServiceUnknown(err))
}

func TestRunnerUnregisterWhileNotHalted(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r.Register(s1), dto, s1))
	tt.MustEqual(0, len(r.Services(service.FindUnregistered, 0)))
	tt.MustEqual(1, len(r.Services(service.FindRegistered, 0)))
	tt.MustAssert(r.State(s1) == service.Started)

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual(0, len(r.Services(service.FindRegistered, 0)))
	tt.MustEqual(1, len(r.Services(service.FindUnregistered, 0)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustEqual(0, len(r.Services(service.AnyState, 0)))
}

func TestHaltableSleep(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We have to use MinHaltableSleep instead of tscale here
	// to make sure we use the channel-based version of the sleep
	// function
	s1 := (&TimedService{RunTime: service.MinHaltableSleep}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r, dto, s1))
	tm := time.Now()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) < time.Duration(float64(service.MinHaltableSleep)*0.9))

	s1.UnhaltableSleep = true
	tt.MustOK(service.StartWait(r, dto, s1))
	tm = time.Now()
	tt.MustOK(r.Halt(dto, s1))
	since := time.Since(tm)

	// only test for 95% of the delay because the timers aren't perfect. sometimes
	// (though very, very rarely) we see test failures like this: "sleep time
	// 49.957051ms, expected 50ms"
	lim := time.Duration(float64(service.MinHaltableSleep) * 0.95)
	tt.MustAssert(since >= lim, "sleep time %s, expected %s", since, service.MinHaltableSleep)
}

func TestRunnerOnError(t *testing.T) {
	// FIXME: this test is brittle and terrible. it has been hacked on until
	// it passes but it should be rewritten.
	t.Parallel()

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

	s1 := (&ErrorService{StartDelay: s1StartTime}).Init()
	s2 := (&ErrorService{StartDelay: s2StartTime}).Init()

	go func() {
		t1 := time.NewTicker(s1Tick)
		t2 := time.NewTicker(s2Tick)
		s1Cnt, s2Cnt := 0, 0
		for s1Cnt < s1Expected || s2Cnt < s2Expected {
			select {
			case t := <-t1.C:
				if s1Cnt < s1Expected {
					s1.errc <- fmt.Errorf("s1: %v", t)
					s1Cnt++
				}
			case t := <-t2.C:
				if s2Cnt < s2Expected {
					s2.errc <- fmt.Errorf("s2: %v", t)
					s2Cnt++
				}
			}
		}
	}()

	lc := NewListenerCollector()
	ew1, ew2 := lc.ErrWaiter(s1, s1Expected), lc.ErrWaiter(s2, s2Expected)
	r := service.NewRunner(lc)
	tt.MustOK(service.StartWait(r, 100*tscale, s1))
	tt.MustOK(service.StartWait(r, 100*tscale, s2))

	s1Errs := ew1.TakeN(s1Expected, 10*runTime)
	s2Errs := ew2.TakeN(s2Expected, 10*runTime)
	tt.MustOK(r.Shutdown(dto, 0))

	tt.MustAssert(len(s1Errs) >= s1Expected)
	tt.MustAssert(len(s2Errs) >= s2Expected)
}

func TestRunnerShutdown(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustOK(service.StartWait(r, dto, s1))
	tt.MustOK(service.StartWait(r, dto, s2))

	tt.MustOK(r.Shutdown(dto, 0))
}

func TestRunnerServices(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	s3 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())

	tt.MustEqual(0, len(r.Services(service.AnyState, 0)))

	r.Register(s1)
	tt.MustEqual([]service.Service{s1}, r.Services(service.FindHalted, 0))

	tt.MustOK(service.StartWait(r, dto, s2))
	tt.MustOK(service.StartWait(r, dto, s3))

	tt.MustEqual([]service.Service{s1}, r.Services(service.FindHalted, 0))
	tt.MustEqual([]service.Service{s2, s3}, r.Services(service.FindStarted, 0))

	tt.MustOK(service.StartWait(r, dto, s1))
	tt.MustEqual([]service.Service{s1, s2, s3}, r.Services(service.AnyState, 0))
	tt.MustEqual([]service.Service{s1, s2, s3}, r.Services(service.FindStarted, 0))

	tt.MustOK(r.Halt(dto, s1))
	tt.MustOK(r.Halt(dto, s2))
	tt.MustOK(r.Halt(dto, s3))

	// halted services are removed from the runner unless they are registered
	tt.MustEqual([]service.Service{s1}, r.Services(service.FindHalted, 0))

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual([]service.Service{}, r.Services(service.AnyState, 0))
}

func TestRunnerServiceFunc(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	ready := make(chan struct{})
	s1 := service.Func("test", func(ctx service.Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	r := service.NewRunner(NewNullListenerFull())

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustEqual(service.Starting, r.State(s1))
	mustStateError(tt, r.Start(s1, rdy), service.Halted, service.Starting)

	close(ready)

	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustOK(r.Halt(dto, s1))
}

func TestRunnerServiceWithHaltingGoroutines(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	var c uint32
	yep := make(chan struct{})

	ready := make(chan struct{})
	s1 := service.Func("test", func(ctx service.Context) error {
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
	})

	r := service.NewRunner(NewNullListenerFull())

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustEqual(service.Starting, r.State(s1))
	mustStateError(tt, r.Start(s1, rdy), service.Halted, service.Starting)

	close(ready)

	tt.MustOK(service.WhenReady(dto, rdy))
	tt.MustOK(r.Halt(dto, s1))

	select {
	case <-yep:
	case <-time.After(2 * time.Second):
		panic("timeout waiting for nested goroutine to stop")
	}
}

func TestRunnerServiceWithHaltingGoroutinesOnEndError(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	var c uint32
	yep := make(chan struct{})

	ready := make(chan struct{})
	s1 := service.Func("test", func(ctx service.Context) error {
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
	})

	r := service.NewRunner(NewNullListenerFull())

	rdy := service.NewReadySignal()
	tt.MustOK(r.Start(s1, rdy))
	tt.MustEqual(service.Starting, r.State(s1))
	mustStateError(tt, r.Start(s1, rdy), service.Halted, service.Starting)

	close(ready)

	tt.MustOK(service.WhenReady(dto, rdy))

	select {
	case <-yep:
	case <-time.After(2 * time.Second):
		panic("timeout waiting for nested goroutine to stop")
	}
}

func TestRunnerServiceHaltedNotRetained(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	r := service.NewRunner(nil)

	tt.MustOK(r.Start(s1, nil))
	service.MustEnsureHalt(r, dto*10, s1)

	services := r.Services(service.AnyState, 0)
	tt.MustEqual(0, len(services))
}

func TestRunnerServiceEndedNotRetained(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&TimedService{}).Init() // should return immediately

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	ew := lc.EndWaiter(s1, 1)

	tt.MustOK(r.Start(s1, nil))
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageRun, Err: service.ErrServiceEnded}}, lc.Ends(s1))

	services := r.Services(service.AnyState, 0)
	tt.MustEqual(0, len(services))
}

func TestRunnerHaltWhileHalting(t *testing.T) {
	// If we attempt to halt a service which is in the halting state, it should
	// wait until the service is halted.

	t.Parallel()

	type haltFunc func(r service.Runner, s service.Service) error

	for _, hf := range []haltFunc{
		func(r service.Runner, s service.Service) error { return r.Halt(dto, s) },
		func(r service.Runner, s service.Service) error { return service.EnsureHalt(r, dto, s) },
	} {
		t.Run("", func(t *testing.T) {
			tt := assert.WrapTB(t)

			haltDelay := 10 * tscale
			s1 := (&BlockingService{HaltDelay: haltDelay}).Init()
			lc := NewListenerCollector()
			r := service.NewRunner(lc)

			tt.MustOK(service.StartWait(r, dto, s1))
			tt.MustAssert(r.State(s1) == service.Started)

			ts := make(chan time.Time, 2)
			go func() {
				if err := hf(r, s1); err != nil {
					panic(err)
				}
				ts <- time.Now()
			}()
			go func() {
				if err := hf(r, s1); err != nil {
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
		})
	}
}

func TestRunnerHaltOverlap(t *testing.T) {
	// The fuzzer unearthed some issues where services would find their way into
	// the "started" or "starting" states during the execution of Runner.Halt(),
	// and *between* the calls to Runner.Halting() and Runner.Halted().

	t.Parallel()

	type haltFunc func(r service.Runner, s service.Service) error

	for _, hf := range []haltFunc{
		func(r service.Runner, s service.Service) error { return r.Halt(dto, s) },
		func(r service.Runner, s service.Service) error { return service.EnsureHalt(r, dto, s) },
	} {
		t.Run("", func(t *testing.T) {
			tt := assert.WrapTB(t)

			haltDelay := 10 * tscale
			s1 := (&BlockingService{HaltDelay: haltDelay}).Init()
			lc := NewListenerCollector()
			r := service.NewRunner(lc)

			tt.MustOK(service.StartWait(r, dto, s1))
			tt.MustAssert(r.State(s1) == service.Started)

			ts := make(chan time.Time, 2)
			go func() {
				if err := hf(r, s1); err != nil {
					panic(err)
				}
				ts <- time.Now()
			}()
			go func() {
				if err := hf(r, s1); err != nil {
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
		})
	}
}
