package servicetest

import service "github.com/shabbyrobe/go-service"

type NullListenerFull struct{}

var _ service.ListenerFull = &NullListenerFull{}

func NewNullListenerFull() *NullListenerFull {
	return &NullListenerFull{}
}

func (t *NullListenerFull) OnServiceState(service service.Service, from, to service.State) {}

func (t *NullListenerFull) OnServiceError(service service.Service, err service.Error) {}

func (t *NullListenerFull) OnServiceEnd(stage service.Stage, service service.Service, err service.Error) {
}
