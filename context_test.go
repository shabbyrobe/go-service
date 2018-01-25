package service

import (
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func TestContextStandalone(t *testing.T) {
	tt := assert.WrapTB(t)
	c := Standalone()
	s := (&blockingService{}).Init()

	end := make(chan struct{})
	go func() {
		defer close(end)
		tt.MustOK(s.Run(c))
	}()
	c.Halt()
	<-end
}

// Done needs to be exported so the compiler doesn't think it should be eliminated.
var TestingDone bool

func BenchmarkContextIsDoneNil(b *testing.B) {
	ctx := newContext(nil, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		TestingDone = ctx.IsDone()
	}
}

func BenchmarkContextIsDoneClosed(b *testing.B) {
	ch := make(chan struct{})
	close(ch)
	ctx := newContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		TestingDone = ctx.IsDone()
	}
}

func BenchmarkContextIsDoneOpen(b *testing.B) {
	ch := make(chan struct{})
	ctx := newContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		TestingDone = ctx.IsDone()
	}
}
