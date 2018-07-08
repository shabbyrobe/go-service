package service

import (
	"context"
	"sync"
)

var (
	globalRunner *runner
	lock         sync.RWMutex
)

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
	lock.Unlock()
}
