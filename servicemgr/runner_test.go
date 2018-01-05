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

	tt.MustOK(StartListen(nil, s1))
	tt.MustAssert(State(s1) == service.Starting)

	tt.MustOK(WhenReady(dto, s1))
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
	tt.MustOK(WhenReady(dto, s1))

	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)
}

func TestRegisterUnregister(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingListener(0)
	tt.MustOK(Register(s1).StartWaitListen(10*tscale, l))
	tt.MustEqual(1, len(getListener().listeners))
	tt.MustEqual(1, len(Services(service.FindRegistered)))

	tt.MustOK(Halt(dto, s1))
	tt.MustAssert(State(s1) == service.Halted)

	tt.MustEqual(1, len(Services(service.FindRegistered)))
	tt.MustOK(Unregister(s1))
	tt.MustEqual(0, len(Services(service.FindRegistered)))
	tt.MustEqual(0, len(Services(service.FindUnregistered)))

	tt.MustEqual(0, len(getListener().listeners))
}

func TestListenerEndsOnHalt(t *testing.T) {
	defer Reset()

	tt := assert.WrapTB(t)

	s1 := &dummyService{runTime: 100 * tscale}

	l := newTestingListener(1)
	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustEqual(1, len(getListener().listeners))
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

	l := newTestingListener(2)
	tt.MustOK(StartWaitListen(10*tscale, l, s1))
	tt.MustOK(StartWaitListen(10*tscale, l, s2))
	tt.MustEqual(2, len(getListener().listeners))

	tt.MustOK(HaltAll(dto))
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
	tt.MustEqual(0, len(getListener().listeners))

	tt.MustEqual([]service.Service{s1, s2}, Services(service.AnyState))

	tt.MustOK(HaltAll(dto))
	tt.MustAssert(State(s1) == service.Halted)
	tt.MustAssert(State(s2) == service.Halted)

	tt.MustEqual([]service.Service{}, Services(service.AnyState))
}
