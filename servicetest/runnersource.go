package servicetest

import service "github.com/shabbyrobe/go-service"

type Listener interface {
	OnServiceEnd() service.OnEnd
	OnServiceError() service.OnError
}

type RunnerSource interface {
	CanHaltRunner() bool
	CanCreateRunner() bool

	FirstRunner(l Listener) service.Runner
	CreateRunner(l Listener) service.Runner

	End()
}

type ServiceRunnerSource struct{}

func (srs *ServiceRunnerSource) CanCreateRunner() bool { return true }
func (srs *ServiceRunnerSource) CanHaltRunner() bool   { return true }

func (srs *ServiceRunnerSource) FirstRunner(l Listener) service.Runner {
	return srs.newRunner(l)
}

func (srs *ServiceRunnerSource) CreateRunner(l Listener) service.Runner {
	return srs.newRunner(l)
}

func (srs *ServiceRunnerSource) newRunner(l Listener) service.Runner {
	var opts []service.RunnerOption
	onEnd, onError := l.OnServiceEnd(), l.OnServiceError()

	if onEnd != nil {
		opts = append(opts, service.RunnerOnEnd(onEnd))
	}
	if onError != nil {
		opts = append(opts, service.RunnerOnError(onError))
	}
	return service.NewRunner(opts...)
}

func (srs *ServiceRunnerSource) End() {}
