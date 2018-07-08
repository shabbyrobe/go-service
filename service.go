package service

import (
	"context"
	"time"
)

type Handle interface {
	Halt(ctx context.Context, done Signal) error
	HaltWait(ctx context.Context) error
	HaltWaitTimeout(time.Duration) error
}

// nilHandle is needed because of Go's typed nil interface.
var nilHandle = &handle{}

type handle struct {
	r   Runner
	svc *Service
}

var _ Handle = &handle{}

func (h *handle) Halt(ctx context.Context, signal Signal) error {
	if h == nilHandle {
		return nil
	}
	return h.r.Halt(ctx, h.svc, signal)
}

func (h *handle) HaltWait(ctx context.Context) error {
	if h == nilHandle {
		return nil
	}
	signal := NewSignal()
	if err := h.r.Halt(ctx, h.svc, signal); err != nil {
		return err
	}
	return AwaitSignal(ctx, signal)
}

func (h *handle) HaltWaitTimeout(timeout time.Duration) error {
	if h == nilHandle {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return h.HaltWait(ctx)
}

type Service struct {
	Name     Name
	Runnable Runnable
	OnEnd    OnEnd
}

func New(n Name, r Runnable) *Service {
	return &Service{Runnable: r}
}

type Runnable interface {
	// Run the service, blocking the caller until the service is complete.
	// ready MUST not be nil. ctx.Ready() MUST be called.
	//
	// If Run() ends because <-ctx.Done() has yielded, you MUST return nil.
	// If Run() ends for any other reason, you MUST return an error.
	Run(ctx Context) error
}

type RunnableFunc func(ctx Context) error

func (r RunnableFunc) Run(ctx Context) error { return r(ctx) }
