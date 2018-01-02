package service

import (
	"errors"
	"testing"

	"github.com/shabbyrobe/golib/assert"
	"github.com/shabbyrobe/golib/errtools"
)

func assertStartHaltCount(tt assert.T, scnt, hcnt int, ss ...statService) {
	tt.Helper()
	for idx, s := range ss {
		tt.MustEqual(scnt, s.Starts(), "%d - %s", idx, s.ServiceName())
		tt.MustEqual(hcnt, s.Halts(), "%d - %s", idx, s.ServiceName())
	}
}

func TestGroup(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()
	r := NewRunner(newDummyListener())
	g := NewGroup("yep", []Service{s1, s2})

	tt.MustOK(r.StartWait(g, dto))
	tt.MustOK(r.Halt(g, dto))
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
	g := NewGroup("yep", []Service{s1, s2, s3})

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(g, dto))
	defer MustEnsureHalt(r, g, dto)
	<-ew

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
	g := NewGroup("yep", []Service{s1, s2, s3})

	tt.MustEqual(e1, errtools.Cause(r.StartWait(g, dto)))
	defer MustEnsureHalt(r, g, dto)

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
	g := NewGroup("yep", []Service{s1, s2, s3})

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(g, dto))
	defer MustEnsureHalt(r, g, dto)
	<-ew

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
	g := NewGroup("yep", []Service{s1, s2, s3})

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(g, dto))
	<-ew

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
	g := NewGroup("yep", []Service{s1, s2, s3})

	ew := lc.endWaiter(g)
	tt.MustOK(r.StartWait(g, dto))
	<-ew

	assertStartHaltCount(tt, 1, 1, s1, s2, s3)
	tt.MustEqual(1, len(lc.ends(g)))

	tt.MustOK(EnsureHalt(r, g, dto))
}

func TestGroupRunTwice(t *testing.T) {
	tt := assert.WrapTB(t)

	s1 := (&blockingService{}).Init()
	s2 := (&blockingService{}).Init()

	r1 := NewRunner(newDummyListener())
	r2 := NewRunner(newDummyListener())
	g := NewGroup("yep", []Service{s1, s2})

	tt.MustOK(r1.StartWait(g, dto))
	tt.MustOK(r2.StartWait(g, dto))
	tt.MustOK(r1.Halt(g, dto))
	tt.MustOK(r2.Halt(g, dto))

	assertStartHaltCount(tt, 2, 2, s1, s2)
}
