/*
Package service implements service-like goroutine lifecycle management.

It is intended for use when you need to co-ordinate the state of one or more
long-running goroutines and control startup and shutdown.


Quick Example

	type MyService struct {}

	func (m *MyService) ServiceName() service.Name { return "My service" }

	func (m *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Halt()
		return nil
	}

	func main() {
		runner := service.NewRunner(nil)
		svc := &MyService{}
		if err := runner.StartWait(svc, 1 * time.Second); err != nil {
			log.Fatal(err)
		}
		if err := runner.Halt(svc, 1 * time.Second); err != nil {
			log.Fatal(err)
		}
	}


Services

Services can be created by implementing the Service interface. This interface
only contains two methods, but there are some very important caveats in order
to correctly implement the Run() method.

	type MyService struct {}

	func (m *MyService) ServiceName() service.Name {
		return "My service"
	}

	func (m *MyService) Run(ctx service.Context) error {
		// valid Run() implementation
	}

The Run() method will be run in the background by a Runner. The Run() method
MUST do the following to be considered valid. Violating any of these rules
will result in Undefined Behaviour (uh-oh!):

	- ctx.Ready() MUST be called and error checked properly

	- <-ctx.Halt() MUST be included in any select {} block

	- ctx.Halted() MUST be checked more frequently than your application's halt
	  timeout if <-ctx.Halt() is not used.

	- If Run() ends before it is halted by a Runner, an error MUST be returned.
	  If there is no obvious application specific error to return in this case,
	  ErrServiceEnded MUST be returned.

The Run() method SHOULD do the following:

	- service.Sleep(ctx) should be used instead of time.Sleep()

Here is an example of a Run() method which uses a select{} loop:

	func (m *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		for {
			select {
			case stuff := <-m.channelOfStuff:
				m.doThingsWithTheStuff(stuff)
			case <-ctx.Halt():
				return nil
			}
		}
	}

Here is an example of a Run() method which sleeps:

	func (m *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		for !ctx.Halted() {
			m.doThingsWithTheStuff(stuff)
			service.Sleep(ctx, 1 * time.Second)
		}
		return nil
	}

ServiceFunc allows you to use a bare function as a Service instead of
implementing the Service interface:

	service.ServiceFunc("My service", func(ctx service.Context) error {
		// valid Run implementation
	})


Runners

To start or halt a service, a Runner is required.

	r := NewRunner(nil)

	// start, but don't wait until the service is ready:
	err := r.Start(&MyService{})

	// start another one and also don't wait:
	err := r.Start(&MyService{})

	// wait no more than 1 second for both services to become ready
	err := <-r.WhenReady(1 * time.Second)

	// start another service, but this time wait no more than 1 second
	// until it's ready before returning:
	svc := &MyService{}
	err := r.StartWait(svc, 1 * time.Second)

	// now halt the service we just started, waiting no more than 1 second
	// for the service to end:
	err := r.Halt(svc, 1 * time.Second)

	// halt every service currently started in the runner, waiting no more
	// than 1 second for each service to be halted (if there are 3 services,
	// the maximum timeout will be 3 seconds):
	err := r.HaltAll(1 * time.Second)


Listeners

Errors may happen during a service's execution. Services may end prematurely.
If these kinds of things happen, the parent context may wish to be notified via
a Listener.

NewRunner() takes an implementation of the Listener interface:

	type MyListener struct {}

	func (m *MyListener) OnServiceError(service Service, err Error) {
		// This will be called every time you call ctx.OnError() in your
		// service so non-fatal errors that occur during the lifetime
		// of your service have a place to go.
	}

	func (m *MyListener) OnServiceEnd(service Service, err Error) {
		// This will always be called for every service whose Run() method
		// stops normally (i.e. without panicking).
	}

	func (m *MyListener) OnServiceState(service Service, state State) {
		// This is called whenever a service transitions into a state.
	}

	func main() {
		l := &MyListener{}
		r := NewRunnner(l)
		// ...
	}

The Listener allows the parent context to respond to changes that may happen
outside of the expected Start/Halt lifecycle.


Restarting

All services can be restarted if they are stopped by default. If written
carefully, it's also possible to start the same Service in multiple Runners.
Maybe that's not a good idea, but who am I to judge? You might have a great
reason.

Some services may wish to explicitly block restart, in which case an atomic
is a good way to prevent it:

	type MyService struct {
		used int32
	}

	func (m *MyService) Run(ctx service.Context) error {
		if !atomic.CompareAndSwapInt32(&m.used, 0, 1) {
			return errors.New("cannot reuse MyService")
		}
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Halt()
		return nil
	}

*/
package service
