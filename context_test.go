package service

import (
	"testing"
)

var done bool

func BenchmarkContextIsDoneNil(b *testing.B) {
	ctx := newContext(nil, nil, nil, nil)
	for i := 0; i < b.N; i++ {
		done = ctx.IsDone()
	}
}

func BenchmarkContextIsDoneClosed(b *testing.B) {
	ch := make(chan struct{})
	close(ch)
	ctx := newContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		done = ctx.IsDone()
	}
}

func BenchmarkContextIsDoneOpen(b *testing.B) {
	ch := make(chan struct{})
	ctx := newContext(nil, nil, nil, ch)
	for i := 0; i < b.N; i++ {
		done = ctx.IsDone()
	}
}
