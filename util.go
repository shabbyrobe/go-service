package service

import (
	"context"
	"time"
)

func StartTimeout(timeout time.Duration, runner Runner, services ...*Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runner.Start(ctx, services...)
}

func MustStart(ctx context.Context, runner Runner, services ...*Service) {
	if err := runner.Start(ctx, services...); err != nil {
		panic(err)
	}
}

func MustStartTimeout(timeout time.Duration, runner Runner, services ...*Service) {
	if err := StartTimeout(timeout, runner, services...); err != nil {
		panic(err)
	}
}

func HaltTimeout(timeout time.Duration, runner Runner, services ...*Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runner.Halt(ctx, services...)
}

// MustHalt calls Runner.Halt() and panics if it does not complete successfully.
//
// This is a convenience to allow you to halt in a defer if it is acceptable to
// crash the server if the service does not Halt.
func MustHalt(ctx context.Context, r Runner, services ...*Service) {
	if r == nil {
		return
	}
	if err := r.Halt(ctx, services...); err != nil {
		panic(err)
	}
}

// MustHaltTimeout calls MustHalt using context.WithTimeout()
func MustHaltTimeout(timeout time.Duration, r Runner, services ...*Service) {
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	MustHalt(ctx, r, services...)
}

func ShutdownTimeout(timeout time.Duration, runner Runner) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return runner.Shutdown(ctx)
}

// MustShutdown calls Runner.Shutdown() and panics if it does not complete
// successfully.
//
// This is a convenience to allow you to halt in a defer if it is acceptable to
// crash the server if the runner does not Shutdown.
func MustShutdown(ctx context.Context, r Runner) {
	if r == nil {
		return
	}
	if err := r.Shutdown(ctx); err != nil {
		panic(err)
	}
}

// MustShutdownTimeout calls MustShutdown using context.WithTimeout()
func MustShutdownTimeout(timeout time.Duration, r Runner) {
	if r == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	MustShutdown(ctx, r)
}
