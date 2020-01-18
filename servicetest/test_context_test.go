// +build ignore

package servicetest

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/internal/assert"
)

func TestContextWithCancelStopsOnHalt(t *testing.T) {
	tt := assert.WrapTB(t)
	r := service.NewRunner(nil)

	done := make(chan struct{})
	svc := service.Func("", func(ctx service.Context) error {
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
	r := service.NewRunner(nil)

	done := make(chan struct{})
	svc := service.Func("", func(ctx service.Context) error {
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
		if err != nil {
			panic(err)
		}
		close(listening)

		go func() {
			defer wg.Done()
			defer listener.Close()
			defer conn.Close()
			<-stop
		}()
	}()

	r := service.NewRunner(nil)

	svc := service.Func("", func(ctx service.Context) error {
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
