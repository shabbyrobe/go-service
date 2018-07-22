package service

// Service wraps a Runnable with common properties.
type Service struct {
	Name     Name
	Runnable Runnable

	// OnEnd allows you to supply a callback which will be executed whenever a
	// Runnable's Run() function returns.
	//
	// If the Runnable has returned because it was halted, err will be nil. If
	// the service has ended for any other reason, err MUST contain an error.
	//
	// If the Runner has a default OnEnd function (see RunnerOnEnd), both
	// callbacks will be called.
	//
	// It is not safe to call methods on a Runner from the OnEnd callback; doing so
	// may cause your application to deadlock. If you need to call the Runner, wrap
	// the body of your OnEnd in an anonymous goroutine:
	//
	//	svc.OnEnd = func(stage Stage, service *Service, err error) {
	//		// Safe:
	//		go func() {
	//			runner.Start(...)
	//		}()
	//
	//		// NOT SAFE:
	//		runner.Start(...)
	//	}
	//
	OnEnd OnEnd

	// OnStateChange allows you to receive notifications when the state
	// of a service changes. The Runner will drop state changes if this
	// channel is not able to receive them, so supply a big buffer if
	// that concerns you.
	OnState chan StateChange
}

func New(n Name, r Runnable) *Service {
	return &Service{Name: n, Runnable: r}
}

type Endable interface {
	AttachEnd(svc *Service)
}

func (s *Service) WithEndListener(endable Endable) *Service {
	endable.AttachEnd(s)
	return s
}

type Runnable interface {
	// Run the service, blocking the caller until the service is complete.
	// ready MUST not be nil. ctx.Ready() MUST be called.
	//
	// If Run() ends because <-ctx.Done() has yielded, you MUST return nil.
	// If Run() ends for any other reason, you MUST return an error.
	Run(ctx Context) error
}

// RunnableFunc allows you to create a Runnable from a Closure, similar to
// http.HandlerFunc:
//
//	service.New("my-runnable-func", func(ctx service.Context) error {
//		if err := ctx.Ready() {
//			return err
//		}
//		<-ctx.Done()
//		return nil
//	})
//
type RunnableFunc func(ctx Context) error

func (r RunnableFunc) Run(ctx Context) error { return r(ctx) }

type OnEnd func(stage Stage, service *Service, err error)
type OnError func(stage Stage, service *Service, err error)
