package service

import (
	"fmt"
	"sync"
)

var _ WaitGroup = &sync.WaitGroup{}

type WaitGroup interface {
	Add(int)
	Done()
	Wait()
}

func NewWaitGroup() WaitGroup {
	return NewCondGroup()
}

// CondGroup implements a less volatile, more general-purpose waitgroup than
// sync.WaitGroup.
//
// Unlike sync.WaitGroup, new Add calls can occur before all previous waits
// have returned.
type CondGroup struct {
	count int
	lock  sync.Mutex
	cond  *sync.Cond
}

func NewCondGroup() *CondGroup {
	wg := &CondGroup{}
	wg.cond = sync.NewCond(&wg.lock)
	return wg
}

func (wg *CondGroup) Stop() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count = 0
	wg.cond.Broadcast()
}

func (wg *CondGroup) Count() int {
	wg.lock.Lock()
	defer wg.lock.Unlock()
	return wg.count
}

func (wg *CondGroup) Done() { wg.Add(-1) }

func (wg *CondGroup) Add(n int) {
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

func (wg *CondGroup) Wait() {
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
