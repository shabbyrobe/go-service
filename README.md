Goroutine lifecycle management: service
=======================================

[![GoDoc](https://godoc.org/github.com/shabbyrobe/go-service?status.svg)](https://godoc.org/github.com/shabbyrobe/go-service)

service implements service-like goroutine lifecycle management.

It is intended for use when you need to co-ordinate the state of one or more
long-running goroutines and control startup, shutdown and ready signalling.

Key features:

- Start and halt backgrounded goroutines (services)
- Check the state of services
- Wait until a service is "ready" (you decide what "ready" means)
- `context.Context` support (`service.Context` is a `context.Context`)

Goroutines are supremely useful, but they're a little too primitive by
themselves to support the common use-case of a long-lived goroutine that can be
started and halted cleanly. Combined with channels, goroutines can perform this
function well, but the amount of error-prone channel boilerplate that starts to
accumulate in an application with a large number of these long-lived goroutines
can become a maintanability nightmare. `go-service` attempts to solve this
problem with the idea of a "Heavy-weight goroutine" that supports common
mechanisms for control.

It is loosely based on .NET/Java style thread classes, but with a distinct Go
flair.

Here's a very simple example (though the
[godoc](https://godoc.org/github.com/shabbyrobe/go-service) contains MUCH
more information):

```go
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
	svc := service.New(rn).WithEndListener(failer)

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
```

Both `Start` and `Halt` can accept a `context.Context` as the first parameter,
There are global functions that can help to reduce boilerplate for the
`context.WithTimeout()` scenario:

```go
runner := service.NewRunner()

// Starting with a timeout...
err := service.StartTimeout(5*time.Second, runner, svc)

// ... is functionally equivalent to this:
context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
err := runner.Start(context, svc)
```

Both `Start` and `Halt` can accept multiple services at once. `Start` will block
until all services have either signalled ready or failed to start, or until the
context is `Done()`:

```go
runner := service.NewRunner()
svc1 := service.New(&MyRunnable{})
svc2 := service.New(&MyRunnable{})
err := runner.Start(context.Background(), svc1, svc2)
```


Rationale
---------

An earlier version of this library separated the Start and Halt mechanics into
separate methods in an interface, rather than bundling everything together into
the single `Run()` function, but this was not as effective and the system 
became significantly easier to follow when the pattern evolved into `Run()`.
`Run()` gives you access to `defer` for your teardown at the same place as your
resource acquisition, which is much easier to understand.


Testing
-------

On first glance it might look like there are not a lot of tests in here, but
look in the servicetest subpackage; there's an absolute truckload in there.

The test runner is a little bit on the messy side at the moment but it does
work:

    go run test.go [-cover=<file>] -- [options]

`test.go` handles calling `go test` for each package with the required
arguments. `[options]` get passed to the child invocations of `go test` if they
are applicable.

If you pass the `-cover*` arguments after the `--`, the reports won't be
merged. If you pass it to `go run test.go` before the `--`, they will be.

If the tester detects any additional goroutines that have not been closed after
the tests have succeeded, the stack traces for those goroutines will be dumped
and the tester will respond with an error code. This may cause issues on other
platforms but it works on OS X and Ubuntu.

The test suite in the `servicetest` package includes a fuzz tester, disabled by
default. To enable it, pass `-service.fuzz=true` to `go run test.go` after the
`--`.

When using the fuzz tester, it is a good idea to pass `-v` as well.

This will fuzz for 10 minutes and print the results:

    go run test.go -- -v -service.fuzz=true -service.fuzztime=600

This will fuzz for 10 seconds, but will only make a randomised decision every
10ms (this is useful to get more contained tests with fewer things happening to
try to reproduce errors):

    go run test.go -- -v -service.fuzz=true -service.fuzztime=10 -service.fuzzticknsec=10000000

Seed the fuzzer with a particular value:

    go run test.go -- -v -service.fuzz=true -service.fuzztime=10 -service.fuzzseed=12345

You should also run the fuzzer with the race detector turned on as well as with
it off. This can help flush out different kinds of bugs. Due to a crazy-low
limit on the number of goroutines Go will let you start with `-race` turned on,
you will need to limit the number of services that can be created
simultaneously so that error doesn't trip:

    go run test.go -- -race -v -service.fuzz=true -service.fuzztime=10 -service.fuzzservicelim=200

See how much coverage we get out of the fuzzer alone:

    go run test.go -cover=cover.out -- -run=TestRunnerFuzz -service.fuzz=true -service.fuzztime=10 -v

