package servicemgr

import (
	"sync"

	service "github.com/shabbyrobe/go-service"
)

type Listener interface {
	OnServiceEnd(service service.Service, err service.Error)
}

type NonHaltingErrorListener interface {
	Listener
	OnServiceError(service service.Service, err service.Error)
}

type StateListener interface {
	Listener
	OnServiceState(service service.Service, state service.State)
}

type listenerDispatcher struct {
	listeners        map[service.Service]Listener
	listenersNHError map[service.Service]NonHaltingErrorListener
	listenersState   map[service.Service]StateListener

	defaultListener service.Listener
	lock            sync.Mutex
}

func newListenerDispatcher() *listenerDispatcher {
	return &listenerDispatcher{
		listeners:        make(map[service.Service]Listener),
		listenersNHError: make(map[service.Service]NonHaltingErrorListener),
		listenersState:   make(map[service.Service]StateListener),
	}
}

func (g *listenerDispatcher) SetDefaultListener(l service.Listener) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.defaultListener = l
}

func (g *listenerDispatcher) Add(service service.Service, l Listener) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.listeners[service] = l
	if el, ok := l.(NonHaltingErrorListener); ok {
		g.listenersNHError[service] = el
	}
	if sl, ok := l.(StateListener); ok {
		g.listenersState[service] = sl
	}
}

func (g *listenerDispatcher) Remove(service service.Service) {
	g.lock.Lock()
	defer g.lock.Unlock()
	delete(g.listeners, service)
	delete(g.listenersNHError, service)
	delete(g.listenersState, service)
}

func (g *listenerDispatcher) OnServiceError(service service.Service, err service.Error) {
	g.lock.Lock()
	l, ok := g.listenersNHError[service]
	if !ok {
		l = g.defaultListener
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceError(service, err)
	}
}

func (g *listenerDispatcher) OnServiceEnd(service service.Service, err service.Error) {
	g.lock.Lock()
	l, ok := g.listeners[service]
	if !ok {
		l = g.defaultListener
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceEnd(service, err)
	}
}

func (g *listenerDispatcher) OnServiceState(service service.Service, state service.State) {
	g.lock.Lock()
	l, ok := g.listenersState[service]
	if !ok {
		l = g.defaultListener
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceState(service, state)
	}
}
