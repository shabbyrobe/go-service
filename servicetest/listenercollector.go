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

type listenerCollectorService struct {
	errs       []service.Error
	states     []service.State
	ends       []*ListenerCollectorEnd
	endWaiters []chan struct{}
	errWaiters []*ErrWaiter
}

type ListenerCollector struct {
	services map[service.Service]*listenerCollectorService
	lock     sync.Mutex
}

func NewListenerCollector() *ListenerCollector {
	return &ListenerCollector{
		services: make(map[service.Service]*listenerCollectorService),
	}
}

func (t *ListenerCollector) errs(svc service.Service) (out []service.Error) {
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

func (t *ListenerCollector) ends(svc service.Service) (out []ListenerCollectorEnd) {
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

func (t *ListenerCollector) endWaiter(svc service.Service) chan struct{} {
	// FIXME: endWaiter should have a timeout
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	w := make(chan struct{}, 1)
	lsvc.endWaiters = append(lsvc.endWaiters, w)
	t.lock.Unlock()

	return w
}

func (t *ListenerCollector) errWaiter(svc service.Service, cap int) *ErrWaiter {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	w := &ErrWaiter{C: make(chan error, cap)}
	lsvc.errWaiters = append(lsvc.errWaiters, w)
	t.lock.Unlock()

	return w
}

func (t *ListenerCollector) OnServiceState(svc service.Service, state service.State) {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	lsvc.states = append(lsvc.states, state)
	t.lock.Unlock()
}

func (t *ListenerCollector) OnServiceError(svc service.Service, err service.Error) {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]
	lsvc.errs = append(lsvc.errs, err)

	if len(lsvc.errWaiters) > 0 {
		for _, w := range lsvc.errWaiters {
			w.C <- err
		}
	}
	t.lock.Unlock()
}

func (t *ListenerCollector) OnServiceEnd(stage service.Stage, svc service.Service, err service.Error) {
	t.lock.Lock()
	if t.services[svc] == nil {
		t.services[svc] = &listenerCollectorService{}
	}
	lsvc := t.services[svc]

	lsvc.ends = append(lsvc.ends, &ListenerCollectorEnd{
		Stage: stage,
		Err:   cause(err),
	})
	if len(lsvc.endWaiters) > 0 {
		for _, w := range lsvc.endWaiters {
			close(w)
		}
		lsvc.endWaiters = nil
	}
	t.lock.Unlock()
}

type ErrWaiter struct {
	C chan error
}

func (e *ErrWaiter) Take(n int, timeout time.Duration) []error {
	out := make([]error, n)
	for i := 0; i < n; i++ {
		wait := time.After(timeout)
		select {
		case out[i] = <-e.C:
		case <-wait:
			panic("errwaiter timeout")
		}
	}
	return out
}
