package service

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

func mustStateError(tt assert.T, err error, expected State, current State) {
	tt.Helper()
	tt.MustAssert(err != nil, "state error not found")
	serr, ok := err.(*errState)
	tt.MustAssert(ok, err)
	tt.MustEqual(expected, serr.Expected, serr.Error())
	tt.MustEqual(current, serr.Current, serr.Error())
}

func mustRecv(tt assert.T, waiter chan struct{}, timeout time.Duration) {
	tt.Helper()
	after := time.After(timeout)
	select {
	case <-waiter:
	case <-after:
		tt.Fatalf("waiter did not yield within timeout %v", timeout)
	}
}

func TestEnsureHalt(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	lc := newListenerCollector()
	r := NewRunner(lc)

	tt.MustOK(r.StartWait(dto, s1))
	tt.MustAssert(r.State(s1) == Started)

	// Unlike runner.Halt(), an arbitrary number of calls to EnsureHalted
	// should be OK
	tt.MustOK(EnsureHalt(r, dto, s1))
	tt.MustOK(EnsureHalt(r, dto, s1))
	tt.MustOK(EnsureHalt(r, dto, s1))

	herr := r.Halt(dto, s1)
	tt.MustAssert(IsErrServiceUnknown(herr), herr)

	// runTime must be long enough to ensure that EnsureHalt times out
	s2 := (&unhaltableService{}).Init()
	e2 := lc.endWaiter(s2)
	tt.MustOK(r.StartWait(dto, s2))
	err := EnsureHalt(r, 1*time.Nanosecond, s2) // 1ns is the shortest possible timeout; 0 means wait forever
	tt.MustAssert(IsErrHaltTimeout(err), err)
	close(s2.halt)
	mustRecv(tt, e2, dto)
}

func TestRunnerStartWait(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	for i := 0; i < 5; i++ {
		tt.MustOK(r.StartWait(dto, s1))
		tt.MustAssert(r.State(s1) == Started)
		tt.MustOK(r.WhenReady(dto, s1)) // make sure WhenReady works in this state

		mustStateError(tt, r.StartWait(dto, s1), Halted, Started)

		tt.MustOK(r.Halt(dto, s1))
		tt.MustAssert(r.State(s1) == Halted)
		tt.MustOK(r.WhenReady(dto, s1)) // make sure WhenReady works in this state

		tt.MustAssert(IsErrServiceUnknown(r.Halt(dto, s1)))
	}
}

func TestRunnerStart(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// FIXME: Must delay start enough to allow Starting state to be checked.
	// This is a brittle test - we could use OnServiceState to help block
	// the service until Starting has been checked.
	s1 := (&blockingService{startDelay: 2 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustAssert(r.State(s1) == Starting)
	mustStateError(tt, r.Start(s1), Halted, Starting)

	tt.MustOK(r.WhenReady(dto, s1))
	tt.MustAssert(r.State(s1) == Started)

	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerSameServiceMultipleRunners(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 2 * tscale}).Init()
	r1 := NewRunner(newDummyListener())
	r2 := NewRunner(newDummyListener())

	tt.MustOK(r1.Start(s1))
	tt.MustOK(r2.Start(s1))
	tt.MustAssert(r1.State(s1) == Starting)
	tt.MustAssert(r2.State(s1) == Starting)
	tt.MustOK(r1.WhenReady(dto, s1))
	tt.MustOK(r2.WhenReady(dto, s1))
	tt.MustAssert(r1.State(s1) == Started)
	tt.MustAssert(r2.State(s1) == Started)
	tt.MustOK(r1.Halt(dto, s1))
	tt.MustOK(r2.Halt(dto, s1))
	tt.MustAssert(r1.State(s1) == Halted)
	tt.MustAssert(r2.State(s1) == Halted)
}

func TestRunnerStartMultiple(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Start(s2))
	tt.MustOK(WhenAllReady(r, dto, s1, s2))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustOK(r.Halt(dto, s2))
}

func TestRunnerStartServiceEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := &dummyService{} // should return immediately

	lc := newListenerCollector()
	r := NewRunner(lc)
	ew := lc.endWaiter(s1)

	tt.MustOK(r.Start(s1))
	mustRecv(tt, ew, dto)
	tt.MustEqual([]listenerCollectorEnd{{err: ErrServiceEnded}}, lc.ends(s1))
}

func TestRunnerStartWaitErrorBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("1")
	s1 := &dummyService{startFailure: serr}
	r := NewRunner(newDummyListener())

	tt.MustEqual(serr, cause(r.StartWait(dto, s1)))
	tt.MustOK(r.WhenReady(dto, s1))
}

func TestRunnerStartErrorBeforeReadyIsReturnedByWhenReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("fail")
	s1 := &dummyService{startFailure: serr}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	result := cause(r.WhenReady(dto, s1))
	if result == nil {
		panic(nil)
	}
	tt.MustEqual(serr, result)
}

func TestRunnerStartFailureBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("1")
	s1 := &dummyService{startFailure: serr}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(serr, cause(r.WhenReady(dto, s1)))
}

func TestRunnerStartWaitServiceEndsAfterReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 10 * time.Millisecond} // should return immediately
	lc := newListenerCollector()
	r := NewRunner(lc)
	ew := lc.endWaiter(s1)

	tt.MustOK(r.StartWait(dto, s1))
	mustRecv(tt, ew, dto)
	tt.MustEqual([]listenerCollectorEnd{{err: ErrServiceEnded}}, lc.ends(s1))
}

func TestRunnerStartWaitServiceEndsBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	e1 := errors.New("e1")
	s1 := &dummyService{startFailure: e1} // should return immediately
	lc := newListenerCollector()
	r := NewRunner(lc)

	tt.MustEqual(e1, cause(r.StartWait(dto, s1)))
}

func TestRunnerStartFirstThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := &dummyService{} // should return immediately
	s2 := (&blockingService{}).Init()

	lc := newListenerCollector()
	r := NewRunner(lc)

	ew1 := lc.endWaiter(s1)
	ew2 := lc.endWaiter(s2)

	tt.MustOK(r.Start(s1))
	mustRecv(tt, ew1, dto)

	tt.MustOK(r.Start(s2))
	tt.MustOK(WhenAllReady(r, 1*time.Second, s1, s2))
	mustStateError(tt, r.Halt(dto, s1), Starting|Started, Halted)

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerStartWaitFirstThenStartSecondAfterFirstEnds(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We need to make the dummy service run for a little while to make
	// sure that the End happens after the Ready
	s1 := &dummyService{runTime: 2 * tscale}

	s2 := (&blockingService{}).Init()

	lc := newListenerCollector()
	r := NewRunner(lc)

	ew1 := lc.endWaiter(s1)
	ew2 := lc.endWaiter(s2)

	tt.MustOK(r.StartWait(dto, s1))
	mustRecv(tt, ew1, dto)

	tt.MustOK(r.Start(s2))
	tt.MustOK(r.WhenReady(dto, s2))
	mustStateError(tt, r.Halt(dto, s1), Starting|Started, Halted)

	tt.MustOK(r.Halt(dto, s2))
	mustRecv(tt, ew2, dto)
}

func TestRunnerReadyTimeout(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 5 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustAssert(r.State(s1) == Starting)
	tt.MustAssert(IsErrWaitTimeout(r.WhenReady(1*tscale, s1)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartHaltWhileInStartDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 2 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartHaltImmediately(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := NewRunner(newDummyListener())

	s1 := (&blockingService{}).Init()
	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartHaltAllImmediately(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := NewRunner(newDummyListener())

	s1 := (&blockingService{}).Init()
	tt.MustOK(r.Start(s1))
	tt.MustOK(r.HaltAll(dto))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := (&blockingService{startDelay: delay}).Init()
	r := NewRunner(newDummyListener())

	tm := time.Now()
	tt.MustOK(r.StartWait(dto, s1))
	defer tt.MustOK(r.Halt(dto, s1))

	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := (&blockingService{haltDelay: delay}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(dto, s1))

	tm := time.Now()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltingState(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 20 * tscale
	s1 := (&blockingService{haltDelay: delay}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(dto, s1))

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
			tt.MustOK(r.WhenReady(dto, s1))
			counts[state]++
			time.Sleep(tscale)
		}
	}()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) > delay)
	tt.MustAssert(r.State(s1) == Halted)
	atomic.StoreInt32(&stop, 1)
	counts := <-out
	tt.MustAssert(counts[Halting] > 0)
}

func TestRunnerRegisterMultiple(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Register(s1).Register(s1).StartWait(dto, s1))
	tt.MustEqual(1, len(r.Services(FindRegistered)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
	tt.MustEqual(Halted, r.State(s1))
	tt.MustEqual(1, len(r.Services(FindRegistered)))

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual(0, len(r.Services(FindRegistered)))
	tt.MustAssert(IsErrServiceUnknown(r.Unregister(s1)))
}

func TestRunnerUnregister(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Register(s1).StartWait(dto, s1))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(r.State(s1) == Halted)
	tt.MustEqual(Halted, r.State(s1))

	tt.MustOK(r.Unregister(s1))
	tt.MustAssert(IsErrServiceUnknown(r.Unregister(s1)))
}

func TestRunnerUnregisterWhileNotHalted(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Register(s1).StartWait(dto, s1))
	tt.MustEqual(0, len(r.Services(FindUnregistered)))
	tt.MustEqual(1, len(r.Services(FindRegistered)))
	tt.MustAssert(r.State(s1) == Started)

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual(0, len(r.Services(FindRegistered)))
	tt.MustEqual(1, len(r.Services(FindUnregistered)))
	tt.MustOK(r.Halt(dto, s1))
	tt.MustEqual(0, len(r.Services(AnyState)))
}

func TestHaltableSleep(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We have to use MinHaltableSleep instead of tscale here
	// to make sure we use the channel-based version of the sleep
	// function
	s1 := &dummyService{runTime: MinHaltableSleep, haltingSleep: true}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(dto, s1))
	tm := time.Now()
	tt.MustOK(r.Halt(dto, s1))
	tt.MustAssert(time.Since(tm) < time.Duration(float64(MinHaltableSleep)*0.9))

	s1.haltingSleep = false
	tt.MustOK(r.StartWait(dto, s1))
	tm = time.Now()
	tt.MustOK(r.Halt(dto, s1))
	since := time.Since(tm)

	// only test for 95% of the delay because the timers aren't perfect. sometimes
	// (though very, very rarely) we see test failures like this: "sleep time
	// 49.957051ms, expected 50ms"
	lim := time.Duration(float64(MinHaltableSleep) * 0.95)
	tt.MustAssert(since >= lim, "sleep time %s, expected %s", since, MinHaltableSleep)
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

	s1 := (&errorService{startDelay: s1StartTime}).Init()
	s2 := (&errorService{startDelay: s2StartTime}).Init()

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

	lc := newListenerCollector()
	ew1, ew2 := lc.errWaiter(s1, s1Expected), lc.errWaiter(s2, s2Expected)
	r := NewRunner(lc)
	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Start(s2))
	tt.MustOK(WhenAllReady(r, 100*tscale, s1, s2))

	s1Errs := ew1.Take(s1Expected, 10*runTime)
	s2Errs := ew2.Take(s2Expected, 10*runTime)
	tt.MustOK(r.HaltAll(dto))

	tt.MustAssert(len(s1Errs) >= s1Expected)
	tt.MustAssert(len(s2Errs) >= s2Expected)
}

func TestRunnerHaltAll(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(dto, s1))
	tt.MustOK(r.StartWait(dto, s2))

	tt.MustOK(r.HaltAll(dto))
}

func TestRunnerServices(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	s3 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustEqual(0, len(r.Services(AnyState)))

	r.Register(s1)
	tt.MustEqual([]Service{s1}, r.Services(FindHalted))

	tt.MustOK(r.StartWait(dto, s2))
	tt.MustOK(r.StartWait(dto, s3))

	tt.MustEqual([]Service{s1}, r.Services(FindHalted))
	tt.MustEqual([]Service{s2, s3}, r.Services(FindStarted))

	tt.MustOK(r.StartWait(dto, s1))
	tt.MustEqual([]Service{s1, s2, s3}, r.Services(AnyState))
	tt.MustEqual([]Service{s1, s2, s3}, r.Services(FindStarted))

	tt.MustOK(r.HaltAll(dto))

	// halted services are removed from the runner unless they are registered
	tt.MustEqual([]Service{s1}, r.Services(FindHalted))

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual([]Service{}, r.Services(AnyState))
}

func TestRunnerServiceFunc(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	ready := make(chan struct{})
	s1 := Func("test", func(ctx Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(Starting, r.State(s1))
	mustStateError(tt, r.Start(s1), Halted, Starting)

	close(ready)

	tt.MustOK(r.WhenReady(dto, s1))
	tt.MustOK(r.Halt(dto, s1))
}

func TestRunnerServiceWithHaltingGoroutines(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	var c uint32
	yep := make(chan struct{})

	ready := make(chan struct{})
	s1 := Func("test", func(ctx Context) error {
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

	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(Starting, r.State(s1))
	mustStateError(tt, r.Start(s1), Halted, Starting)

	close(ready)

	tt.MustOK(r.WhenReady(dto, s1))
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
	s1 := Func("test", func(ctx Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			atomic.AddUint32(&c, 1)
			close(yep)
		}()
		return fmt.Errorf("nup")
	})

	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(Starting, r.State(s1))
	mustStateError(tt, r.Start(s1), Halted, Starting)

	close(ready)

	tt.MustOK(r.WhenReady(dto, s1))

	select {
	case <-yep:
	case <-time.After(2 * time.Second):
		panic("timeout waiting for nested goroutine to stop")
	}
}
