package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
	"github.com/shabbyrobe/golib/errtools"
)

func mustStateError(tt assert.T, err error, expected State, current State) {
	tt.Helper()
	tt.MustAssert(err != nil, "state error not found")
	serr, ok := err.(*errState)
	tt.MustAssert(ok)
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

	tt.MustOK(r.StartWait(s1, dto))
	tt.MustAssert(r.State(s1) == Started)

	// Unlike runner.Halt(), an arbitrary number of calls to EnsureHalted
	// should be OK
	tt.MustOK(EnsureHalt(r, s1, dto))
	tt.MustOK(EnsureHalt(r, s1, dto))
	tt.MustOK(EnsureHalt(r, s1, dto))

	mustStateError(tt, r.Halt(s1, dto), Starting|Started, Halted)

	s2 := &dummyService{runTime: 1 * tscale}
	e2 := lc.endWaiter(s2)
	tt.MustOK(r.StartWait(s2, dto))
	err := EnsureHalt(r, s2, 1*time.Nanosecond)
	tt.MustAssert(IsErrHaltTimeout(err), err)
	mustRecv(tt, e2, dto)
}

func TestRunnerStartWait(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	for i := 0; i < 5; i++ {
		tt.MustOK(r.StartWait(s1, dto))
		tt.MustAssert(r.State(s1) == Started)

		mustStateError(tt, r.StartWait(s1, dto), Halted, Started)

		tt.MustOK(r.Halt(s1, dto))
		tt.MustAssert(r.State(s1) == Halted)

		mustStateError(tt, r.Halt(s1, dto), Starting|Started, Halted)
	}
}

func TestRunnerStart(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 2 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustAssert(r.State(s1) == Starting)
	mustStateError(tt, r.Start(s1), Halted, Starting)

	tt.MustOK(<-r.WhenReady(dto))
	tt.MustOK(r.Halt(s1, dto))
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
	tt.MustOK(<-r1.WhenReady(dto))
	tt.MustOK(<-r2.WhenReady(dto))
	tt.MustAssert(r1.State(s1) == Started)
	tt.MustAssert(r2.State(s1) == Started)
	tt.MustOK(r1.Halt(s1, dto))
	tt.MustOK(r2.Halt(s1, dto))
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
	tt.MustOK(<-r.WhenReady(dto))
	tt.MustOK(r.Halt(s1, dto))
	tt.MustOK(r.Halt(s2, dto))
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

	tt.MustEqual(serr, errtools.Cause(r.StartWait(s1, dto)))
	tt.MustOK(<-r.WhenReady(dto))
}

func TestRunnerStartErrorBeforeReadyIsReturnedByWhenReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("fail")
	s1 := &dummyService{startFailure: serr}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(serr, errtools.Cause(<-r.WhenReady(dto)))
}

func TestRunnerStartFailureBeforeReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	serr := errors.New("1")
	s1 := &dummyService{startFailure: serr}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(serr, errtools.Cause(<-r.WhenReady(dto)))
}

func TestRunnerStartWaitServiceEndsAfterReady(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 10 * time.Millisecond} // should return immediately
	lc := newListenerCollector()
	r := NewRunner(lc)
	ew := lc.endWaiter(s1)

	tt.MustOK(r.StartWait(s1, dto))
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

	tt.MustEqual(e1, errtools.Cause(r.StartWait(s1, dto)))
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
	tt.MustOK(<-r.WhenReady(1 * time.Second))
	mustStateError(tt, r.Halt(s1, dto), Starting|Started, Halted)

	tt.MustOK(r.Halt(s2, dto))
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

	tt.MustOK(r.StartWait(s1, dto))
	mustRecv(tt, ew1, dto)

	tt.MustOK(r.Start(s2))
	tt.MustOK(<-r.WhenReady(dto))
	mustStateError(tt, r.Halt(s1, dto), Starting|Started, Halted)

	tt.MustOK(r.Halt(s2, dto))
	mustRecv(tt, ew2, dto)
}

func TestRunnerReadyTimeout(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 3 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustAssert(r.State(s1) == Starting)
	tt.MustAssert(IsErrWaitTimeout(<-r.WhenReady(1 * tscale)))
	tt.MustOK(r.Halt(s1, dto))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartHaltWhileInStartDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{startDelay: 2 * tscale}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Halt(s1, dto))
	tt.MustAssert(r.State(s1) == Halted)
}

func TestRunnerStartHaltImmediately(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	r := NewRunner(newDummyListener())

	s1 := (&blockingService{}).Init()
	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Halt(s1, dto))
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
	tt.MustOK(r.StartWait(s1, dto))
	defer tt.MustOK(r.Halt(s1, dto))

	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerHaltDelay(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	delay := 2 * tscale
	s1 := (&blockingService{haltDelay: delay}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(s1, dto))

	tm := time.Now()
	tt.MustOK(r.Halt(s1, dto))
	tt.MustAssert(time.Since(tm) > delay)
}

func TestRunnerUnregister(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(s1, dto))
	tt.MustOK(r.Halt(s1, dto))
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

	tt.MustOK(r.StartWait(s1, dto))
	tt.MustAssert(r.State(s1) == Started)

	err := (r.Unregister(s1))
	mustStateError(tt, err, Halted, Started)

	tt.MustOK(r.Halt(s1, dto))
}

func TestHaltableSleep(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	// We have to use MinHaltableSleep instead of tscale here
	// to make sure we use the channel-based version of the sleep
	// function
	s1 := &dummyService{runTime: MinHaltableSleep, haltingSleep: true}
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(s1, dto))
	tm := time.Now()
	tt.MustOK(r.Halt(s1, dto))
	tt.MustAssert(time.Since(tm) < time.Duration(float64(MinHaltableSleep)*0.9))

	s1.haltingSleep = false
	tt.MustOK(r.StartWait(s1, dto))
	tm = time.Now()
	tt.MustOK(r.Halt(s1, dto))
	since := time.Since(tm)

	// only test for 95% of the delay because the timers aren't perfect. sometimes
	// (though very, very rarely) we see test failures like this: "sleep time
	// 49.957051ms, expected 50ms"
	lim := time.Duration(float64(MinHaltableSleep) * 0.95)
	tt.MustAssert(since >= lim, "sleep time %s, expected %s", since, MinHaltableSleep)
}

func TestRunnerOnError(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	var s1StartTime, s2StartTime, runTime time.Duration = 4, 6, 10
	var s1Tick, s2Tick time.Duration = 2, 3
	s1Expected := int((s1StartTime+runTime)/s1Tick) - 1
	s2Expected := int((s2StartTime+runTime)/s2Tick) - 1

	s1 := (&errorService{startDelay: s1StartTime * tscale}).Init()
	s2 := (&errorService{startDelay: s2StartTime * tscale}).Init()

	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		t1 := time.NewTicker(s1Tick * tscale)
		t2 := time.NewTicker(s2Tick * tscale)
		for {
			select {
			case t := <-t1.C:
				s1.errc <- fmt.Errorf("s1: %v", t)
			case t := <-t2.C:
				s2.errc <- fmt.Errorf("s2: %v", t)
			case <-done:
				close(stop)
				return
			}
		}
	}()

	lc := newListenerCollector()
	r := NewRunner(lc)
	tt.MustOK(r.Start(s1))
	tt.MustOK(r.Start(s2))
	tt.MustOK(<-r.WhenReady(dto))

	time.Sleep(runTime * tscale) // let a few more errors accumulate
	tt.MustOK(r.HaltAll(dto))
	close(done)
	<-stop

	tt.MustAssert(len(lc.errs(s1)) >= s1Expected)
	tt.MustAssert(len(lc.errs(s2)) >= s2Expected)
}

func TestRunnerHaltAll(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustOK(r.StartWait(s1, dto))
	tt.MustOK(r.StartWait(s2, dto))

	tt.MustOK(r.HaltAll(dto))
}

func TestRunnerServices(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())

	tt.MustEqual(0, len(r.Services(AnyState)))
	tt.MustOK(r.StartWait(s1, dto))
	tt.MustOK(r.StartWait(s2, dto))

	tt.MustEqual([]Service{s1, s2}, r.Services(AnyState))
	tt.MustEqual([]Service{s1, s2}, r.Services(Started))

	tt.MustOK(r.HaltAll(dto))
	tt.MustEqual([]Service{s1, s2}, r.Services(Halted))

	tt.MustOK(r.Unregister(s1))
	tt.MustEqual([]Service{s2}, r.Services(AnyState))
}

func TestRunnerServiceFunc(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	ready := make(chan struct{})
	s1 := ServiceFunc("test", func(ctx Context) error {
		<-ready
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Halt()
		return nil
	})

	r := NewRunner(newDummyListener())

	tt.MustOK(r.Start(s1))
	tt.MustEqual(Starting, r.State(s1))
	mustStateError(tt, r.Start(s1), Halted, Starting)

	close(ready)

	tt.MustOK(<-r.WhenReady(dto))
	tt.MustOK(r.Halt(s1, dto))
}
