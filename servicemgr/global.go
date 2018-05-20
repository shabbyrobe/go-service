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
func StartWaitListen(timeout time.Duration, l service.Listener, s service.Service) error {
	lock.RLock()
	defer lock.RUnlock()
	if l == nil {
		l, _ = s.(service.Listener)
	}
	if l != nil {
		listener.Add(s, l)
	}
	return service.StartWait(runner, timeout, s)
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
func StartListen(l service.Listener, s service.Service, rdy service.ReadySignal) error {
	lock.RLock()
	defer lock.RUnlock()
	if l == nil {
		l, _ = s.(service.Listener)
	}
	if l != nil {
		listener.Add(s, l)
	}
	return runner.Start(s, rdy)
}

func Start(s service.Service, rdy service.ReadySignal) error {
	return StartListen(nil, s, rdy)
}

// Services lists services in the global runner based on the criteria.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func Services(state service.StateQuery, limit int) []service.Service {
	lock.RLock()
	defer lock.RUnlock()

	return runner.Services(state, limit)
}

// Register registers a service from the global runner.
func Register(s service.Service) Starter {
	lock.Lock()
	defer lock.Unlock()
	runner.Register(s)
	listener.Register(s)
	return Starter{s}
}

// Unregister unregisters a service from the global runner.
func Unregister(s service.Service) (deferred bool, err error) {
	lock.Lock()
	defer lock.Unlock()

	deferred, err = runner.Unregister(s)
	listener.Unregister(s, deferred)
	return deferred, err
}

// Halt halts a service in the global runner.
//
// See github.com/shabbyrobe/go-service.Runner for more documentation.
func Halt(timeout time.Duration, s service.Service) error {
	lock.RLock()
	defer lock.RUnlock()

	return runner.Halt(timeout, s)
}

func EnsureHalt(timeout time.Duration, s service.Service) error {
	return service.EnsureHalt(Runner(), timeout, s)
}

func MustEnsureHalt(timeout time.Duration, s service.Service) {
	service.MustEnsureHalt(Runner(), timeout, s)
}

func EnsureHaltMany(timeout time.Duration, ss ...service.Service) (n int, rerr error) {
	var msgs []string
	r := Runner()
	for _, s := range ss {
		cerr := service.EnsureHalt(r, timeout, s)
		if cerr != nil {
			msgs = append(msgs, fmt.Sprintf("%s: %s", s.ServiceName(), cerr.Error()))
		} else {
			n++
		}
	}
	if len(msgs) > 0 {
		return n, fmt.Errorf("haltall failed:\n%s", strings.Join(msgs, "\n"))
	}
	return n, nil
}

func MustEnsureHaltMany(timeout time.Duration, ss ...service.Service) {
	if _, err := EnsureHaltMany(timeout, ss...); err != nil {
		panic(err)
	}
}

func Shutdown(timeout time.Duration) (n int, err error) {
	return EnsureHaltMany(timeout, Runner().Services(service.FindRunning, 0)...)
}

func MustShutdown(timeout time.Duration) {
	MustEnsureHaltMany(timeout, Runner().Services(service.FindRunning, 0)...)
}

func DeferEnsureHalt(into *error, timeout time.Duration, service service.Service) {
	cerr := EnsureHalt(timeout, service)
	if *into == nil && cerr != nil {
		*into = cerr
	}
}

func DeferHalt(into *error, timeout time.Duration, service service.Service) {
	cerr := Halt(timeout, service)
	if *into == nil && cerr != nil {
		*into = cerr
	}
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

func (s Starter) StartWaitListen(timeout time.Duration, l service.Listener) error {
	svc := s.Service
	s.Service = nil
	return StartWaitListen(timeout, l, svc)
}

func (s Starter) StartWait(timeout time.Duration) error {
	svc := s.Service
	s.Service = nil
	return StartWait(timeout, svc)
}

func (s Starter) StartListen(l service.Listener, rdy service.ReadySignal) error {
	svc := s.Service
	s.Service = nil
	return StartListen(l, svc, rdy)
}

func (s Starter) Start(rdy service.ReadySignal) error {
	svc := s.Service
	s.Service = nil
	return Start(svc, rdy)
}
