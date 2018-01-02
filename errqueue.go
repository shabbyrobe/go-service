package service

import (
	"fmt"
	"sync"
)

type errQueue struct {
	count int
	errs  []error
	lock  sync.Mutex
	cond  *sync.Cond
}

func newErrQueue() *errQueue {
	wg := &errQueue{}
	wg.cond = sync.NewCond(&wg.lock)
	return wg
}

func (wg *errQueue) Stop() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count = 0
	wg.errs = nil
	wg.cond.Broadcast()
}

func (wg *errQueue) Count() int {
	wg.lock.Lock()
	defer wg.lock.Unlock()
	return wg.count
}

func (wg *errQueue) Add(n int) {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count += n
	if wg.count < 0 {
		panic(fmt.Errorf("negative waitgroup counter: %d", wg.count))
	}
}

func (wg *errQueue) Put(err error) (ok bool) {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	if wg.count == 0 {
		return false
	}

	wg.count--
	if err != nil {
		wg.errs = append(wg.errs, err)
	}

	if wg.count == 0 {
		wg.cond.Broadcast()
	}
	return true
}

func (wg *errQueue) Wait() []error {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	for {
		if wg.count > 0 {
			wg.cond.Wait()
		} else {
			errs := wg.errs
			wg.errs = nil
			return errs
		}
	}
}
