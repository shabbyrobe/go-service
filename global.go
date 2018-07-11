package service

import (
	"context"
	"sync"
)

var (
	globalRunner  *runner
	globalOnEnd   OnEnd
	globalOnError OnError
	globalMu      sync.RWMutex
)

func globalOnEndFn(stage Stage, service *Service, err error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalOnEnd != nil {
		globalOnEnd(stage, service, err)
	}
}

func globalOnErrorFn(stage Stage, service *Service, err error) {
	globalMu.RLock()
	defer globalMu.RUnlock()
	if globalOnError != nil {
		globalOnError(stage, service, err)
	}
}

func GlobalRunner() (r Runner) {
	globalMu.RLock()
	r = globalRunner
	globalMu.RUnlock()
	return r
}

func GlobalRunnerOnEnd(cb OnEnd) {
	globalRunner.mu.Lock()
	globalRunner.onEnd = cb
	globalRunner.mu.Unlock()
}

func GlobalRunnerOnError(cb OnError) {
	globalRunner.mu.Lock()
	globalRunner.onError = cb
	globalRunner.mu.Unlock()
}

// GlobalReset is used to replace the global runner with a fresh one. The
// existing runner will be Shutdown() before it is repolaced. If the existing
// runner could not be Shutdown, it will panic.
//
// context may be nil.
func GlobalReset(ctx context.Context) {
	globalMu.Lock()
	if globalRunner != nil {
		if err := globalRunner.Shutdown(ctx); err != nil {
			globalMu.Unlock()
			panic(err)
		}
	}

	globalRunner = NewRunner().(*runner)
	globalOnEnd = nil
	globalOnError = nil
	globalMu.Unlock()
}
