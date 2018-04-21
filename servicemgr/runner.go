// +build ignore

// This is an attempt to clean up the listener/runner state complexity in servicemgr.
// It's not complete yet.

package servicemgr

import (
	"fmt"
	"sync"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type manager struct {
	runner service.Runner

	listeners        map[service.Service]Listener
	listenersNHError map[service.Service]NonHaltingErrorListener
	listenersState   map[service.Service]StateListener
	retained         map[service.Service]bool

	defaultListener service.Listener
	lock            sync.Mutex
}

func newManager() *manager {
	m := &manager{
		listeners:        make(map[service.Service]Listener),
		listenersNHError: make(map[service.Service]NonHaltingErrorListener),
		listenersState:   make(map[service.Service]StateListener),
		retained:         make(map[service.Service]bool),
	}
	m.runner = service.NewRunner(m)
	return m
}

var _ service.Runner = &manager{}

func (m *manager) SetDefaultListener(l service.Listener) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.defaultListener = l
}

func (m *manager) State(svc service.Service) service.State {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.State(svc)
}

func (m *manager) StartWait(timeout time.Duration, svc service.Service) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.StartWait(timeout, svc)
}

func (m *manager) Start(svc service.Service, rdy service.ReadySignal) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.Start(svc, rdy)
}

func (m *manager) Halt(timeout time.Duration, svc service.Service) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if err := m.runner.Halt(timeout, svc); err != nil {
		return err
	}
	return nil
}

// HaltAll is not supported
func (m *manager) HaltAll(timeout time.Duration, errlimit int) (n int, err error) {
	// It's too hard to manage the shared listener and still support HaltAll for now.
	// global Shutdown is the way to do this.
	return 0, fmt.Errorf("not supported")
}

func (m *manager) Services(state service.StateQuery) []service.Service {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.Services(state)
}

func (m *manager) Register(svc service.Service) service.Runner {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.Register(svc)
}

func (m *manager) Unregister(svc service.Service) (deferred bool, err error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.runner.Unregister(svc)
}

func (m *manager) Listen(service service.Service, l Listener) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.listeners[service] = l
	if el, ok := l.(NonHaltingErrorListener); ok {
		m.listenersNHError[service] = el
	}
	if sl, ok := l.(StateListener); ok {
		m.listenersState[service] = sl
	}
}

func (m *manager) Unlisten(service service.Service) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.unlisten(service)
}

// unlisten expects m.lock is acquired
func (m *manager) unlisten(service service.Service) {
	delete(m.listeners, service)
	delete(m.listenersNHError, service)
	delete(m.listenersState, service)
	delete(m.retained, service)
}

func (m *manager) OnServiceError(service service.Service, err service.Error) {
	m.lock.Lock()
	l, ok := m.listenersNHError[service]
	if !ok {
		l = m.defaultListener
	}
	m.lock.Unlock()
	if l != nil {
		l.OnServiceError(service, err)
	}
}

func (m *manager) OnServiceEnd(service service.Service, err service.Error) {
	m.lock.Lock()
	l, ok := m.listeners[service]
	if !ok {
		l = m.defaultListener
	}
	if !m.retained[service] {
		m.unlisten(service)
	}
	m.lock.Unlock()
	if l != nil {
		l.OnServiceEnd(service, err)
	}
}

func (m *manager) OnServiceState(service service.Service, state service.State) {
	m.lock.Lock()
	l, ok := m.listenersState[service]
	if !ok {
		l = m.defaultListener
	}
	m.lock.Unlock()
	if l != nil {
		l.OnServiceState(service, state)
	}
}
