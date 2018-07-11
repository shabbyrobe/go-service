package servicetest

import (
	"sync"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type ListenerCollectorEnd struct {
	Stage service.Stage
	Err   error
}

type Waiter interface {
	TakeN(n int, timeout time.Duration) []error
	Take(timeout time.Duration) error
	C() <-chan error
}

type StateWaiter interface {
	TakeN(n int, timeout time.Duration) []service.StateChange
	Take(timeout time.Duration) *service.StateChange
	C() <-chan service.StateChange
}

// ListenerCollector allows you to test things that make use of service.Runner's
// listener.
//
// Please do not use it for any other purpose, it is only built to be useful in
// a test.
//
type ListenerCollector struct {
	services map[*service.Service]*listenerCollectorService
	states   chan service.StateChange
	lock     sync.Mutex
}

func NewListenerCollector() *ListenerCollector {
	return &ListenerCollector{
		services: make(map[*service.Service]*listenerCollectorService),
		states:   make(chan service.StateChange, 50), // FIXME: configurable
	}
}

func (t *ListenerCollector) RunnerOptions(opts ...service.RunnerOption) []service.RunnerOption {
	opts = append(opts, service.RunnerOnEnd(t.OnServiceEnd))
	opts = append(opts, service.RunnerOnError(t.OnServiceError))
	opts = append(opts, service.RunnerOnState(t.states))
	return opts
}

func (t *ListenerCollector) Errs(svc *service.Service) (out []error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	lsvc := t.services[svc]
	if lsvc == nil {
		return
	}
	for _, e := range lsvc.errs {
		out = append(out, e)
	}
	return
}

func (t *ListenerCollector) ErrWaiter(svc *service.Service, cap int) Waiter {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	if lsvc.errWaiter != nil {
		close(lsvc.errWaiter.c)
	}
	w := &errWaiter{c: make(chan error, cap)}
	lsvc.errWaiter = w
	t.lock.Unlock()

	return w
}

func (t *ListenerCollector) Ends(svc *service.Service) (out []ListenerCollectorEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()
	lsvc := t.services[svc]
	if lsvc == nil {
		return
	}
	for _, e := range lsvc.ends {
		out = append(out, *e)
	}
	return
}

func (t *ListenerCollector) EndWaiter(svc *service.Service, cap int) Waiter {
	// FIXME: endWaiter should have a timeout
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	if lsvc.endWaiter != nil {
		close(lsvc.endWaiter.c)
	}
	w := &errWaiter{c: make(chan error, cap)}
	lsvc.endWaiter = w
	t.lock.Unlock()

	return w
}

func (t *ListenerCollector) StateWaiter(svc *service.Service, cap int) StateWaiter {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	w := &stateWaiter{c: t.states}
	lsvc.stateWaiter = w
	t.lock.Unlock()

	return w
}

func (t *ListenerCollector) OnServiceError(stage service.Stage, svc *service.Service, err error) {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	lsvc.errs = append(lsvc.errs, err)

	if lsvc.errWaiter != nil {
		lsvc.errWaiter.c <- err
	}
	t.lock.Unlock()
}

func (t *ListenerCollector) OnServiceEnd(stage service.Stage, svc *service.Service, err error) {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]

	lsvc.ends = append(lsvc.ends, &ListenerCollectorEnd{
		Stage: stage,
		Err:   cause(err),
	})
	if lsvc.endWaiter != nil {
		lsvc.endWaiter.c <- err
	}
	t.lock.Unlock()
}

type listenerCollectorService struct {
	errs        []error
	states      []service.StateChange
	ends        []*ListenerCollectorEnd
	endWaiter   *errWaiter
	errWaiter   *errWaiter
	stateWaiter *stateWaiter
}

type errWaiter struct {
	c chan error
}

func (e *errWaiter) C() <-chan error { return e.c }

func (e *errWaiter) Take(timeout time.Duration) error {
	errs := e.TakeN(1, timeout)
	if len(errs) == 1 {
		return errs[0]
	} else if len(errs) == 0 {
		return nil
	} else {
		panic("unexpected errors")
	}
}

func (e *errWaiter) TakeN(n int, timeout time.Duration) []error {
	out := make([]error, n)
	for i := 0; i < n; i++ {
		wait := time.After(timeout)
		select {
		case out[i] = <-e.c:
		case <-wait:
			panic("errwaiter timeout")
		}
	}
	return out
}

type stateWaiter struct {
	c chan service.StateChange
}

func (e *stateWaiter) C() <-chan service.StateChange { return e.c }

func (e *stateWaiter) Take(timeout time.Duration) *service.StateChange {
	vs := e.TakeN(1, timeout)
	if len(vs) == 1 {
		return &vs[0]
	} else if len(vs) == 0 {
		return nil
	} else {
		panic("unexpected states")
	}
}

func (e *stateWaiter) TakeN(n int, timeout time.Duration) []service.StateChange {
	out := make([]service.StateChange, n)
	for i := 0; i < n; i++ {
		wait := time.After(timeout)
		select {
		case out[i] = <-e.c:
		case <-wait:
			panic("statewaiter timeout")
		}
	}
	return out
}
