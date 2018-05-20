package servicetest

import (
	"errors"
	"fmt"
	"testing"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

type statService interface {
	ServiceName() service.Name
	Starts() int
	Halts() int
}

func assertStartHaltCount(tt assert.T, scnt, hcnt int, ss ...statService) {
	tt.Helper()
	for idx, s := range ss {
		tt.MustEqual(scnt, s.Starts(), "expected %d starts, found %d: @%d - %s", scnt, s.Starts(), idx, s.ServiceName())
		tt.MustEqual(hcnt, s.Halts(), "expected %d halts, found %d: @%d - %s", hcnt, s.Halts(), idx, s.ServiceName())
	}
}

func TestGroup(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	r := service.NewRunner(NewNullListenerFull())
	g := service.NewGroup("yep", s1, s2)

	tt.MustOK(r.StartWait(dto, g))
	tt.MustOK(r.Halt(dto, g))
	assertStartHaltCount(tt, 1, 1, s1, s2)

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupEndOne(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	s3 := (&TimedService{RunTime: 2 * tscale}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustOK(r.StartWait(dto, g))
	defer service.MustEnsureHalt(r, dto, g)
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.Ends(g)))

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupOneFailsBeforeReady(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("boom")
	s1 := (&BlockingService{StartDelay: tscale}).Init()
	s2 := (&BlockingService{StartDelay: tscale}).Init()
	s3 := (&TimedService{StartFailure: e1}).Init() // should end immediately

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	tt.MustEqual(e1, cause(r.StartWait(dto, g)))
	defer service.MustEnsureHalt(r, dto, g)

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 1, 0, s3)
	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupOneFailsAfterReady(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("boom")
	s1 := (&BlockingService{StartDelay: tscale}).Init()
	s2 := (&BlockingService{StartDelay: tscale}).Init()
	s3 := (&TimedService{RunFailure: e1}).Init() // should end immediately

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustOK(r.StartWait(dto, g))
	defer service.MustEnsureHalt(r, dto, g)
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.Ends(g)))
	tt.MustEqual(e1, lc.Ends(g)[0].Err)

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupEndMultiple(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&TimedService{RunTime: 2 * tscale}).Init()
	s3 := (&TimedService{RunTime: 2 * tscale}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustOK(r.StartWait(dto, g))
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.Ends(g)))

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupEndAll(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&TimedService{RunTime: 2 * tscale}).Init()
	s2 := (&TimedService{RunTime: 2 * tscale}).Init()
	s3 := (&TimedService{RunTime: 2 * tscale}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustOK(r.StartWait(dto, g))
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.Ends(g)))

	tt.MustOK(service.EnsureHalt(r, dto, g))
}

func TestGroupRunTwice(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()

	r1 := service.NewRunner(NewNullListenerFull())
	r2 := service.NewRunner(NewNullListenerFull())
	g := service.NewGroup("yep", s1, s2)

	tt.MustOK(r1.StartWait(dto, g))
	tt.MustOK(r2.StartWait(dto, g))
	tt.MustOK(r1.Halt(dto, g))
	tt.MustOK(r2.Halt(dto, g))

	assertStartHaltCount(tt, 2, 2, s1, s2)
}

func TestGroupStartError(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure")

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	s3 := (&TimedService{StartFailure: e1}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustEqual(e1, cause(r.StartWait(dto, g)))

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 1, 0, s3)

	// The start error should be passed to the failer
	mustRecv(tt, ew, dto)

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupStartMultipleErrors(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure 1")
	e2 := errors.New("start failure 2")

	s1 := (&BlockingService{}).Init()
	s2 := (&TimedService{StartFailure: e1}).Init()
	s3 := (&TimedService{StartFailure: e2}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	expErr := []error{e1, e2}
	foundErr := cause(r.StartWait(dto, g))
	tt.MustEqual(expErr, causeListSorted(foundErr))

	assertStartHaltCount(tt, 1, 1, s1)
	assertStartHaltCount(tt, 1, 0, s2, s3)

	// The start error should be passed to the failer
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageReady, Err: foundErr}}, lc.Ends(g))

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupRunnerErrorWithFailingStart(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure")

	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	s3 := (&BlockingService{}).Init()

	lc := NewListenerCollector()
	r := service.NewRunner(lc)

	runnerBuilder := func(l service.Listener) service.Runner {
		return &runnerWithFailingStart{
			failAfter: 2,
			err:       e1,
			Runner:    service.NewRunner(l),
		}
	}
	g := service.NewGroupWithRunnerBuilder("yep", runnerBuilder, s1, s2, s3)

	ew := lc.EndWaiter(g, 1)
	tt.MustEqual(e1, cause(r.StartWait(dto, g)))

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 0, 0, s3)

	// The start error should be passed to the failer
	mustRecv(tt, ew, dto)
	tt.MustEqual([]ListenerCollectorEnd{{Stage: service.StageReady, Err: e1}}, lc.Ends(g))

	tt.MustEqual(service.Halted, r.State(g))
}

func TestGroupOnError(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&ErrorService{}).Init()
	s2 := (&ErrorService{}).Init()

	expected := 3 // at least this many errors should occur

	go func() {
		for i := 0; i < expected; i++ {
			s1.errc <- fmt.Errorf("s1: %d", i)
			s2.errc <- fmt.Errorf("s2: %d", i)
		}
	}()

	lc := NewListenerCollector()

	r := service.NewRunner(lc)
	g := service.NewGroup("yep", s1, s2)
	ew := lc.ErrWaiter(g, expected)

	tt.MustOK(r.StartWait(tscale*10, g))

	errs := ew.TakeN(expected, tscale*10)
	tt.MustOK(r.Halt(dto, g))

	tt.MustAssert(len(errs) >= expected, "%d < %d", errs, expected)
}

func TestGroupRestartableEnabled(t *testing.T) {
	tt := assert.WrapTB(t)
	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	gr := service.NewGroup("a", s1, s2)

	lc := NewListenerCollector()
	runner := service.NewRunner(lc)
	defer runner.HaltAll(tscale*10, 0)

	tt.MustOK(runner.StartWait(tscale*10, gr))
	tt.MustOK(runner.Halt(tscale*10, gr))
	tt.MustOK(runner.StartWait(tscale*10, gr))
}

func TestGroupRestartableDisabled(t *testing.T) {
	tt := assert.WrapTB(t)
	s1 := (&BlockingService{}).Init()
	s2 := (&BlockingService{}).Init()
	gr := service.NewGroup("a", s1, s2)
	gr.Restartable(false)

	lc := NewListenerCollector()
	runner := service.NewRunner(lc)
	defer runner.HaltAll(tscale*10, 0)

	tt.MustOK(runner.StartWait(tscale*10, gr))
	tt.MustOK(runner.Halt(tscale*10, gr))

	err := runner.StartWait(tscale*10, gr)
	tt.MustAssert(err != nil)
}
