package servicetest

import (
	"fmt"
	"sync"
)

// condGroup implements a less volatile, more general-purpose waitgroup than
// sync.WaitGroup.
//
// Unlike sync.WaitGroup, new Add calls can occur before all previous waits
// have returned.
//
// This is copy-pastad in from golib, don't export it.
//
type condGroup struct {
	count int
	lock  sync.Mutex
	cond  *sync.Cond
}

func newCondGroup() *condGroup {
	wg := &condGroup{}
	wg.cond = sync.NewCond(&wg.lock)
	return wg
}

func (wg *condGroup) Stop() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count = 0
	wg.cond.Broadcast()
}

func (wg *condGroup) Count() int {
	wg.lock.Lock()
	defer wg.lock.Unlock()
	return wg.count
}

func (wg *condGroup) Done() { wg.Add(-1) }

func (wg *condGroup) Add(n int) {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count += n
	if wg.count < 0 {
		panic(fmt.Errorf("negative waitgroup counter: %d", wg.count))
	}
	if wg.count == 0 {
		wg.cond.Broadcast()
	}
}

func (wg *condGroup) Wait() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	for {
		if wg.count > 0 {
			wg.cond.Wait()
		} else {
			return
		}
	}
}
