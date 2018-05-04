package service

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

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

func TestContextWithCancelStopsOnHalt(t *testing.T) {
	tt := assert.WrapTB(t)
	r := NewRunner(nil)

	done := make(chan struct{})
	svc := Func("", func(ctx Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		cctx, cancel := context.WithCancel(ctx)

		go func() {
			<-cctx.Done()
			close(done)
			cancel()
		}()

		<-ctx.Done()

		return nil
	})
	tt.MustOK(r.Start(svc, nil))
	tt.MustOK(r.Halt(5*time.Second, svc))
	<-done
}

func TestContextWithDeadlineStopsOnHalt(t *testing.T) {
	tt := assert.WrapTB(t)
	r := NewRunner(nil)

	done := make(chan struct{})
	svc := Func("", func(ctx Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		cctx, cancel := context.WithDeadline(ctx, time.Now().Add(1*time.Hour))

		go func() {
			<-cctx.Done()
			close(done)
			cancel()
		}()

		<-ctx.Done()

		return nil
	})
	tt.MustOK(r.Start(svc, nil))
	tt.MustOK(r.Halt(5*time.Second, svc))
	<-done
}

func TestContextNetConn(t *testing.T) {
	// This is kinda a terrible test.

	tt := assert.WrapTB(t)

	var wg sync.WaitGroup
	wg.Add(2) // one for the conn handler, one for the service

	var (
		listening = make(chan struct{})
		connected = make(chan struct{})
		stop      = make(chan struct{})
	)

	listener, err := net.Listen("tcp", ":0")
	tt.MustOK(err)
	port := listener.Addr().(*net.TCPAddr).Port

	go func() {
		conn, err := listener.Accept()
		tt.MustOK(err)
		close(listening)

		go func() {
			defer wg.Done()
			defer listener.Close()
			defer conn.Close()
			<-stop
		}()
	}()

	r := NewRunner(nil)

	svc := Func("", func(ctx Context) error {
		defer wg.Done()

		if err := ctx.Ready(); err != nil {
			return err
		}

		dialer := &net.Dialer{Timeout: 5 * time.Second}

		conn, err := dialer.DialContext(ctx, "tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return err
		}
		defer conn.Close()

		close(connected)

		<-ctx.Done()

		return nil
	})

	tt.MustOK(r.Start(svc, nil))
	<-listening
	<-connected

	tt.MustOK(r.Halt(5*time.Second, svc))

	close(stop)
	wg.Wait()
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
