package service

import "time"

// DefaultHaltTimeout should be inferred wherever possible when <= 0 is used as
// the halt timeout.
var DefaultHaltTimeout = 20 * time.Second

type Service interface {
	// Run the service, blocking the caller until the service is complete.
	// ready MUST not be nil. ctx.Ready() MUST be called.
	//
	// If Run() ends because <-ctx.Halt() has yielded, you MUST return nil.
	// If Run() ends for any other reason, you MUST return an error.
	Run(ctx Context) error

	// Must be unique for each Runner the service is used in.
	ServiceName() Name
}

type serviceFunc struct {
	name Name
	fn   func(ctx Context) error
}

func ServiceFunc(name Name, fn func(ctx Context) error) Service {
	return &serviceFunc{name: name, fn: fn}
}

func (f *serviceFunc) Run(ctx Context) error {
	return f.fn(ctx)
}

func (f *serviceFunc) ServiceName() Name {
	return f.name
}

func Timeout(timeout time.Duration) <-chan time.Time {
	var after <-chan time.Time
	if timeout <= 0 {
		timeout = DefaultHaltTimeout
	}
	if timeout > 0 {
		after = time.After(timeout)
	}
	return after
}
