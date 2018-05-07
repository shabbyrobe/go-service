package servicetest

import (
	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/servicemgr"
)

type RunnerSource interface {
	CanHaltRunner() bool
	CanCreateRunner() bool

	FirstRunner(l service.Listener) service.Runner
	CreateRunner(l service.Listener) service.Runner

	End()
}

type ServiceRunnerSource struct{}

func (srs *ServiceRunnerSource) CanCreateRunner() bool { return true }
func (srs *ServiceRunnerSource) CanHaltRunner() bool   { return true }

func (srs *ServiceRunnerSource) FirstRunner(l service.Listener) service.Runner {
	return service.NewRunner(l)
}

func (srs *ServiceRunnerSource) CreateRunner(l service.Listener) service.Runner {
	return service.NewRunner(l)
}

func (srs *ServiceRunnerSource) End() {}

type ServiceManagerRunnerSource struct{}

func (srs *ServiceManagerRunnerSource) CanCreateRunner() bool { return false }
func (srs *ServiceManagerRunnerSource) CanHaltRunner() bool   { return false }

func (srs *ServiceManagerRunnerSource) FirstRunner(l service.Listener) service.Runner {
	servicemgr.Reset()
	servicemgr.DefaultListener(l)
	return &globalProxy{}
}

func (srs *ServiceManagerRunnerSource) CreateRunner(l service.Listener) service.Runner {
	panic("can not create runner with this source")
}

func (srs *ServiceManagerRunnerSource) End() {
	servicemgr.Reset()
}
