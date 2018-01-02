package service

import (
	"sync"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

func BenchmarkRunnerStart10(b *testing.B) {
	benchmarkRunnerStartN(b, 10)
}

func BenchmarkGoroutineStart10(b *testing.B) {
	benchmarkGoroutineStartN(b, 10)
}

func benchmarkRunnerStartN(b *testing.B, n int) {
	b.StopTimer()
	b.ResetTimer()

	tt := assert.WrapTB(b)
	r := NewRunner(newDummyListener())

	svcs := make([]Service, n)
	for i := 0; i < n; i++ {
		svcs[i] = (&blockingService{}).Init()
	}

	for i := 0; i < b.N; i++ {
		b.StartTimer()
		for i := 0; i < n; i++ {
			_ = r.Start(svcs[i])
		}
		b.StopTimer()
		tt.MustOK(r.HaltAll(1 * time.Second))
	}
}

func benchmarkGoroutineStartN(b *testing.B, n int) {
	b.StopTimer()
	b.ResetTimer()

	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		stop := make(chan struct{})
		wg.Add(n)
		b.StartTimer()
		for i := 0; i < n; i++ {
			go func() {
				<-stop
				wg.Done()
			}()
		}
		b.StopTimer()
		close(stop)
		wg.Wait()
	}
}
