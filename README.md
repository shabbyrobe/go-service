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
- Service groups (should start and halt together)
- `context.Context` support (`service.Context` is a `context.Context`)


It is loosely based on .NET/Java style thread classes.

Here's a quick example (though the
[godoc](https://godoc.org/github.com/shabbyrobe/go-service) contains MUCH
more information):

```go
type MyService struct {}

func (m *MyService) ServiceName() service.Name { return "My service" }

func (m *MyService) Run(ctx service.Context) error {
    if err := ctx.Ready(); err != nil {
        return err
    }
    <-ctx.Done()
    return nil
}

type MyListener struct {}

func (m *MyListener) OnServiceEnd(service Service, err Error) {
    if err != nil {
        fmt.Println("oh noes my service finished prematurely", err)
    }
}

func (m *MyListener) OnServiceError(service Service, err Error) {}

func (m *MyListener) OnServiceState(service Service, state State) {}

func main() {
    runner := service.NewRunner(l)
    svc := &MyService{}

    // Start a service in the background and wait for it to signal it is
    // ready:
    if err := runner.StartWait(1 * time.Second, svc); err != nil {
        log.Fatal(err)
    }

    // Halt a service and wait for it to signal it finished:
    if err := runner.Halt(1 * time.Second, svc); err != nil {
        log.Fatal(err)
    }
}
```


Service Group
-------------

`service.Group` implements the following rules:

- All services should start at the same time
- The group is Ready when all services are Ready
- If one or more services fails while the service is starting, all services are halted and the
  error is returned by StartWait or WaitReady().
- If one or more services fails after the service is started, all services are halted and the
  error is passed to the Listener.
- All services are halted when the group is halted.

It comes with some caveats:

- If halting fails, you should probably panic as I have not yet found a good
  way to recover resources in this case. This should be absolutely exceptional
  for any properly written Service.


```go
func main() {
    runner := service.NewRunner(l)

    group := service.NewGroup([]service.Service{
        &MyService{},
        &MyService{},
        &MyService{},
    })

    // Start the group in the background and wait for all of its child services
    // to signal they are ready:
    if err := runner.StartWait(1 * time.Second, group); err != nil {
        log.Fatal(err)
    }

    // Halt a service and wait for it to signal it finished:
    if err := runner.Halt(1 * time.Second, svc); err != nil {
        log.Fatal(err)
    }
}
```


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
it off. This can help flush out different bugs:

    go run test.go -- -race -v -service.fuzz=true -service.fuzztime=10

See how much coverage we get out of the fuzzer alone:

    go run test.go -cover=cover.out -- -run=TestRunnerFuzz -service.fuzz=true -service.fuzztime=10 -v

