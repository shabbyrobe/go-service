/*
Package service implements service-like goroutine lifecycle management.

It is intended for use when you need to co-ordinate the state of one or more
long-running goroutines and control startup and shutdown.


Quick Example

	type MyRunnable struct {}

	func (m *MyRunnable) Run(ctx service.Context) error {
		// Set up your stuff:
		t := time.NewTicker()
		defer t.Stop()

		// Notify the Runner that we are 'ready', which will unblock the call
		// Runner.Start().
		//
		// If you omit this, Start() will never unblock; failing to call Ready()
		// in a Runnable is an error.
		if err := ctx.Ready(); err != nil {
			return err
		}

		// Run the service, awaiting an instruction from the runner to Halt:
		select {
		case <-ctx.Done():
		case t := <-tick:
			fmt.Println(t)
		}

		return nil
	}

	func run() error {
		runner := service.NewRunner()

		// Ensure that every service is shut down within 10 seconds, or panic
		// if the deadline is exceeded:
		defer service.MustShutdownTimeout(10*time.Second, runner)

		rn := &MyRunnable{}

		// If you want to be notified if the service ends prematurely, attach
		// an EndListener.
		failer := service.NewFailureListener(1)
		svc := service.New("my-service", rn).WithEndListener(failer)

		// Start a service in the background. The call to Start will unblock when
		// MyRunnable.Run() calls ctx.Ready():
		if err := runner.Start(context.TODO(), svc); err != nil {
			return err
		}

		after := time.After(10*time.Second)

		select {
		case <-after:
			// Halt a service and wait for it to signal it finished:
			if err := runner.Halt(context.TODO(), svc); err != nil {
				return err
			}

		case err := <-failer.Failures():
			// If something goes wrong and MyRunnable ends prematurely,
			// the error returned by MyRunnable.Run() will be sent to the
			// FailureListener.Failures() channel.
			return err
		}

		return nil
	}

Performance

Services are by nature heavier than a regular goroutine; they're up to 10x
slower and use more memory. You should probably only use Services when you need
to fully control the management of a long-lived goroutine, otherwise they're
likely not worth it:

	BenchmarkRunnerStart1-4          	  500000	      2951 ns/op	     352 B/op	       6 allocs/op
	BenchmarkGoroutineStart1-4       	 5000000	       368 ns/op	       0 B/op	       0 allocs/op
	BenchmarkRunnerStart10-4         	  100000	     20429 ns/op	    3521 B/op	      60 allocs/op
	BenchmarkGoroutineStart10-4      	  500000	      2933 ns/op	       0 B/op	       0 allocs/op

There are plenty of opportunities for memory savings in the library, but the
chief priority has been to get a working, stable and complete API first. I
don't plan to start 50,000 services a second in any app I am currently working
on, but this is not to say that optimising the library isn't important, it's
just not a priority yet. YMMV.


Runnables

Runnables can be created by implementing the Runnable interface. This interface
only contains one method (Run), but there are some very important caveats in order
to correctly implement it:

	type MyRunnable struct {}

	func (m *MyRunnable) Run(ctx service.Context) error {
		// This MUST be present in every implementation of service.Runnable:
		if err := ctx.Ready(); err != nil {
			return err
		}

		// You must wait for the signal to Halt. You can also poll
		// ctx.ShouldHalt().
		<-ctx.Done()

		return nil
	}

The Run() method will be run in the background by a Runner. The Run() method
MUST do the following to be considered valid. Violating any of these rules
will result in Undefined Behaviour (uh-oh!):

	- ctx.Ready() MUST be called and error checked properly

	- <-ctx.Done() MUST be included in any select {} block

	- OR... ctx.ShouldHalt() MUST be checked frequently enough that your
	  calls to Halt() won't time out if <-ctx.Done() is not used.

	- If Run() ends before it is halted by a Runner, an error MUST be returned.
	  If there is no obvious application specific error to return in this case,
	  service.ErrServiceEnded MUST be returned.

The Run() method SHOULD do the following:

	- service.Sleep(ctx) should be used instead of time.Sleep(); service.Sleep()
	  is haltable.

Here is an example of a Run() method which uses a select{} loop:

	func (m *MyRunnable) Run(ctx service.Context) error {
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
		for !ctx.ShouldHalt() {
			m.doThingsWithTheStuff(stuff)
			service.Sleep(ctx, 1 * time.Second)
		}
		return nil
	}

service.RunnableFunc allows you to use a bare function as a Runnable instead of
implementing the Service interface:

	service.RunnableFunc("My service", func(ctx service.Context) error {
		// valid Run implementation
	})


Runners

To start or halt a Runnable, a Runner is required and the Runnable must be wrapped
in a service.Service:

	runner := service.NewRunner(nil)
	rn1, rn2 := &MyRunnable{}, &MyRunnable{}
	svc1, svc2 := service.New("s1", rn1), service.New("s2", rn2)

	// start svc1 and wait until it is ready:
	err := runner.Start(context.TODO(), svc1)

	// start svc1 and svc2 simultaneously and wait until both of them are ready:
	err := runner.Start(context.TODO(), svc1, svc2)

	// start both services, but wait no more than 1 second for them both to be ready:
	err := service.StartTimeout(1 * time.Second, runner, svc1, svc2)
	if err != nil {
		// You MUST attempt to halt the services if StartTimeout does not succeed:
		service.MustHaltTimeout(1 * time.Second, runner, svc1, svc2)
	}

	// the above StartTimeout call is equivalent to the following (error handling
	// skipped for brevity):
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := runner.Start(ctx, svc1, svc2)

	// now halt the services we just started, unblocking when both services have
	// ended or failed to end:
	err := runner.Halt(context.TODO(), svc1, svc2)
	if err != nil {
		// If Halt() fails, we have probably leaked a resource. If you have no
		// mechanism to recover it, it could be time to crash:
		panic(err)
	}

	// halt every service currently started in the runner:
	err := runner.Shutdown(context.TODO())

	// halt every service in the runner, waiting no more than 1 second for them
	// to finish halting:
	err := service.ShutdownTimeout(1*time.Second, runner)


Contexts

Service.Run receives a service.Context as its first parameter. service.Context
implements context.Context (https://golang.org/pkg/context/).

service.Context can be used exactly as a context.Context is used for your
service code, with some caveats:

	func (s *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}

		dctx, cancel := context.WithDeadline(ctx, time.Now().Add(2 * time.Second))
		defer cancel()

		// This service will be "Done" either when the service is halted,
		// or the deadline arrives (though in the latter case, the service
		// will be considered to have ended prematurely)
		<-dctx.Done()

		// If the service wasn't halted (i.e. if the deadline elapsed), we must
		// return an error to satisfy the service.Run contract outlined in the
		// docs:
		if !ctx.IsDone() {
			returh errServiceEnded
		}

		return nil
	}

Services can not work without a cancelable context (how else would you implement
runner.Halt?), so the service package assumes control of context creation. This
is not ideal, but the context package provides no mechanism to detect whether a
context has been wrapped with WithCancel, and no way to access the cancel()
function via the context.Context itself. I haven't found a good way of allowing
externally created contexts to be passed in without totally destroying the API
yet, but it's definitely something I'm looking into.


Listeners

Errors may happen during a service's execution. Services may end prematurely.
If these kinds of things happen, the parent context may wish to be notified via
a Listener.

NewRunner() takes an implementation of the Listener interface:

	type MyListener struct {}

	func (m *MyListener) OnServiceEnd(stage Stage, service Service, err Error) {
		// This will always be called for every service whose Run() method
		// stops, whether normally or in error, but will not be called if the
		// service panics.
	}

	func main() {
		l := &MyListener{}
		r := NewRunnner(l)
		// ...
	}

Every call to Runner.Start or service.StartWait is matched with a call to
OnServiceEnd, regardless of whether the call to Start failed at any stage,
ended prematurely, or was halted by Runner.Halt. The err argument will be nil
if the service was halted, but MUST be an error in any other circumstance.

The Listener may also optionally implement service.ErrorListener and/or
service.StateListener:

	func (m *MyListener) OnServiceError(service Service, err Error) {
		// This will be called every time you call ctx.OnError() in your
		// service so non-fatal errors that occur during the lifetime
		// of your service have a place to go.
	}

	func (m *MyListener) OnServiceState(service Service, state State) {
		// This is called whenever a service transitions into a state.
	}


Restarting

All services can be restarted if they are stopped by default. If written
carefully, it's also possible to start the same Service in multiple Runners.
Maybe that's not a good idea, but who am I to judge? You might have a great
reason.

Some services may wish to explicitly block restart, such as services that
wrap a net.Conn (which will not be available if the service fails). An
atomic can be a good tool for this job:

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
