package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func TestSignalPutNil(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal().(*signal)
	tt.MustAssert(srs.Done(nil))
	tt.MustOK(<-srs.Waiter())
	tt.MustOK(<-srs.Waiter())

	// Make sure we can't call Done again after
	tt.MustAssert(!srs.Done(nil))

	// Subsequent calls to Waiter() should immediately yield
	tt.MustAssert(!srs.Done(errors.New("yep")))
}

func TestSignalPutError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal().(*signal)
	err := errors.New("yep")
	tt.MustAssert(srs.Done(err))
	tt.MustEqual(err, <-srs.Waiter())
	tt.MustOK(<-srs.Waiter())

	// Subsequent calls to Waiter() should immediately yield
	tt.MustOK(<-srs.Waiter())

	// Make sure we can't call Done again after
	tt.MustAssert(!srs.Done(nil))
	tt.MustAssert(!srs.Done(err))
}

func TestSignalWhenWaiterTimeout(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal().(*signal)

	err := AwaitSignalTimeout(tscale, srs)
	tt.MustEqual(err, context.DeadlineExceeded)

	tt.MustEqual(int32(1), atomic.LoadInt32(&srs.signalled))

	// The signal should be cancelled, so receiving on Waiter should return
	// immediately.
	tt.MustOK(<-srs.Waiter())
}

func TestSignalWhenWaiterError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal().(*signal)
	err := errors.New("yep")
	srs.Done(err)

	tt.MustEqual(err, AwaitSignalTimeout(tscale, srs))

	tt.MustOK(<-srs.Waiter())
}

func TestSignalWhenWaiter(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal().(*signal)
	srs.Done(nil)
	tt.MustOK(AwaitSignalTimeout(tscale, srs))

	tt.MustOK(<-srs.Waiter())
}

func TestMultiWaiterSignal(t *testing.T) {
	tt := assert.WrapTB(t)

	mrs := NewMultiSignal(1)
	mrs.Done(nil)

	// should be able to increase count after it has been decreased to zero
	mrs.Add(1)
	mrs.Done(nil)

	// many adds:
	mrs.Add(2)
	mrs.Done(nil)
	mrs.Done(nil)

	tt.MustOK(<-mrs.Waiter())

	// subsequent calls to Waiter should not block
	tt.MustOK(<-mrs.Waiter())

	// calls to Add() after Waiter() has yielded should panic
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

func TestMultiWaiterSignalCancelUnblocks(t *testing.T) {
	tt := assert.WrapTB(t)

	mrs := NewMultiSignal(1)

	out := make(chan error, 1)
	go func() {
		out <- <-mrs.Waiter()
	}()

	mrs.(*multiSignal).Cancel()
	tt.MustAssert(IsErrSignalCancelled(<-out))
	tt.MustOK(<-mrs.Waiter())
}
