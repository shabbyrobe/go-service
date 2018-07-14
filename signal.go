package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var errSignalCancelled = errors.New("service: ready cancelled")
var errUsedDoneSignal = errors.New("service: tried to add to a done signal")

func IsErrSignalCancelled(err error) bool { return err == errSignalCancelled }
func IsErrUsedDoneSignal(err error) bool  { return err == errUsedDoneSignal }

type Signal interface {
	Done(err error) (ok bool)
	Waiter() <-chan error
}

// MultiSignal coalesces multiple signals into a single error yielded to
// the Waiter().
//
// Much like a sync.WaitGroup, every call to Done() must be preceded by a call
// to Add(1). Done() decrements the internal counter, which must not go below
// zero. If the counter is at zero or goes down to zero after Ready() is called,
// the channel returned by Ready() will yield.
//
// The counter may go down to zero and be incremented again, but only if this
// happens before Ready() is called.
//
// Be careful calling Add() after calling Ready(). Probably just don't.
//
type MultiSignal interface {
	Signal

	// You may not call Add() on a MultiSignal that has yielded to its Ready()
	// channel. This will cause a panic.
	Add(int)
}

func AwaitSignal(context context.Context, r Signal) error {
	select {
	case err := <-r.Waiter():
		return err
	case <-context.Done():
		return context.Err()
	}
}

func AwaitSignalTimeout(timeout time.Duration, signal Signal) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return AwaitSignal(ctx, signal)
}

func NewSignal(expected int) Signal {
	switch expected {
	case 0:
		var signal *signal
		return signal
	case 1:
		return &signal{
			c: make(chan error, 1),
		}
	default:
		return NewMultiSignal(expected)
	}
}

type signal struct {
	c         chan error
	signalled int32
}

func (ss *signal) Done(err error) (ok bool) {
	if ss == nil {
		return false
	}
	if !atomic.CompareAndSwapInt32(&ss.signalled, 0, 1) {
		return false
	}
	ss.c <- err
	return true
}

func (ss *signal) Waiter() <-chan error {
	return ss.c
}

type multiSignal struct {
	expected int
	done     bool
	buffer   []error
	lock     sync.Mutex
	cond     *sync.Cond
}

func NewMultiSignal(expected int) MultiSignal {
	ms := &multiSignal{
		expected: expected,
	}
	ms.cond = sync.NewCond(&ms.lock)
	return ms
}

func (ms *multiSignal) Cancel() {
	ms.lock.Lock()
	if ms.done {
		ms.lock.Unlock()
		return
	}
	ms.expected = 0
	ms.buffer = []error{errSignalCancelled}
	ms.cond.Broadcast()
	ms.lock.Unlock()
}

func (ms *multiSignal) Add(n int) {
	ms.lock.Lock()
	if ms.done {
		ms.lock.Unlock()
		panic(errUsedDoneSignal)
	}
	if n <= 0 {
		ms.lock.Unlock()
		panic("service: Add must be > 0")
	}
	ms.expected += n
	ms.lock.Unlock()
}

func (ms *multiSignal) Done(err error) (ok bool) {
	ms.lock.Lock()
	if ms.done {
		ms.lock.Unlock()
		return false
	}

	ms.expected--
	if ms.expected < 0 {
		ms.lock.Unlock()
		panic("negative expected count")
	}

	ms.buffer = append(ms.buffer, err)
	ms.cond.Broadcast()
	ms.lock.Unlock()
	return true
}

func (ms *multiSignal) Waiter() <-chan error {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	if ms.done {
		return closed
	}

	c := make(chan error, 1)
	go func() {
		ms.lock.Lock()
		for ms.expected > 0 {
			ms.cond.Wait()
		}
		ms.done = true
		out := ms.buffer
		ms.buffer = nil
		ms.lock.Unlock()

		errs := out[:0]
		for _, v := range out {
			if v != nil {
				errs = append(errs, v)
			}
		}

		var ret error
		if len(errs) == 1 {
			ret = errs[0]
		} else if len(errs) > 1 {
			ret = &serviceErrors{errors: errs}
		}
		c <- ret
	}()

	return c
}
