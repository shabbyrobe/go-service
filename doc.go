/*
Package service implements service-like goroutine lifecycle management.

It is intended for use when you need to co-ordinate the state of one or more
long-running goroutines and control startup and shutdown.

Package service is complemented by the
'github.com/shabbyrobe/go-service/services' package, which provides a global
version of a service.Runner for use in simpler applications.


Quick Example

	type MyRunnable struct {}

	func (m *MyRunnable) Run(ctx service.Context) error {
		// Set up your stuff:
		tick := time.NewTicker()
		defer tick.Stop()

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
		case t := <-tick.C:
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
	if err := runner.Halt(context.TODO(), svc1, svc2); err != nil {
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

You should use the ctx as the basis for any child contexts you wish to create
in your service, as you will then gain access to cancellation propagation from
Runner.Halt():

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
		return dctx.Err()
	}

The rules around when the ctx passed to Run() is considered Done() are different
depending on whether ctx.Ready() has been called. If ctx.Ready() has not yet been
called, the ctx passed to Run() is Done() if:

	- The service is halted using Runner.Halt()
	- The context passed to Runner.Start() is either cancelled or its deadline
	  is exceeded.

If ctx.Ready() has been called, the ctx passed to Run() is Done() if:

	- The service is halted using Runner.Halt()
	- That's it.

The context passed to Runner.Halt() is not bound to the ctx passed to Run(). The
rationale for this decision is that if you need to do things in your service that
require a context.Context after Run's ctx is Done() (i.e. when your Runnable is
Halting), you are responsible for creating your own context:

	func (s *MyService) Run(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	}

The guideline for this may change, but at the moment the best recommendation is
to use context.Background() in this case: if you are calling Runner.Halt() and
your context deadline runs out, you have lost resources no matter what.


Listeners

Runner takes a list of functional RunnerOptions that can be used to listen
to events.

The RunnerOnEnd option allows you to supply a callback which is executed
when a service ends:

	endFn := func(stage Stage, service *Service, err error) {
		// behaviour you want when a service ends before it is halted
	}
	r := service.NewRunner(service.RunnerOnEnd(endFn))

OnEnd will always be called if a Runnable's Run() function returns, whether
that is because the service failed prematurely or because it was halted. If the
service has ended because it was halted, err will be nil. If the service has
ended for any other reason, err MUST contain an error.

The RunnerOnError option allows you to supply a callback which is executed
when a service calls service.Context.OnError(err). Context.OnError(err) is
used when an error occurs that cannot be handled and does not terminate the
service. It's most useful for logging:

	errorFn := func(stage Stage, service *Service, err error) {
		log.Println(service.Name, err)
	}
	r := service.NewRunner(service.RunnerOnError(endFn))

You MUST NOT attempt to call your Runner from inside your OnEnd or OnError
function. This will cause a deadlock. If you wish to call the runner, wrap
your function body in an anonymous goroutine:

	endFn := func(stage Stage, service *Service, err error) {
		// Safe:
		go func() {
			err := runner.Start(...)
		}()

		// NOT SAFE:
		err := runner.Start(...)
	}


Restarting

All Runnable implementations are restartable by default. If written carefully,
it's also possible to start the same Runnable in multiple Runners. Maybe that's
not a good idea, but who am I to judge? You might have a great reason.

Some Runnable implementations may wish to explicitly block restart, such as
tings that wrap a net.Conn (which will not be available if the service fails).
An atomic can be a good tool for this job:

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
