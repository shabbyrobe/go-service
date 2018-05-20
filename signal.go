package service

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// closed will return a permanently closed channel. It allows <-Ready()
// to be called multiple times for a completed signal that can't be re-used.
var closed = make(chan error)

var errSignalCancelled = errors.New("service: ready cancelled")
var errUsedDoneSignal = errors.New("service: tried to add to a done signal")

func IsErrSignalCancelled(err error) bool { return err == errSignalCancelled }
func IsErrUsedDoneSignal(err error) bool  { return err == errUsedDoneSignal }

func init() {
	close(closed)
}

type ReadySignal interface {
	Done(err error) (ok bool)
	Cancel()

	// Ready returns a channel that yields a nil when the service is ready,
	// or an error when the service has failed to become ready. Use WhenReady
	// to wait for this with a timeout.
	Ready() <-chan error
}

// MultiReadySignal allows you to wait for multiple services to Start()
// concurrently.
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
type MultiReadySignal interface {
	ReadySignal

	// You may not call Add() on a MultiReadySignal that has yielded to its
	// Ready() channel. This will cause a panic.
	Add(int)

	Start(Runner, Service) error
}

func NewReadySignal() ReadySignal {
	return &singleReadySignal{
		c: make(chan error, 1),
	}
}

func NewMultiReadySignal(expected int) MultiReadySignal {
	ms := &multiReadySignal{
		expected: expected,
	}
	ms.cond = sync.NewCond(&ms.lock)
	return ms
}

func WhenReady(timeout time.Duration, r ReadySignal) error {
	errc := make(chan error, 1)
	go func() {
		errc <- <-r.Ready()
	}()

	var after <-chan time.Time
	if timeout > 0 {
		after = time.After(timeout)
	}
	select {
	case err := <-errc:
		return err
	case <-after:
		r.Cancel()
		return errWaitTimeout(0)
	}
}

type multiReadySignal struct {
	expected int
	done     bool
	buffer   []error
	lock     sync.Mutex
	cond     *sync.Cond
}

func (ms *multiReadySignal) Start(r Runner, s Service) error {
	ms.Add(1)
	return r.Start(s, ms)
}

func (ms *multiReadySignal) Cancel() {
	ms.lock.Lock()
	defer ms.lock.Unlock()
	if ms.done {
		return
	}
	ms.expected = 0
	ms.buffer = []error{errSignalCancelled}
	ms.cond.Broadcast()
}

func (ms *multiReadySignal) Add(n int) {
	ms.lock.Lock()
	defer ms.lock.Unlock()
	if ms.done {
		panic(errUsedDoneSignal)
	}
	if n <= 0 {
		panic("service: Add must be > 0")
	}
	ms.expected += n
}

func (ms *multiReadySignal) Done(err error) (ok bool) {
	ms.lock.Lock()
	defer ms.lock.Unlock()
	if ms.done {
		return false
	}

	ms.expected--
	if ms.expected < 0 {
		panic("negative expected count")
	}

	ms.buffer = append(ms.buffer, err)
	ms.cond.Broadcast()
	return true
}

func (ms *multiReadySignal) Ready() <-chan error {
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

const (
	signalUnused    int32 = 0
	signalCancelled int32 = 1
	signalDone      int32 = 2
)

type singleReadySignal struct {
	c           chan error
	state       int32
	readyCalled int32
}

func (ss *singleReadySignal) Cancel() {
	if atomic.CompareAndSwapInt32(&ss.state, signalUnused, signalCancelled) {
		ss.c <- errSignalCancelled
	}
}

func (ss *singleReadySignal) Done(err error) (ok bool) {
	if !atomic.CompareAndSwapInt32(&ss.state, signalUnused, signalDone) {
		return false
	}
	ss.c <- err
	return true
}

func (ss *singleReadySignal) Ready() <-chan error {
	if !atomic.CompareAndSwapInt32(&ss.readyCalled, 0, 1) {
		return closed
	}
	return ss.c
}
