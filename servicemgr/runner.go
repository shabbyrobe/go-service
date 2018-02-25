package servicemgr

import (
	"fmt"
	"strings"
	"sync"
	"time"

	service "github.com/shabbyrobe/go-service"
)

var (
	listener        *listenerDispatcher
	defaultListener service.Listener
	runner          service.Runner
	lock            sync.RWMutex
)

func init() {
	Reset()
}

func Runner() service.Runner {
	lock.RLock()
	r := runner
	lock.RUnlock()
	return r
}

// Reset is used to replace the global runner with a fresh one. If you do not ensure
// all services are halted before calling Reset(), you will leak reasources.
func Reset() {
	lock.Lock()
	listener = newListenerDispatcher()
	listener.SetDefaultListener(defaultListener)
	runner = service.NewRunner(listener)
	lock.Unlock()
}

func DefaultListener(l service.Listener) {
	lock.Lock()
	defer lock.Unlock()
	defaultListener = l
	listener.SetDefaultListener(l)
}

func State(s service.Service) service.State {
	lock.RLock()
	defer lock.RUnlock()

	return runner.State(s)
}

// StartWait starts a service in the global runner.
//
// You may also provide an optional Listener which will allow the caller to
// respond to errors and service ends. If the listener argument is nil and the
// service itself implements Listener, it will be used.
//
// Listeners can be used multiple times when starting different services.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func StartWaitListen(timeout time.Duration, l Listener, s service.Service) error {
	lock.RLock()
	defer lock.RUnlock()
	if l == nil {
		l, _ = s.(Listener)
	}
	if l != nil {
		listener.Add(s, l)
	}
	return runner.StartWait(timeout, s)
}

func StartWait(timeout time.Duration, s service.Service) error {
	return StartWaitListen(timeout, nil, s)
}

// StartListen starts a service in the global runner.
//
// You may also provide an optional Listener (which may be the service itself),
// which will allow the caller to respond to errors and service ends.
//
// Listeners can be used multiple times when starting different services.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func StartListen(l Listener, s service.Service, rdy service.ReadySignal) error {
	lock.RLock()
	defer lock.RUnlock()
	if l == nil {
		l, _ = s.(Listener)
	}
	if l != nil {
		listener.Add(s, l)
	}
	return runner.Start(s, rdy)
}

func Start(s service.Service, rdy service.ReadySignal) error {
	return StartListen(nil, s, rdy)
}

// Halt halts a service in the global runner.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func Halt(timeout time.Duration, s service.Service) error {
	lock.RLock()
	defer lock.RUnlock()

	return runner.Halt(timeout, s)
}

// HaltAll halts all services in the global runner.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func HaltAll(timeout time.Duration, errlimit int) (n int, err error) {
	lock.RLock()
	defer lock.RUnlock()

	return runner.HaltAll(timeout, errlimit)
}

// Services lists services in the global runner based on the criteria.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func Services(state service.StateQuery) []service.Service {
	lock.RLock()
	defer lock.RUnlock()

	return runner.Services(state)
}

// Register registers a service from the global runner.
func Register(s service.Service) Starter {
	lock.RLock()
	defer lock.RUnlock()
	_ = runner.Register(s)
	return Starter{s}
}

// Unregister unregisters a service from the global runner.
func Unregister(s service.Service) error {
	lock.RLock()
	defer lock.RUnlock()

	listener.Remove(s)
	return runner.Unregister(s)
}

func EnsureHalt(timeout time.Duration, s service.Service) error {
	return service.EnsureHalt(Runner(), timeout, s)
}

func MustEnsureHalt(timeout time.Duration, s service.Service) {
	service.MustEnsureHalt(Runner(), timeout, s)
}

func EnsureHaltAll(timeout time.Duration, ss ...service.Service) error {
	var msgs []string
	r := Runner()
	for _, s := range ss {
		cerr := service.EnsureHalt(r, timeout, s)
		if cerr != nil {
			msgs = append(msgs, fmt.Sprintf("%s: %s", s.ServiceName(), cerr.Error()))
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("haltall failed:\n%s", strings.Join(msgs, "\n"))
	}
	return nil
}

func MustEnsureHaltAll(timeout time.Duration, ss ...service.Service) {
	if err := EnsureHaltAll(timeout, ss...); err != nil {
		panic(err)
	}
}

func MustShutdown(timeout time.Duration) {
	MustEnsureHaltAll(timeout, Runner().Services(service.FindRunning)...)
}

func getListener() *listenerDispatcher {
	lock.RLock()
	l := listener
	lock.RUnlock()
	return l
}

type Starter struct {
	service.Service
}

func (s Starter) StartWaitListen(timeout time.Duration, l Listener) error {
	svc := s.Service
	s.Service = nil
	return StartWaitListen(timeout, l, svc)
}

func (s Starter) StartWait(timeout time.Duration) error {
	svc := s.Service
	s.Service = nil
	return StartWait(timeout, svc)
}

func (s Starter) StartListen(l Listener, rdy service.ReadySignal) error {
	svc := s.Service
	s.Service = nil
	return StartListen(l, svc, rdy)
}

func (s Starter) Start(rdy service.ReadySignal) error {
	svc := s.Service
	s.Service = nil
	return Start(svc, rdy)
}
