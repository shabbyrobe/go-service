package service

import (
	"context"
	"sync"
)

var (
	globalRunner  *runner
	globalOnEnd   OnEnd
	globalOnError OnError
	globalOnState OnState
	lock          sync.RWMutex
)

func globalOnEndFn(stage Stage, service *Service, err error) {
	lock.RLock()
	defer lock.RUnlock()
	if globalOnEnd != nil {
		globalOnEnd(stage, service, err)
	}
}

func globalOnErrorFn(stage Stage, service *Service, err error) {
	lock.RLock()
	defer lock.RUnlock()
	if globalOnError != nil {
		globalOnError(stage, service, err)
	}
}

func globalOnStateFn(service *Service, from, to State) {
	lock.RLock()
	defer lock.RUnlock()
	if globalOnState != nil {
		globalOnState(service, from, to)
	}
}

func GlobalRunner() (r Runner) {
	lock.RLock()
	r = globalRunner
	lock.RUnlock()
	return r
}

func GlobalRunnerOnEnd(cb OnEnd) {
	globalRunner.lock.Lock()
	globalRunner.onEnd = cb
	globalRunner.lock.Unlock()
}

func GlobalRunnerOnError(cb OnError) {
	globalRunner.lock.Lock()
	globalRunner.onError = cb
	globalRunner.lock.Unlock()
}

// GlobalReset is used to replace the global runner with a fresh one. The
// existing runner will be Shutdown() before it is repolaced. If the existing
// runner could not be Shutdown, it will panic.
//
// context may be nil.
func GlobalReset(ctx context.Context) {
	lock.Lock()
	if globalRunner != nil {
		if err := globalRunner.Shutdown(ctx); err != nil {
			lock.Unlock()
			panic(err)
		}
	}

	globalRunner = NewRunner().(*runner)
	globalOnEnd = nil
	globalOnError = nil
	globalOnState = nil
	lock.Unlock()
}
