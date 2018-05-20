package servicetest

import (
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/servicemgr"
)

var _ service.Runner = &globalProxy{}

// globalProxy allows the global servicemgr functions to pretend they're a
// service.Runner.
type globalProxy struct{}

func (g *globalProxy) State(s service.Service) service.State {
	return servicemgr.State(s)
}

func (g *globalProxy) StartWait(timeout time.Duration, svc service.Service) error {
	return servicemgr.StartWait(timeout, svc)
}

func (g *globalProxy) Start(svc service.Service, ready service.ReadySignal) error {
	return servicemgr.Start(svc, ready)
}

func (g *globalProxy) Halt(timeout time.Duration, svc service.Service) error {
	return servicemgr.Halt(timeout, svc)
}

func (g *globalProxy) Shutdown(timeout time.Duration, errlimit int) (n int, err error) {
	return servicemgr.Shutdown(timeout)
}

func (g *globalProxy) Services(state service.StateQuery, limit int) []service.Service {
	return servicemgr.Services(state, limit)
}

func (g *globalProxy) Register(svc service.Service) service.Runner {
	_ = servicemgr.Register(svc)
	return g
}

func (g *globalProxy) Unregister(svc service.Service) (deferred bool, err error) {
	return servicemgr.Unregister(svc)
}
