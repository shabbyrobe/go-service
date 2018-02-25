package service

type Service interface {
	// Run the service, blocking the caller until the service is complete.
	// ready MUST not be nil. ctx.Ready() MUST be called.
	//
	// If Run() ends because <-ctx.Done() has yielded, you MUST return nil.
	// If Run() ends for any other reason, you MUST return an error.
	Run(ctx Context) error

	// Must be unique for each Runner the service is used in.
	ServiceName() Name
}

// Func is a convenience function which creates a Service from a function:
//
//	runner.Start(service.Func("service", func(ctx service.Context) error {
//		if err := ctx.Ready(); err != nil {
//			return err
//		}
//		<-ctx.Done()
//		return nil
//	}))
//
func Func(name Name, fn func(ctx Context) error) Service {
	return &serviceFunc{name: name, fn: fn}
}

type serviceFunc struct {
	name Name
	fn   func(ctx Context) error
}

func (f *serviceFunc) Run(ctx Context) error {
	return f.fn(ctx)
}

func (f *serviceFunc) ServiceName() Name {
	return f.name
}
