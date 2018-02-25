package service

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func TestSingleReadySignalCancel(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	srs.Cancel()
	err := <-srs.Ready()
	tt.MustAssert(IsErrSignalCancelled(err))

	// subsequent calls to Ready should not block
	err = <-srs.Ready()
	tt.MustOK(err)
}

func TestSingleReadySignalCancelUnblocks(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)

	out := make(chan error, 1)
	go func() {
		out <- <-srs.Ready()
	}()

	srs.Cancel()
	tt.MustAssert(IsErrSignalCancelled(<-out))
	tt.MustOK(<-srs.Ready())
}

func TestSingleReadySignalPutNil(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	tt.MustAssert(srs.Done(nil))
	tt.MustOK(<-srs.Ready())
	tt.MustOK(<-srs.Ready())

	// Make sure we can't call Done again after
	tt.MustAssert(!srs.Done(nil))

	// Subsequent calls to Ready() should immediately yield
	tt.MustAssert(!srs.Done(errors.New("yep")))
}

func TestSingleReadySignalPutError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	err := errors.New("yep")
	tt.MustAssert(srs.Done(err))
	tt.MustEqual(err, <-srs.Ready())
	tt.MustOK(<-srs.Ready())

	// Subsequent calls to Ready() should immediately yield
	tt.MustOK(<-srs.Ready())

	// Make sure we can't call Done again after
	tt.MustAssert(!srs.Done(nil))
	tt.MustAssert(!srs.Done(err))
}

func TestSingleReadySignalWhenReadyTimeout(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	err := WhenReady(tscale, srs)
	tt.MustAssert(IsErrWaitTimeout(err))

	tt.MustEqual(signalCancelled, atomic.LoadInt32(&srs.state))
	tt.MustEqual(int32(1), atomic.LoadInt32(&srs.readyCalled))

	// The signal should be cancelled, so receiving on Ready should return
	// immediately.
	tt.MustOK(<-srs.Ready())
}

func TestSingleReadySignalWhenReadyError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	err := errors.New("yep")
	srs.Done(err)
	tt.MustEqual(err, WhenReady(tscale, srs))

	tt.MustOK(<-srs.Ready())
}

func TestSingleReadySignalWhenReady(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewReadySignal().(*singleReadySignal)
	srs.Done(nil)
	tt.MustOK(WhenReady(tscale, srs))

	tt.MustOK(<-srs.Ready())
}

func TestMultiReadySignal(t *testing.T) {
	tt := assert.WrapTB(t)

	mrs := NewMultiReadySignal(1)
	mrs.Done(nil)

	// should be able to increase count after it has been decreased to zero
	mrs.Add(1)
	mrs.Done(nil)

	// many adds:
	mrs.Add(2)
	mrs.Done(nil)
	mrs.Done(nil)

	tt.MustOK(<-mrs.Ready())

	// subsequent calls to Ready should not block
	tt.MustOK(<-mrs.Ready())

	// calls to Add() after Ready() has yielded should panic
	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				tt.MustAssert(IsErrUsedDoneSignal(err), err)
			} else {
				panic(r)
			}
		}
	}()
	mrs.Add(1)
}

func TestMultiReadySignalCancelUnblocks(t *testing.T) {
	tt := assert.WrapTB(t)

	mrs := NewMultiReadySignal(1)

	out := make(chan error, 1)
	go func() {
		out <- <-mrs.Ready()
	}()

	mrs.Cancel()
	tt.MustAssert(IsErrSignalCancelled(<-out))
	tt.MustOK(<-mrs.Ready())
}
