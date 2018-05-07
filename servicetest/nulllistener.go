package servicetest

import service "github.com/shabbyrobe/go-service"

type NullListener struct{}

var _ service.ListenerFull = &NullListener{}

func NewNullListener() *NullListener {
	return &NullListener{}
}

func (t *NullListener) OnServiceState(service service.Service, state service.State) {}

func (t *NullListener) OnServiceError(service service.Service, err service.Error) {}

func (t *NullListener) OnServiceEnd(stage service.Stage, service service.Service, err service.Error) {}
