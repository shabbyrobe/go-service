package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func assertStartHaltCount(tt assert.T, scnt, hcnt int, ss ...statService) {
	tt.Helper()
	for idx, s := range ss {
		tt.MustEqual(scnt, s.Starts(), "expected %d starts, found %d: @%d - %s", scnt, s.Starts(), idx, s.ServiceName())
		tt.MustEqual(hcnt, s.Halts(), "expected %d halts, found %d: @%d - %s", hcnt, s.Halts(), idx, s.ServiceName())
	}
}

func TestGroup(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())
	g := NewGroup("yep", s1, s2)

	tt.MustOK(r.StartWait(dto, g))
	tt.MustOK(r.Halt(dto, g))
	assertStartHaltCount(tt, 1, 1, s1, s2)

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupEndOne(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	s3 := &dummyService{runTime: 2 * tscale}

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(dto, g))
	defer MustEnsureHalt(r, dto, g)
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.ends(g)))

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupOneFailsBeforeReady(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("boom")
	s1 := (&blockingService{startDelay: tscale}).Init()
	s2 := (&blockingService{startDelay: tscale}).Init()
	s3 := &dummyService{startFailure: e1} // should end immediately

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	tt.MustEqual(e1, cause(r.StartWait(dto, g)))
	defer MustEnsureHalt(r, dto, g)

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 1, 0, s3)
	tt.MustEqual(Halted, r.State(g))
}

func TestGroupOneFailsAfterReady(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("boom")
	s1 := (&blockingService{startDelay: tscale}).Init()
	s2 := (&blockingService{startDelay: tscale}).Init()
	s3 := &dummyService{runFailure: e1} // should end immediately

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(dto, g))
	defer MustEnsureHalt(r, dto, g)
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.ends(g)))
	tt.MustEqual(e1, lc.ends(g)[0].err)

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupEndMultiple(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := &dummyService{runTime: 2 * tscale}
	s3 := &dummyService{runTime: 2 * tscale}

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(dto, g))
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.ends(g)))

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupEndAll(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 2 * tscale}
	s2 := &dummyService{runTime: 2 * tscale}
	s3 := &dummyService{runTime: 2 * tscale}

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(dto, g))
	mustRecv(tt, ew, dto)

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.ends(g)))

	tt.MustOK(EnsureHalt(r, dto, g))
}

func TestGroupRunTwice(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()

	r1 := NewRunner(newDummyListener())
	r2 := NewRunner(newDummyListener())
	g := NewGroup("yep", s1, s2)

	tt.MustOK(r1.StartWait(dto, g))
	tt.MustOK(r2.StartWait(dto, g))
	tt.MustOK(r1.Halt(dto, g))
	tt.MustOK(r2.Halt(dto, g))

	assertStartHaltCount(tt, 2, 2, s1, s2)
}

func TestGroupStartError(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure")

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	s3 := &dummyService{startFailure: e1}

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustEqual(e1, cause(r.StartWait(dto, g)))

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 1, 0, s3)

	mustNotRecv(tt, ew)

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupStartMultipleErrors(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure 1")
	e2 := errors.New("start failure 2")

	s1 := (&blockingService{}).Init()
	s2 := &dummyService{startFailure: e1}
	s3 := &dummyService{startFailure: e2}

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	ew := lc.endWaiter(g)
	tt.MustEqual([]error{e1, e2}, causeListSorted(cause(r.StartWait(dto, g))))

	assertStartHaltCount(tt, 1, 1, s1)
	assertStartHaltCount(tt, 1, 0, s2, s3)

	mustNotRecv(tt, ew)

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupRunnerError(t *testing.T) {
	tt := assert.WrapTB(t)

	e1 := errors.New("start failure")

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	s3 := (&blockingService{}).Init()

	lc := newListenerCollector()
	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2, s3)

	g.runnerBuilder = func(l Listener) Runner {
		return &runnerWithFailingStart{
			failAfter: 2,
			err:       e1,
			Runner:    NewRunner(l),
		}
	}

	ew := lc.endWaiter(g)
	tt.MustEqual(e1, cause(r.StartWait(dto, g)))

	assertStartHaltCount(tt, 1, 1, s1, s2)
	assertStartHaltCount(tt, 0, 0, s3)

	mustNotRecv(tt, ew)

	tt.MustEqual(Halted, r.State(g))
}

func TestGroupOnError(t *testing.T) {
	t.Parallel()

	tt := assert.WrapTB(t)

	s1 := (&errorService{}).Init()
	s2 := (&errorService{}).Init()

	expected := 3 // at least this many errors should occur

	go func() {
		for i := 0; i < expected; i++ {
			s1.errc <- fmt.Errorf("s1: %d", i)
			s2.errc <- fmt.Errorf("s2: %d", i)
		}
	}()

	lc := newListenerCollector()

	r := NewRunner(lc)
	g := NewGroup("yep", s1, s2)
	ew := lc.errWaiter(g, expected)

	tt.MustOK(r.StartWait(tscale*10, g))

	errs := ew.Take(expected, tscale*10)
	tt.MustOK(r.Halt(dto, g))

	tt.MustAssert(len(errs) >= expected, "%d < %d", errs, expected)
}
