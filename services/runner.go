package services

import (
	"context"

	service "github.com/shabbyrobe/go-service"
)

type ServiceInfo = service.ServiceInfo

func Start(ctx context.Context, services ...*Service) error {
	return Runner().Start(ctx, services...)
}

func Halt(ctx context.Context, services ...*Service) error {
	return Runner().Halt(ctx, services...)
}

func Shutdown(ctx context.Context) (err error) {
	return Runner().Shutdown(ctx)
}

func RunnerState() service.RunnerState {
	return Runner().RunnerState()
}

func State(svc *Service) service.State {
	return Runner().State(svc)
}

func Services(state service.State, limit int, into []ServiceInfo) []ServiceInfo {
	return Runner().Services(state, limit, into)
}
