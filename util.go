package service

import (
	"context"
	"time"
)

// StartWait starts a Service and waits for it to become Ready().
//
// If an error is returned and the service's status is not Halted or Ended,
// you shoud attempt to Halt() the service. If the service does not successfully
// halt, you should panic as resources have been lost.
//
// An optional context.Context allows you to provide cancellation and timeout
// externally. A nil context may be passed.
func StartWait(ctx context.Context, runner Runner, service *Service) (h Handle, rerr error) {
	sr := NewSignal()
	h, rerr = runner.Start(ctx, service, sr)
	if rerr != nil {
		return nil, rerr
	}
	return h, AwaitSignal(ctx, sr)
}

func StartWaitTimeout(timeout time.Duration, runner Runner, service *Service) (h Handle, rerr error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return StartWait(ctx, runner, service)
}

// HaltWait halts a Service and waits for it to finish halting.
//
// If an error is returned and the service's status is not Halted or Ended,
// you should panic as resources have been lost.
//
// An optional context.Context allows you to provide cancellation and timeout
// externally. A nil context may be passed.
func HaltWait(ctx context.Context, runner Runner, service *Service) error {
	sr := NewSignal()
	if err := runner.Halt(ctx, service, sr); err != nil {
		return err
	}
	return AwaitSignal(ctx, sr)
}

func HaltWaitTimeout(timeout time.Duration, runner Runner, service *Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return HaltWait(ctx, runner, service)
}

// MustHalt calls HaltWait() and panics if it does not complete successfully.
//
// This is a convenience to allow HaltWait to be called in a defer if it is
// acceptable to crash the server if the service does not Halt.
func MustHalt(ctx context.Context, r Runner, s *Service) {
	if r == nil || s == nil {
		return
	}
	if err := HaltWait(ctx, r, s); err != nil {
		panic(err)
	}
}
