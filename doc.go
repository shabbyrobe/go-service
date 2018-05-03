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
		<-ctx.Done()
		return nil
	}

	func main() {
		runner := service.NewRunner(nil)
		svc := &MyService{}
		if err := runner.StartWait(1 * time.Second, svc); err != nil {
			log.Fatal(err)
		}
		if err := runner.Halt(1 * time.Second, svc); err != nil {
			log.Fatal(err)
		}
	}


Performance

Services are by nature heavier than a regular goroutine; they're about 10x slower
and use quite a bit more memory. You should probably only use Services when you
need to fully control the management of a long-lived goroutine, otherwise
they're likely not worth it:

	BenchmarkRunnerStart10-4      	   50000	     24519 ns/op	    4641 B/op	      90 allocs/op
	BenchmarkGoroutineStart10-4   	 1000000	      2239 ns/op	       0 B/op	       0 allocs/op

There are opportunities for memory savings in the library, but the chief
priority has been to get it working and stable, rather than fast. I don't plan
to start 50,000 services a second in any app I am currently working on.


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

	- <-ctx.Done() MUST be included in any select {} block

	- service.IsDone(ctx) MUST be checked more frequently than your
	  application's halt timeout if <-ctx.Done() is not used.

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
			case <-ctx.Done():
				return nil
			}
		}
	}

Here is an example of a Run() method which sleeps:

	func (m *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		for !ctx.IsDone() {
			m.doThingsWithTheStuff(stuff)
			service.Sleep(ctx, 1 * time.Second)
		}
		return nil
	}

service.Func allows you to use a bare function as a Service instead of
implementing the Service interface:

	service.Func("My service", func(ctx service.Context) error {
		// valid Run implementation
	})


Runners

To start or halt a service, a Runner is required.

	r := service.NewRunner(nil)
	svc1, svc2 := &MyService{}, &MyService{}

	// start, but don't wait until the service is ready:
	err := r.Start(svc1)

	// start another one and also don't wait:
	err := r.Start(svc2)

	// wait no more than 1 second each for both services to become ready (if
	// there are 2 services, the maximum timeout will be 2 seconds)
	err := service.WhenAllReady(1 * time.Second, svc1, svc2)

	// start another service and wait no more than 1 second until it's ready
	// before returning:
	svc := &MyService{}
	err := r.StartWait(1 * time.Second, svc)

	// the above StartWait call is equivalent to the following (error handling
	// skipped for brevity):
	svc := &MyService{}
	err := r.Start(svc)
	err := r.WhenReady(1 * time.Second, svc)

	// now halt the service we just started, waiting no more than 1 second
	// for the service to end:
	err := r.Halt(1 * time.Second, svc)

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

	func (m *MyListener) OnServiceEnd(stage Stage, service Service, err Error) {
		// This will always be called for every service whose Run() method
		// stops, whether normally or in error, but will not be called if the
		// service panics.
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
		<-ctx.Done()
		return nil
	}

*/
package service
