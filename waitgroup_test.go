package service

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

func TestWaitGroup(t *testing.T) {
	tt := assert.WrapTB(t)

	for i := 0; i < 100; i++ {
		wg := NewWaitGroup()
		var done int32
		var n int32 = 100
		for j := 0; j < int(n); j++ {
			wg.Add(1)
			go func() {
				atomic.AddInt32(&done, 1)
				wg.Done()
			}()
		}
		wg.Wait()
		tt.MustEqual(n, atomic.LoadInt32(&done))
	}
}

func TestWaitGroupRecycle(t *testing.T) {
	tt := assert.WrapTB(t)

	wg := NewWaitGroup()
	for i := 0; i < 100; i++ {
		var done int32
		var n int32 = 100
		for j := 0; j < int(n); j++ {
			wg.Add(1)
			go func() {
				atomic.AddInt32(&done, 1)
				wg.Done()
			}()
		}
		wg.Wait()
		tt.MustEqual(n, atomic.LoadInt32(&done))
	}
}

func TestWaitGroupIncreaseWhileWaiting(t *testing.T) {
	wg := NewWaitGroup()
	wg.Add(1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		wg.Add(1)
		wg.Done()
		wg.Done()
	}()
	wg.Wait()
}

func TestWaitGroupWaitOnNothing(t *testing.T) {
	tt := assert.WrapTB(t)

	wg := NewWaitGroup()
	tm := time.Now()
	wg.Wait()
	tt.MustAssert(time.Since(tm) < time.Millisecond)
}

func TestWaitGroupMultipleWaiters(t *testing.T) {
	tt := assert.WrapTB(t)

	wg := NewWaitGroup()
	for i := 0; i < 100; i++ {
		var done int32
		var n int32 = 1000
		for j := 0; j < int(n); j++ {
			wg.Add(1)
			go func() {
				atomic.AddInt32(&done, 1)
				wg.Done()
			}()
		}
		for k := 0; k < 50; k++ {
			go wg.Wait()
		}

		wg.Wait()
		tt.MustEqual(n, atomic.LoadInt32(&done))
	}
}
