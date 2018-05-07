package service

import (
	"testing"
)

func TestRunContextMultipleHalts(t *testing.T) {
	rc := &runContext{
		svcContext: svcContext{
			done: make(chan struct{}, 1),
		},
	}
	rc.Halt()
	rc.Halt()
	rc.Halt()
}

// Done needs to be exported so the compiler doesn't think it should be eliminated.
var TestingDone bool

func BenchmarkContextIsDoneNil(b *testing.B) {
	ctx := newSvcContext(nil, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		TestingDone = IsDone(ctx)
	}
}

func BenchmarkContextIsDoneClosed(b *testing.B) {
	ch := make(chan struct{})
	close(ch)
	ctx := newSvcContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		TestingDone = IsDone(ctx)
	}
}

func BenchmarkContextIsDoneOpen(b *testing.B) {
	ch := make(chan struct{})
	ctx := newSvcContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		TestingDone = IsDone(ctx)
	}
}
