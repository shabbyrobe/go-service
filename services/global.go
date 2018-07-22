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

func OnEnd(cb service.OnEnd) {
	globalMu.Lock()
	globalOnEnd = cb
	globalMu.Unlock()
}

func OnError(cb service.OnError) {
	globalMu.Lock()
	globalOnError = cb
	globalMu.Unlock()
}

// GlobalReset is used to replace the global runner with a fresh one. The
// existing runner will be Shutdown() before it is replaced. If the existing
// runner could not be Shutdown, it will panic.
//
// ctx may be nil.
func Reset(ctx context.Context) {
	globalMu.Lock()
	if globalRunner != nil {
		if err := globalRunner.Shutdown(ctx); err != nil {
			globalMu.Unlock()
			panic(err)
		}
	}
	globalRunner = service.NewRunner(
		service.RunnerOnEnd(globalOnEndFn),
		service.RunnerOnError(globalOnErrorFn),
	)
	globalMu.Unlock()
}
