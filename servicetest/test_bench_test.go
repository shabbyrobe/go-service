package servicetest

import (
	"sync"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

func BenchmarkRunnerStart1(b *testing.B) {
	benchmarkRunnerStartN(b, 1)
}

func BenchmarkGoroutineStart1(b *testing.B) {
	benchmarkGoroutineStartN(b, 1)
}

func BenchmarkRunnerStart10(b *testing.B) {
	benchmarkRunnerStartN(b, 10)
}

func BenchmarkGoroutineStart10(b *testing.B) {
	benchmarkGoroutineStartN(b, 10)
}

func benchmarkRunnerStartN(b *testing.B, n int) {
	tt := assert.WrapTB(b)
	r := service.NewRunner(NewNullListener())

	svcs := make([]service.Service, n)
	for i := 0; i < n; i++ {
		svcs[i] = (&BlockingService{}).Init()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StartTimer()
		for j := 0; j < n; j++ {
			_ = r.Start(svcs[j], nil)
		}
		b.StopTimer()
		tt.MustOK(r.HaltAll(1*time.Second, 0))
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
