package servicemgr

import (
	"sync"

	service "github.com/shabbyrobe/go-service"
)

type listenerDispatcher struct {
	listeners map[service.Service]service.Listener
	lock      sync.Mutex
}

func newListenerDispatcher() *listenerDispatcher {
	return &listenerDispatcher{
		listeners: make(map[service.Service]service.Listener),
	}
}

func (g *listenerDispatcher) Add(service service.Service, l service.Listener) {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.listeners[service] = l
}

func (g *listenerDispatcher) Remove(service service.Service) {
	g.lock.Lock()
	defer g.lock.Unlock()
	delete(g.listeners, service)
}

func (g *listenerDispatcher) OnServiceError(service service.Service, err service.Error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if sl, ok := g.listeners[service]; ok {
		sl.OnServiceError(service, err)
	}
}

func (g *listenerDispatcher) OnServiceEnd(service service.Service, err service.Error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if sl, ok := g.listeners[service]; ok {
		sl.OnServiceEnd(service, err)
	}
}

func (g *listenerDispatcher) OnServiceState(service service.Service, state service.State) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if sl, ok := g.listeners[service]; ok {
		sl.OnServiceState(service, state)
	}
}
