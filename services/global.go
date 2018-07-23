package services

import (
	"context"
	"sync"

	service "github.com/shabbyrobe/go-service"
)

type Service = service.Service

var (
	globalRunner  service.Runner
	globalOnEnd   service.OnEnd
	globalOnError service.OnError
	globalMu      sync.RWMutex
)

func globalOnEndFn(stage service.Stage, service *service.Service, err error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalOnEnd != nil {
		globalOnEnd(stage, service, err)
	}
}

func globalOnErrorFn(stage service.Stage, service *service.Service, err error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalOnError != nil {
		globalOnError(stage, service, err)
	}
}

func Runner() (r service.Runner) {
	globalMu.RLock()
	r = globalRunner
	globalMu.RUnlock()
	return r
}

func OnEnd(cb service.OnEnd) (old service.OnEnd) {
	globalMu.Lock()
	old = globalOnEnd
	globalOnEnd = cb
	globalMu.Unlock()
	return old
}

func OnError(cb service.OnError) (old service.OnError) {
	globalMu.Lock()
	old = globalOnError
	globalOnError = cb
	globalMu.Unlock()
	return old
}

// GlobalReset is used to replace the global runner with a fresh one. The
// existing runner will be Shutdown() before it is replaced. If the existing
// runner could not be Shutdown, it will panic.
//
// ctx may be nil.
func Reset(ctx context.Context) {
	globalMu.Lock()
	defer globalMu.Unlock()

	if globalRunner != nil {
		if err := globalRunner.Shutdown(ctx); err != nil {
			panic(err)
		}
	}
	globalRunner = service.NewRunner(
		service.RunnerOnEnd(globalOnEndFn),
		service.RunnerOnError(globalOnErrorFn),
	)
}
