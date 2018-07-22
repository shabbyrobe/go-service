package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

// TODO:
// - NewSignal(0)
// - NewMultiSignal(2)

func assertWaiterEmpty(tt assert.T, w <-chan error) {
	tt.Helper()
	select {
	case v := <-w:
		tt.Error("expected no receive, found", v)
	default:
	}
}

func TestSignalPutNil(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal(1).(*signal)
	tt.MustAssert(srs.Done(nil))
	tt.MustOK(<-srs.Waiter())
	assertWaiterEmpty(tt, srs.Waiter())

	// Subsequent calls to 'done' should not push anything into the channel:
	tt.MustAssert(!srs.Done(nil))
	assertWaiterEmpty(tt, srs.Waiter())
}

func TestSignalPutError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal(1).(*signal)
	err := errors.New("yep")
	tt.MustAssert(srs.Done(err))
	tt.MustEqual(err, <-srs.Waiter())
	assertWaiterEmpty(tt, srs.Waiter())

	// Subsequent calls to 'done' should not push anything into the channel:
	tt.MustAssert(!srs.Done(nil))
	tt.MustAssert(!srs.Done(err))
	assertWaiterEmpty(tt, srs.Waiter())
}

func TestSignalAwaitTimeout(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal(1).(*signal)

	err := AwaitSignalTimeout(tscale, srs)
	tt.MustEqual(err, context.DeadlineExceeded)

	tt.MustEqual(int32(0), atomic.LoadInt32(&srs.signalled))

	assertWaiterEmpty(tt, srs.Waiter())
}

func TestSignalAwaitError(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal(1).(*signal)
	err := errors.New("yep")
	tt.MustAssert(srs.Done(err))

	tt.MustEqual(err, AwaitSignalTimeout(tscale, srs))
	assertWaiterEmpty(tt, srs.Waiter())
}

func TestSignalAwaitNil(t *testing.T) {
	tt := assert.WrapTB(t)

	srs := NewSignal(1).(*signal)
	tt.MustAssert(srs.Done(nil))
	tt.MustOK(AwaitSignalTimeout(tscale, srs))
	assertWaiterEmpty(tt, srs.Waiter())
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
