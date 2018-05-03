package service

import (
	"bytes"
	"expvar"
	"flag"
	"fmt"
	"net/http"
	netprof "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	// HACK FEST: This needs to be high enough so that tests that rely on
	// timing don't fail because your computer was too slow
	tscale = 5 * time.Millisecond

	dto = 100 * tscale
)

var (
	fuzzEnabled   bool
	fuzzTimeStr   string
	fuzzTimeDur   time.Duration
	fuzzTickNsec  int64
	fuzzSeed      int64
	fuzzDebugHost string
	fuzzMetaRolls int64
	fuzzMetaMin   int
)

func TestMain(m *testing.M) {
	flag.BoolVar(&fuzzEnabled, "service.fuzz", false, "Fuzz? Nope by default.")
	flag.StringVar(&fuzzTimeStr, "service.fuzztime", "1s", "Run the fuzzer for this duration")
	flag.Int64Var(&fuzzTickNsec, "service.fuzzticknsec", 0, "How frequently to tick in the fuzzer's loop.")
	flag.Int64Var(&fuzzSeed, "service.fuzzseed", -1, "Randomise the fuzz tester with this non-negative seed prior to every fuzz test")
	flag.StringVar(&fuzzDebugHost, "service.debughost", "", "Start a debug server at this host to allow expvars/pprof")
	flag.Int64Var(&fuzzMetaRolls, "service.fuzzmetarolls", 20, "Re-roll the meta fuzz tester this many times")
	flag.IntVar(&fuzzMetaMin, "service.fuzzmetamin", 5, "Minimum number of times to run the meta fuzzer regardless of duration")
	flag.Parse()

	var err error
	fuzzTimeDur, err = time.ParseDuration(fuzzTimeStr)
	if err != nil {
		panic(err)
	}

	if fuzzDebugHost != "" {
		mux := http.NewServeMux()
		mux.Handle("/debug/vars", expvar.Handler())
		mux.Handle("/debug/pprof/", http.HandlerFunc(netprof.Index))
		mux.Handle("/debug/pprof/cmdline", http.HandlerFunc(netprof.Cmdline))
		mux.Handle("/debug/pprof/profile", http.HandlerFunc(netprof.Profile))
		mux.Handle("/debug/pprof/symbol", http.HandlerFunc(netprof.Symbol))
		mux.Handle("/debug/pprof/trace", http.HandlerFunc(netprof.Trace))
		expvar.Publish("app", expvar.Func(func() interface{} {
			out := map[string]interface{}{
				"Goroutines": runtime.NumGoroutine(),
			}
			return out
		}))
		expvar.Publish("fuzz", expvar.Func(func() interface{} {
			fz := getCurrentFuzzer()
			if fz != nil {
				return fz.Stats.Map()
			}
			return nil
		}))

		runtime.SetMutexProfileFraction(5)

		go func() {
			if err := http.ListenAndServe(fuzzDebugHost, mux); err != nil {
				panic(err)
			}
		}()
	}

	if fuzzSeed < 0 {
		fuzzSeed = time.Now().UnixNano()
		// I mean, this is almost certainly not going to happen, but what if you set the
		// clock to something stupid for a legitimate test? Who am I to judge?
		if fuzzSeed < 0 {
			fuzzSeed = -fuzzSeed
		}
	}

	fmt.Printf("Fuzz seed: %d\n", fuzzSeed)

	beforeCount := pprof.Lookup("goroutine").Count()
	code := m.Run()

	// This little hack gives things like "go OnServiceState" a chance to
	// finish - it routinely shows up in the profile.
	//
	// Also, some calls to the listener that are called with "go" might
	// not have had a chance to finish. This is brittle, true, but some
	// of the tests are hopelessly complicated without it.
	time.Sleep(20 * time.Millisecond)

	if code == 0 {

		after := pprof.Lookup("goroutine")
		afterCount := after.Count()

		diff := afterCount - beforeCount
		if diff > 0 {
			var buf bytes.Buffer
			after.WriteTo(&buf, 1)
			fmt.Fprintf(os.Stderr, "stray goroutines: %d\n%s\n", diff, buf.String())
			os.Exit(2)
		}
	}

	os.Exit(code)
}

type listenerCollectorEnd struct {
	stage Stage
	err   error
}

type listenerCollectorService struct {
	errs       []Error
	states     []State
	ends       []*listenerCollectorEnd
	endWaiters []chan struct{}
	errWaiters []*errWaiter
}

type listenerCollector struct {
	services map[Service]*listenerCollectorService
	lock     sync.Mutex
}

func newListenerCollector() *listenerCollector {
	return &listenerCollector{
		services: make(map[Service]*listenerCollectorService),
	}
}

func (t *listenerCollector) errs(service Service) (out []Error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	svc := t.services[service]
	if svc == nil {
		return
	}
	for _, e := range svc.errs {
		out = append(out, e)
	}
	return
}

func (t *listenerCollector) ends(service Service) (out []listenerCollectorEnd) {
	t.lock.Lock()
	defer t.lock.Unlock()
	svc := t.services[service]
	if svc == nil {
		return
	}
	for _, e := range svc.ends {
		out = append(out, *e)
	}
	return
}

func (t *listenerCollector) endWaiter(service Service) chan struct{} {
	// FIXME: endWaiter should have a timeout
	t.lock.Lock()
	if t.services[service] == nil {
		t.services[service] = &listenerCollectorService{}
	}
	svc := t.services[service]
	w := make(chan struct{}, 1)
	svc.endWaiters = append(svc.endWaiters, w)
	t.lock.Unlock()

	return w
}

type errWaiter struct {
	C chan error
}

func (e *errWaiter) Take(n int, timeout time.Duration) []error {
	out := make([]error, n)
	for i := 0; i < n; i++ {
		wait := time.After(timeout)
		select {
		case out[i] = <-e.C:
		case <-wait:
			panic("errwaiter timeout")
		}
	}
	return out
}

func (t *listenerCollector) errWaiter(service Service, cap int) *errWaiter {
	t.lock.Lock()
	if t.services[service] == nil {
		t.services[service] = &listenerCollectorService{}
	}
	svc := t.services[service]
	w := &errWaiter{C: make(chan error, cap)}
	svc.errWaiters = append(svc.errWaiters, w)
	t.lock.Unlock()

	return w
}

func (t *listenerCollector) OnServiceState(service Service, state State) {
	t.lock.Lock()
	if t.services[service] == nil {
		t.services[service] = &listenerCollectorService{}
	}
	svc := t.services[service]
	svc.states = append(svc.states, state)
	t.lock.Unlock()
}

func (t *listenerCollector) OnServiceError(service Service, err Error) {
	t.lock.Lock()
	if t.services[service] == nil {
		t.services[service] = &listenerCollectorService{}
	}
	svc := t.services[service]
	svc.errs = append(svc.errs, err)

	if len(svc.errWaiters) > 0 {
		for _, w := range svc.errWaiters {
			w.C <- err
		}
	}
	t.lock.Unlock()
}

func (t *listenerCollector) OnServiceEnd(stage Stage, service Service, err Error) {
	t.lock.Lock()
	if t.services[service] == nil {
		t.services[service] = &listenerCollectorService{}
	}
	svc := t.services[service]

	svc.ends = append(svc.ends, &listenerCollectorEnd{
		stage: stage,
		err:   cause(err),
	})
	if len(svc.endWaiters) > 0 {
		for _, w := range svc.endWaiters {
			close(w)
		}
		svc.endWaiters = nil
	}
	t.lock.Unlock()
}

type dummyListener struct {
}

func newDummyListener() *dummyListener {
	return &dummyListener{}
}

func (t *dummyListener) OnServiceState(service Service, state State) {}

func (t *dummyListener) OnServiceError(service Service, err Error) {}

func (t *dummyListener) OnServiceEnd(stage Stage, service Service, err Error) {}

type statService interface {
	ServiceName() Name
	Starts() int
	Halts() int
}

type dummyService struct {
	name         Name
	startFailure error
	startDelay   time.Duration
	runFailure   error
	runTime      time.Duration
	haltDelay    time.Duration
	haltingSleep bool
	starts       int32
	halts        int32
}

func (d *dummyService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *dummyService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *dummyService) ServiceName() Name {
	if d.name == "" {
		// This is a nasty cheat, don't do it in any real code!
		return Name(fmt.Sprintf("dummyService-%p", d))
	}
	return d.name
}

func (d *dummyService) Run(ctx Context) error {
	atomic.AddInt32(&d.starts, 1)

	if d.startDelay > 0 {
		time.Sleep(d.startDelay)
	}
	if d.startFailure != nil {
		return d.startFailure
	}
	if err := ctx.Ready(); err != nil {
		return err
	}

	defer atomic.AddInt32(&d.halts, 1)

	if d.runTime > 0 {
		if d.haltingSleep {
			Sleep(ctx, d.runTime)
		} else {
			time.Sleep(d.runTime)
		}
	}
	if IsDone(ctx) {
		if d.haltDelay > 0 {
			time.Sleep(d.haltDelay)
		}
		return nil
	} else {
		if d.runFailure == nil {
			return ErrServiceEnded
		}
		return d.runFailure
	}
}

type errorService struct {
	name       Name
	startDelay time.Duration
	errc       chan error
	buf        int
	init       bool
}

func (d *errorService) Init() *errorService {
	d.init = true
	if d.buf <= 0 {
		d.buf = 10
	}
	d.errc = make(chan error, d.buf)
	return d
}

func (d *errorService) ServiceName() Name {
	if d.name == "" {
		// This is a nasty cheat, don't do it in any real code!
		return Name(fmt.Sprintf("errorService-%p", d))
	}
	return d.name
}

func (d *errorService) Run(ctx Context) error {
	if !d.init {
		panic("call Init()!")
	}
	if d.startDelay > 0 {
		after := time.After(d.startDelay)
		for {
			select {
			case err := <-d.errc:
				ctx.OnError(err)
			case <-after:
				goto startDone
			}
		}
	startDone:
	}
	if err := ctx.Ready(); err != nil {
		return err
	}
	for {
		select {
		case err := <-d.errc:
			ctx.OnError(err)
		case <-ctx.Done():
			return nil
		}
	}
}

type unhaltableService struct {
	halt chan error
	name Name
	init bool
}

func (u *unhaltableService) Init() *unhaltableService {
	u.init = true
	if u.name == "" {
		u.name.AppendUnique()
	}
	u.halt = make(chan error)
	return u
}

func (u *unhaltableService) ServiceName() Name { return u.name }

func (u *unhaltableService) Run(ctx Context) error {
	if !u.init {
		panic("call Init()!")
	}
	if err := ctx.Ready(); err != nil {
		return err
	}
	return <-u.halt
}

type blockingService struct {
	name         Name
	startFailure error
	runFailure   error
	startDelay   time.Duration
	haltDelay    time.Duration
	init         bool
	starts       int32
	halts        int32
}

func (d *blockingService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *blockingService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *blockingService) Init() *blockingService {
	d.init = true
	if d.name == "" {
		d.name.AppendUnique()
	}
	return d
}

func (d *blockingService) ServiceName() Name { return d.name }

func (d *blockingService) Run(ctx Context) error {
	// defer fmt.Println("dummy ENDED", d.ServiceName())
	// fmt.Println("RUNNING", d.ServiceName())

	atomic.AddInt32(&d.starts, 1)

	if !d.init {
		panic("call Init()!")
	}
	if d.startDelay > 0 {
		time.Sleep(d.startDelay)
	}
	if d.startFailure != nil {
		return d.startFailure
	}
	if err := ctx.Ready(); err != nil {
		return err
	}

	<-ctx.Done()
	if d.haltDelay > 0 {
		time.Sleep(d.haltDelay)
	}

	atomic.AddInt32(&d.halts, 1)
	return d.runFailure
}

type runnerWithFailingStart struct {
	Runner

	// start this many services, then fail
	failAfter int

	err error
}

func (t *runnerWithFailingStart) Start(service Service, ready ReadySignal) (err error) {
	if t.failAfter > 0 {
		err = t.Runner.Start(service, ready)
		t.failAfter--
	} else {
		err = t.err
	}
	return
}

func errorListSorted(err error) (out []error) {
	if eg, ok := err.(errorGroup); ok {
		out = eg.Errors()
		sort.Slice(out, func(i, j int) bool {
			return out[i].Error() < out[j].Error()
		})
		return
	} else {
		return []error{err}
	}
}

func causeListSorted(err error) (out []error) {
	if eg, ok := err.(errorGroup); ok {
		out = eg.Errors()
		for i := 0; i < len(out); i++ {
			out[i] = cause(out[i])
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].Error() < out[j].Error()
		})
		return
	} else {
		return []error{err}
	}
}
