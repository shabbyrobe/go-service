package servicemgr

import (
	"sync"

	service "github.com/shabbyrobe/go-service"
)

type listenerDispatcher struct {
	listeners      map[service.Service]service.Listener
	listenersError map[service.Service]service.ErrorListener
	listenersState map[service.Service]service.StateListener
	retained       map[service.Service]bool

	defaultListener      service.Listener
	defaultErrorListener service.ErrorListener
	defaultStateListener service.StateListener

	lock sync.Mutex
}

func newListenerDispatcher() *listenerDispatcher {
	return &listenerDispatcher{
		listeners:      make(map[service.Service]service.Listener),
		listenersError: make(map[service.Service]service.ErrorListener),
		listenersState: make(map[service.Service]service.StateListener),
		retained:       make(map[service.Service]bool),
	}
}

func (g *listenerDispatcher) SetDefaultListener(l service.Listener) {
	g.lock.Lock()
	defer g.lock.Unlock()

	g.defaultListener = l
	g.defaultErrorListener, _ = l.(service.ErrorListener)
	g.defaultStateListener, _ = l.(service.StateListener)
}

func (g *listenerDispatcher) Add(svc service.Service, l service.Listener) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.listeners[svc] = l
	if el, ok := l.(service.ErrorListener); ok {
		g.listenersError[svc] = el
	}
	if sl, ok := l.(service.StateListener); ok {
		g.listenersState[svc] = sl
	}
}

func (g *listenerDispatcher) Register(service service.Service) {
	g.lock.Lock()
	g.retained[service] = true
	g.lock.Unlock()
}

func (g *listenerDispatcher) Unregister(service service.Service, deferred bool) {
	g.lock.Lock()
	if deferred {
		delete(g.retained, service)
	} else {
		g.remove(service)
	}
	g.lock.Unlock()
}

func (g *listenerDispatcher) Remove(service service.Service) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.remove(service)
}

// remove expects g.loc is acquired
func (g *listenerDispatcher) remove(service service.Service) {
	delete(g.listeners, service)
	delete(g.listenersError, service)
	delete(g.listenersState, service)
	delete(g.retained, service)
}

func (g *listenerDispatcher) OnServiceEnd(stage service.Stage, service service.Service, err service.Error) {
	g.lock.Lock()
	l, ok := g.listeners[service]
	if !ok {
		l = g.defaultListener
	}
	if !g.retained[service] {
		g.remove(service)
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceEnd(stage, service, err)
	}
}

func (g *listenerDispatcher) OnServiceError(svc service.Service, err service.Error) {
	g.lock.Lock()
	l, ok := g.listenersError[svc]
	if !ok {
		l = g.defaultErrorListener
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceError(svc, err)
	}
}

func (g *listenerDispatcher) OnServiceState(service service.Service, state service.State) {
	g.lock.Lock()
	l, ok := g.listenersState[service]
	if !ok {
		l = g.defaultStateListener
	}
	g.lock.Unlock()
	if l != nil {
		l.OnServiceState(service, state)
	}
}
