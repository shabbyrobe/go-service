package service

type Service struct {
	Name     Name
	Runnable Runnable
	OnEnd    OnEnd
	OnState  OnState
}

func New(n Name, r Runnable) *Service {
	return &Service{Name: n, Runnable: r}
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

type OnEnd func(stage Stage, service *Service, err error)
type OnError func(stage Stage, service *Service, err error)
type OnState func(service *Service, from, to State)
