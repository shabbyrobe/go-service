package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

// TODO:
// - should attempt to restart certain services
// - runnerWithFailingStart chance

const (
	DefaultRunnerLimit  int = 20000
	DefaultServiceLimit int = 100000
)

// Used for expvar
var currentFuzzer *RunnerFuzzer
var currentFuzzerLock sync.Mutex

func getCurrentFuzzer() *RunnerFuzzer {
	currentFuzzerLock.Lock()
	defer currentFuzzerLock.Unlock()
	return currentFuzzer
}

func setCurrentFuzzer(fz *RunnerFuzzer) {
	currentFuzzerLock.Lock()
	currentFuzzer = fz
	currentFuzzerLock.Unlock()
}

func TestRunnerFuzzHappy(t *testing.T) {
	// Happy config: should yield no errors
	stats := NewStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		SyncHalt:           true,
		RunnerCreateChance: 0.001,
		RunnerHaltChance:   0.0,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0,
		GroupServiceRunFailureChance:   0,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{10 * time.Second, 10 * time.Second},
		ServiceRunTime:     TimeRange{10 * time.Second, 10 * time.Second},
		ServiceHaltAfter:   TimeRange{200 * time.Millisecond, 1 * time.Second},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{10 * time.Second, 10 * time.Second},

		ServiceRegisterBeforeStartChance:  0.1,
		ServiceRegisterAfterStartChance:   0.1,
		ServiceUnregisterHaltChance:       0.3,
		ServiceUnregisterUnexpectedChance: 0,

		StateCheckChance: 0.2,

		Stats: stats,
	})

	tt := assert.WrapTB(t)
	tt.MustEqual(0, stats.GetServicesCurrent())
	for _, s := range []*ServiceStats{stats.ServiceStats, stats.GroupStats} {
		tt.MustEqual(0, s.ServiceHalt.Failed())
		tt.MustEqual(0, s.ServiceStart.Failed())
		tt.MustEqual(0, s.ServiceStartWait.Failed())
		tt.MustEqual(0, s.ServiceRegisterBeforeStart.Failed())
		tt.MustEqual(0, s.ServiceUnregisterHalt.Failed())
		tt.MustEqual(0, s.ServiceUnregisterUnexpected.Failed())
	}
}

func TestRunnerFuzzReasonable(t *testing.T) {
	// Happy error path: will yield plenty of errors, but should not yield
	// any unrecoverable ones.
	stats := NewStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		SyncHalt:           true,
		RunnerCreateChance: 0.001,
		RunnerHaltChance:   0.0,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0.01,
		ServiceRunFailureChance:   0.02,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.02,
		GroupServiceRunFailureChance:   0.02,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{10 * time.Second, 10 * time.Second},
		ServiceRunTime:     TimeRange{700 * time.Millisecond, 2 * time.Second},
		ServiceHaltAfter:   TimeRange{100 * time.Millisecond, 1 * time.Second},
		ServiceHaltDelay:   TimeRange{0, 100 * time.Microsecond},
		ServiceHaltTimeout: TimeRange{10 * time.Second, 10 * time.Second},

		ServiceRegisterBeforeStartChance:  0.01,
		ServiceRegisterAfterStartChance:   0.01,
		ServiceUnregisterHaltChance:       0.01,
		ServiceUnregisterUnexpectedChance: 0.01,

		StateCheckChance: 0.2,

		Stats: stats,
	})

	tt := assert.WrapTB(t)
	tt.MustEqual(0, stats.GetServicesCurrent())
	tt.MustEqual(stats.Starts(), stats.Ends())

	tt.MustAssert(stats.GroupStats.ServiceStartWait.Succeeded() > stats.GroupStats.ServiceStartWait.Failed())
	tt.MustAssert(stats.GroupStats.ServiceStart.Succeeded() > stats.GroupStats.ServiceStart.Failed())
	tt.MustAssert(stats.GroupStats.ServiceHalt.Succeeded() > stats.GroupStats.ServiceHalt.Failed())

	tt.MustAssert(stats.ServiceStats.ServiceStartWait.Succeeded() > stats.ServiceStats.ServiceStartWait.Failed())
	tt.MustAssert(stats.ServiceStats.ServiceStart.Succeeded() > stats.ServiceStats.ServiceStart.Failed())
	tt.MustAssert(stats.ServiceStats.ServiceHalt.Succeeded() > stats.ServiceStats.ServiceHalt.Failed())
}

func TestRunnerFuzzMessy(t *testing.T) {
	stats := NewStats()
	fz := &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.000,
		RunnerHaltChance:   0.000,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0.05,
		ServiceRunFailureChance:   0.05,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.05,
		GroupServiceRunFailureChance:   0.05,

		StartWaitChance:                   0.2,
		ServiceStartTime:                  TimeRange{0, 21 * time.Millisecond},
		StartWaitTimeout:                  TimeRange{20 * time.Millisecond, 1 * time.Second},
		ServiceRunTime:                    TimeRange{0, 500 * time.Millisecond},
		ServiceHaltAfter:                  TimeRange{0, 500 * time.Millisecond},
		ServiceHaltDelay:                  TimeRange{0, 10 * time.Millisecond},
		ServiceHaltTimeout:                TimeRange{9 * time.Millisecond, 10 * time.Millisecond},
		StateCheckChance:                  0.2,
		ServiceUnregisterHaltChance:       0.3,
		ServiceUnregisterUnexpectedChance: 0.02,

		Stats: stats,
	}
	testFuzz(t, fz)

	tt := assert.WrapTB(t)

	// Experimental check
	// mustWithin(tt, 0.5, fz.ServiceStartFailureChance+fz.ServiceRunFailureChance,
	//     fz.Stats.ServiceStats.ServiceEnds["start failure"]+
	//         fz.Stats.ServiceStats.ServiceEnds["run failure"]+
	//         fz.Stats.ServiceStats.ServiceEnds["service ended"],
	//     fz.Stats.ServiceStats.ServiceStart.succeeded)

	for _, s := range []*ServiceStats{stats.ServiceStats, stats.GroupStats} {
		tt.MustEqual(0, s.ServiceStart.Failed())
	}
}

func TestRunnerFuzzOutrage(t *testing.T) {
	// Pathological configuration - should fail far more often than it succeeds and
	// seems to leave stray crap lying around.
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.005,
		RunnerHaltChance:   0.005,

		ServiceCreateChance:       0.3,
		ServiceStartFailureChance: 0.1,
		ServiceRunFailureChance:   0.2,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.05,
		GroupServiceRunFailureChance:   0.05,

		StartWaitChance:                   0.2,
		ServiceStartTime:                  TimeRange{0, 50 * time.Millisecond},
		StartWaitTimeout:                  TimeRange{0, 50 * time.Millisecond},
		ServiceRunTime:                    TimeRange{0, 50 * time.Millisecond},
		ServiceHaltAfter:                  TimeRange{0, 50 * time.Millisecond},
		ServiceHaltDelay:                  TimeRange{0, 50 * time.Millisecond},
		ServiceHaltTimeout:                TimeRange{1 * time.Nanosecond, 50 * time.Millisecond},
		StateCheckChance:                  0.2,
		ServiceUnregisterHaltChance:       0.3,
		ServiceUnregisterUnexpectedChance: 0.3,

		Stats: NewStats(),
	})
}

func TestRunnerFuzzImpatient(t *testing.T) {
	// All wait times are zero. This should hopefully flush out some timing
	// bugs in the API like this one:
	// - Start() a service without Waiting or retaining
	// - Service ends with an error before calling Ready(), causing
	//   the runner to remove references to it.
	// - <-WhenReady() (old api) was called, but the error had been removed
	//   so instead of returning the Start() error, it was swallowed.
	//
	// This error was happening because the calling code just didn't make
	// it to WhenReady() in time. Hopefully this helps minimise the chances
	// of bugs like that slipping the net again.

	stats := NewStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0,
		RunnerHaltChance:   0,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0,
		GroupServiceRunFailureChance:   0,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceRunTime:     TimeRange{0, 0},
		ServiceHaltAfter:   TimeRange{0, 0},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{1 * time.Second, 1 * time.Second},

		ServiceRegisterBeforeStartChance:  0.1,
		ServiceRegisterAfterStartChance:   0.1,
		ServiceUnregisterHaltChance:       0.3,
		ServiceUnregisterUnexpectedChance: 0,

		StateCheckChance: 0.2,

		Stats: stats,
	})
}

func TestRunnerMetaFuzzInsanity(t *testing.T) {
	if !fuzzEnabled {
		t.Skip("skipping fuzz test")
	}

	// OK, we've fuzzed the Runner. Now let's fuzz fuzzing the runner.
	//            ,--.!,       ,--.!,       ,--.!,       ,--.!,
	//         __/   -*-    __/   -*-    __/   -*-    __/   -*-
	//       ,d08b.  '|`  ,d08b.  '|`  ,d08b.  '|`  ,d08b.  '|`
	//       0088MM       0088MM       0088MM       0088MM
	//       `9MMP'       `9MMP'       `9MMP'       `9MMP'
	rand.Seed(fuzzSeed)

	start := time.Now()
	dur := time.Duration(fuzzTimeDur)

	builder := &RunnerFuzzerBuilder{
		RunnerCreateChance: FloatRange{0.001, 0.02},
		RunnerHaltChance:   FloatRange{0, 0.001},

		ServiceCreateChance: FloatRange{0.001, 0.5},
		GroupCreateChance:   FloatRange{0.0, 0.5},
		GroupSize:           IntRangeMaker{IntRange{2, 3}, IntRange{3, 5}},

		ServiceStartFailureChance:      FloatRange{0, 0.01},
		ServiceRunFailureChance:        FloatRange{0, 0.01},
		GroupServiceStartFailureChance: FloatRange{0, 0.01},
		GroupServiceRunFailureChance:   FloatRange{0, 0.01},

		StartWaitChance:    FloatRange{0, 1},
		ServiceStartTime:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 10 * time.Millisecond}},
		StartWaitTimeout:   TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},
		ServiceRunTime:     TimeRangeMaker{TimeRange{100 * time.Microsecond, 100 * time.Millisecond}, TimeRange{1 * time.Second, 5 * time.Second}},
		ServiceHaltAfter:   TimeRangeMaker{TimeRange{10 * time.Microsecond, 1000 * time.Millisecond}, TimeRange{1 * time.Second, 2 * time.Second}},
		ServiceHaltDelay:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 0}},
		ServiceHaltTimeout: TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},

		StateCheckChance:                  FloatRange{0, 1},
		ServiceUnregisterHaltChance:       FloatRange{0, 1},
		ServiceUnregisterUnexpectedChance: FloatRange{0, 1},
	}

	iterDur := dur / time.Duration(fuzzMetaRolls)
	min := fuzzMetaMin
	i := 0

	for i < min || time.Since(start) < dur {
		t.Run("", func(t *testing.T) {
			stats := NewStats()
			stats.Seed = fuzzSeed

			tt := assert.WrapTB(t)
			fz := builder.Next(iterDur)
			if testing.Verbose() {
				e := json.NewEncoder(os.Stdout)
				e.SetIndent("", "  ")
				e.Encode(fz)
			}

			fz.Tick = time.Duration(fuzzTickNsec)
			fz.Stats = stats
			fz.Run(tt)
			if testing.Verbose() {
				fuzzOutput(fuzzOutputFormat, stats, os.Stdout)
			}
		})
		i++
	}
}

func testFuzz(t *testing.T, fz *RunnerFuzzer) {
	if !fuzzEnabled {
		t.Skip("skipping fuzz test")
	}

	if fz.RunnerLimit == 0 {
		fz.RunnerLimit = DefaultRunnerLimit
	}
	if fz.ServiceLimit == 0 {
		fz.ServiceLimit = DefaultServiceLimit
	}

	rand.Seed(fuzzSeed)
	fz.Stats.Seed = fuzzSeed

	setCurrentFuzzer(fz)

	dur := time.Duration(fuzzTimeDur)
	fz.Duration = dur
	fz.Run(assert.WrapTB(t))

	if testing.Verbose() {
		fuzzOutput(fuzzOutputFormat, fz.Stats, os.Stdout)
	}
}

type RunnerFuzzer struct {
	Duration     time.Duration
	Tick         time.Duration
	RunnerLimit  int
	ServiceLimit int

	SyncHalt bool

	RunnerCreateChance float64

	// This is a wonderful mess-maker. It will attempt, in a goroutine,
	// to halt every service it finds in a runner. It does NOT a happy
	// path make.
	RunnerHaltChance float64

	ServiceCreateChance       float64
	ServiceStartFailureChance float64
	ServiceRunFailureChance   float64
	StartWaitChance           float64

	// Chance the fuzzer will register a service just before it is started.
	// This should always succeed.
	ServiceRegisterBeforeStartChance float64

	// Chance the fuzzer will register a service just after it is started.
	// There is a chance this won't succeed if the service halts before
	// Register is called.
	ServiceRegisterAfterStartChance float64

	// Chance the fuzzer will attempt to register a random service at a random
	// moment.
	ServiceRegisterUnexpectedChance float64

	// Chance the service will be unregistered immediately after it is halted, but
	// only if it was registered by either ServiceRegisterBeforeStartChance or
	// ServiceRegisterAfterStartChance.
	ServiceUnregisterHaltChance float64

	// Chance the fuzzer will attempt to unregister a random service at a random
	// moment.
	ServiceUnregisterUnexpectedChance float64

	ServiceStartTime   TimeRange
	StartWaitTimeout   TimeRange
	ServiceRunTime     TimeRange
	ServiceHaltAfter   TimeRange
	ServiceHaltDelay   TimeRange
	ServiceHaltTimeout TimeRange
	StateCheckChance   float64

	GroupCreateChance              float64
	GroupSize                      IntRange
	GroupServiceStartFailureChance float64
	GroupServiceRunFailureChance   float64

	Stats *Stats `json:"-"`

	runners []Runner   `json:"-"`
	wg      *condGroup `json:"-"`

	services     []Service `json:"-"`
	servicesLock sync.Mutex
}

var (
	errStartFailure = errors.New("start failure")
	errRunFailure   = errors.New("run failure")
)

func (r *RunnerFuzzer) haltRunner() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	runner := r.runners[idx]

	// delete runner before we go off and halt it so we can keep the runners
	// list single threaded
	last := len(r.runners) - 1
	r.runners[idx], r.runners[last] = r.runners[last], nil
	r.runners = r.runners[:last]
	r.Stats.AddRunnersCurrent(-1)

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		// this can take a while so make sure it's done in a goroutine
		n, _ := runner.HaltAll(r.ServiceHaltTimeout.Rand(), 0)
		r.Stats.AddServicesCurrent(-n)
		r.Stats.AddRunnersHalted(1)
	}()
}

func (r *RunnerFuzzer) checkState() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	rn := r.runners[idx]

	if rr, ok := rn.(*runner); ok {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()

			svc := randomService(rr)
			if svc != nil {
				state := rr.State(svc)
				r.Stats.AddStateCheckResult(state)
			}
		}()
	}
}

func (r *RunnerFuzzer) unexpectedUnregister() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	rn := r.runners[idx]

	if rr, ok := rn.(*runner); ok {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()

			svc := randomService(rr)
			if svc != nil {
				_, err := rr.Unregister(svc)
				r.Stats.StatsForService(svc).ServiceUnregisterUnexpected.Add(err)
			}
		}()
	}
}

func (r *RunnerFuzzer) startRunner() {
	runner := NewRunner(r)
	r.Stats.AddRunnersCurrent(1)
	r.Stats.AddRunnersStarted(1)
	r.runners = append(r.runners, runner)
}

func (r *RunnerFuzzer) createService() {
	service := &dummyService{
		startDelay:   r.ServiceStartTime.Rand(),
		runTime:      r.ServiceRunTime.Rand(),
		haltDelay:    r.ServiceHaltDelay.Rand(),
		haltingSleep: true,
	}
	if should(r.ServiceStartFailureChance) {
		service.startFailure = errStartFailure
	} else if should(r.ServiceRunFailureChance) {
		service.runFailure = errRunFailure
	}
	r.runService(service, r.Stats.ServiceStats)
}

func (r *RunnerFuzzer) createGroup() {
	n := Name(fmt.Sprintf("%d", rand.Int()))

	var services []Service

	cs := r.GroupSize.Rand()
	r.Stats.AddGroupSize(cs)

	for i := 0; i < cs; i++ {
		service := &dummyService{
			startDelay:   r.ServiceStartTime.Rand(),
			runTime:      r.ServiceRunTime.Rand(),
			haltDelay:    r.ServiceHaltDelay.Rand(),
			haltingSleep: true,
		}
		if should(r.GroupServiceStartFailureChance) {
			service.startFailure = errStartFailure
		} else if should(r.GroupServiceRunFailureChance) {
			service.runFailure = errRunFailure
		}
		services = append(services, service)
	}

	group := NewGroup(n, services...)
	r.runService(group, r.Stats.GroupStats)
}

func (r *RunnerFuzzer) runService(service Service, stats *ServiceStats) {
	runner := r.runners[rand.Intn(r.Stats.GetRunnersCurrent())]

	r.servicesLock.Lock()
	r.services = append(r.services, service)
	r.servicesLock.Unlock()

	willRegisterBefore := should(r.ServiceRegisterBeforeStartChance)
	willRegisterAfter := false
	if !willRegisterBefore {
		willRegisterAfter = should(r.ServiceRegisterAfterStartChance)
	}
	willRegister := willRegisterBefore || willRegisterAfter

	var syncHalt chan struct{}
	if r.SyncHalt {
		syncHalt = make(chan struct{})
	}

	// After a while, we will halt the service, but only if it hasn't ended
	// first.
	// This needs to happen before we start the service so that it is possible
	// under certain configurations for the halt to happen before the start.
	r.wg.Add(1)
	time.AfterFunc(r.ServiceHaltAfter.Rand(), func() {
		defer r.wg.Done()

		if syncHalt != nil {
			<-syncHalt
		}

		err := runner.Halt(r.ServiceHaltTimeout.Rand(), service)
		stats.ServiceHalt.Add(err)

		if err == nil && willRegister {
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				if should(r.ServiceUnregisterHaltChance) {
					_, err := runner.Unregister(service)
					stats.ServiceUnregisterHalt.Add(err)
				}
			}()
		}
	})

	// Start the service
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		if willRegisterBefore {
			_ = runner.Register(service)
			stats.ServiceRegisterBeforeStart.Add(nil)
		}

		var err error
		if should(r.StartWaitChance) {
			err = runner.StartWait(r.StartWaitTimeout.Rand(), service)
			stats.ServiceStartWait.Add(err)
		} else {
			err = runner.Start(service, nil)
			stats.ServiceStart.Add(err)
		}

		if syncHalt != nil {
			close(syncHalt)
		}

		r.Stats.AddServicesCurrent(1)

		if willRegisterAfter {
			_ = runner.Register(service)
			stats.ServiceRegisterAfterStart.Add(nil)
		}
	}()
}

func (r *RunnerFuzzer) doTick() {
	// maybe halt a runnner, but never if it's the last.
	rcur := r.Stats.GetRunnersCurrent()
	if rcur > 1 && should(r.RunnerHaltChance) {
		r.haltRunner()
	}

	// maybe start a runner
	if rcur == 0 || r.Stats.GetTick() == 0 || (should(r.RunnerCreateChance)) && rcur < r.RunnerLimit {
		r.startRunner()
	}

	// maybe start a service into one of the existing runners, chosen
	// at random
	scur := r.Stats.GetServicesCurrent()
	if should(r.ServiceCreateChance) && scur < r.ServiceLimit {
		r.createService()
	}

	// maybe start a group of services into one of the existing runners, chosen
	// at random
	if should(r.GroupCreateChance) && scur < r.ServiceLimit {
		r.createGroup()
	}

	// maybe check the state of a randomly chosen service
	if should(r.StateCheckChance) {
		r.checkState()
	}

	// maybe check the state of a randomly chosen service
	if should(r.ServiceUnregisterUnexpectedChance) {
		r.unexpectedUnregister()
	}

	r.Stats.AddTick()
}

func (r *RunnerFuzzer) Run(tt assert.T) {
	tt.Helper()
	r.wg = newCondGroup()

	r.Stats.Start()

	if r.Tick == 0 {
		r.hotLoop()
	} else {
		r.tickLoop()
	}

	r.wg.Wait()

	// OK now we gotta clean up after ourselves.
	timeout := 2*time.Second + (r.ServiceHaltDelay.Max * 10)
	for _, rn := range r.runners {
		n, err := rn.HaltAll(timeout, 0)
		r.Stats.AddServicesCurrent(-n)
		tt.MustOK(err)
	}

	// Need to wait for any stray halt delays - the above HaltAll
	// call may report everything has halted, but it is skipping
	// things that are already Halting and not blocking to wait for them.
	// That may not be ideal, perhaps it should be fixed.
	time.Sleep(r.ServiceHaltDelay.Max)
}

func (r *RunnerFuzzer) Service(service Service) *ServiceStats {
	switch service.(type) {
	case *Group:
		return r.Stats.GroupStats
	default:
		return r.Stats.ServiceStats
	}
}

func (r *RunnerFuzzer) OnServiceError(service Service, err Error) {
	switch service.(type) {
	case *Group:
		r.Stats.GroupStats.AddServiceError(err)
	default:
		r.Stats.ServiceStats.AddServiceError(err)
	}
}

func (r *RunnerFuzzer) OnServiceEnd(stage Stage, service Service, err Error) {
	var s *ServiceStats
	switch service.(type) {
	case *Group:
		s = r.Stats.GroupStats
	default:
		s = r.Stats.ServiceStats
	}
	s.AddServiceEnd(err)

	r.Stats.AddServicesCurrent(-1)
}

func (r *RunnerFuzzer) OnServiceState(service Service, state State) {}

func (r *RunnerFuzzer) hotLoop() {
	start := time.Now()
	for time.Since(start) < r.Duration {
		r.doTick()
	}
}

func randomService(rr *runner) Service {
	rr.statesLock.Lock()
	var s Service
	// FIXME: relies on unspecified random map iteration
	for s = range rr.states {
		break
	}
	rr.statesLock.Unlock()
	return s
}

func randomServices(rr *runner, n int) []Service {
	rr.statesLock.Lock()
	out := make([]Service, 0, n)

	i := 0
	// FIXME: relies on unspecified random map iteration
	for s := range rr.states {
		out = append(out, s)
		i++
		if i >= n {
			break
		}
	}
	rr.statesLock.Unlock()
	return out
}

type RunnerFuzzerBuilder struct {
	StateCheckChance FloatRange

	RunnerCreateChance FloatRange
	RunnerHaltChance   FloatRange

	ServiceCreateChance FloatRange
	GroupCreateChance   FloatRange
	GroupSize           IntRangeMaker

	GroupServiceStartFailureChance FloatRange
	GroupServiceRunFailureChance   FloatRange

	StartWaitChance           FloatRange
	ServiceStartFailureChance FloatRange
	ServiceRunFailureChance   FloatRange

	ServiceUnregisterHaltChance       FloatRange
	ServiceUnregisterUnexpectedChance FloatRange

	ServiceStartTime   TimeRangeMaker
	StartWaitTimeout   TimeRangeMaker
	ServiceRunTime     TimeRangeMaker
	ServiceHaltAfter   TimeRangeMaker
	ServiceHaltDelay   TimeRangeMaker
	ServiceHaltTimeout TimeRangeMaker
}

func (f RunnerFuzzerBuilder) Next(dur time.Duration) *RunnerFuzzer {
	return &RunnerFuzzer{
		Duration:                          dur,
		GroupCreateChance:                 f.GroupCreateChance.Rand(),
		GroupServiceRunFailureChance:      f.GroupServiceRunFailureChance.Rand(),
		GroupServiceStartFailureChance:    f.GroupServiceStartFailureChance.Rand(),
		GroupSize:                         f.GroupSize.Rand(),
		RunnerCreateChance:                f.RunnerCreateChance.Rand(),
		RunnerHaltChance:                  f.RunnerHaltChance.Rand(),
		RunnerLimit:                       DefaultRunnerLimit,
		ServiceCreateChance:               f.ServiceCreateChance.Rand(),
		ServiceHaltAfter:                  f.ServiceHaltAfter.Rand(),
		ServiceHaltDelay:                  f.ServiceHaltDelay.Rand(),
		ServiceHaltTimeout:                f.ServiceHaltTimeout.Rand(),
		ServiceLimit:                      DefaultServiceLimit,
		ServiceRunFailureChance:           f.ServiceRunFailureChance.Rand(),
		ServiceRunTime:                    f.ServiceRunTime.Rand(),
		ServiceStartFailureChance:         f.ServiceStartFailureChance.Rand(),
		ServiceStartTime:                  f.ServiceStartTime.Rand(),
		ServiceUnregisterHaltChance:       f.ServiceUnregisterHaltChance.Rand(),
		ServiceUnregisterUnexpectedChance: f.ServiceUnregisterUnexpectedChance.Rand(),
		StartWaitChance:                   f.StartWaitChance.Rand(),
		StartWaitTimeout:                  f.StartWaitTimeout.Rand(),
		StateCheckChance:                  f.StateCheckChance.Rand(),
	}
}

type Stats struct {
	Duration        time.Duration
	Seed            int64
	Tick            int32
	RunnersStarted  int32
	RunnersCurrent  int32
	RunnersHalted   int32
	ServicesCurrent int32

	ServiceStats *ServiceStats
	GroupStats   *ServiceStats

	StateCheckResults     map[State]int
	stateCheckResultsLock sync.Mutex

	GroupSizes     map[int]int
	groupSizesLock sync.Mutex
	start          time.Time
}

func NewStats() *Stats {
	return &Stats{
		GroupStats:        NewServiceStats(),
		ServiceStats:      NewServiceStats(),
		GroupSizes:        make(map[int]int),
		StateCheckResults: make(map[State]int),
	}
}

func (s *Stats) StatsForService(svc Service) *ServiceStats {
	switch svc.(type) {
	case *Group:
		return s.GroupStats
	default:
		return s.ServiceStats
	}
}

func (s *Stats) Starts() int {
	return s.ServiceStats.ServiceStart.Total() +
		s.ServiceStats.ServiceStartWait.Total() +
		s.GroupStats.ServiceStart.Total() +
		s.GroupStats.ServiceStartWait.Total()
}

func (s *Stats) Ends() int {
	return int(s.ServiceStats.ServiceEnded()) + int(s.GroupStats.ServiceEnded())
}

func (s *Stats) Start() {
	s.start = time.Now()
	atomic.StoreInt32(&s.RunnersCurrent, 0)
	atomic.StoreInt32(&s.Tick, 0)
}

func (s *Stats) GetServicesCurrent() int  { return int(atomic.LoadInt32(&s.ServicesCurrent)) }
func (s *Stats) AddServicesCurrent(n int) { atomic.AddInt32(&s.ServicesCurrent, int32(n)) }

func (s *Stats) GetTick() int { return int(atomic.LoadInt32(&s.Tick)) }
func (s *Stats) AddTick()     { atomic.AddInt32(&s.Tick, 1) }

func (s *Stats) GetRunnersCurrent() int  { return int(atomic.LoadInt32(&s.RunnersCurrent)) }
func (s *Stats) AddRunnersCurrent(n int) { atomic.AddInt32(&s.RunnersCurrent, int32(n)) }

func (s *Stats) GetRunnersStarted() int  { return int(atomic.LoadInt32(&s.RunnersStarted)) }
func (s *Stats) AddRunnersStarted(n int) { atomic.AddInt32(&s.RunnersStarted, int32(n)) }

func (s *Stats) GetRunnersHalted() int  { return int(atomic.LoadInt32(&s.RunnersHalted)) }
func (s *Stats) AddRunnersHalted(n int) { atomic.AddInt32(&s.RunnersHalted, int32(n)) }

func (s *Stats) AddGroupSize(size int) {
	s.groupSizesLock.Lock()
	s.GroupSizes[size]++
	s.groupSizesLock.Unlock()
}

func (s *Stats) AddStateCheckResult(state State) {
	s.stateCheckResultsLock.Lock()
	s.StateCheckResults[state]++
	s.stateCheckResultsLock.Unlock()
}

func (s *Stats) Map() map[string]interface{} {
	return map[string]interface{}{
		"Seed":            s.Seed,
		"Tick":            s.GetTick(),
		"RunnersCurrent":  s.GetRunnersCurrent(),
		"RunnersStarted":  s.GetRunnersStarted(),
		"RunnersHalted":   s.GetRunnersHalted(),
		"ServicesCurrent": s.GetServicesCurrent(),
		"ServiceStats":    s.ServiceStats.Map(),
		"GroupStats":      s.GroupStats.Map(),
	}
}

func (s *Stats) MarshalJSON() ([]byte, error) {
	// Strip off the methods before marshalling to avoid errant recursion:
	type stats Stats
	ss := (*stats)(s)

	bts, err := json.Marshal(ss)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	if err := json.Unmarshal(bts, &m); err != nil {
		return nil, err
	}
	m["Starts"] = s.Starts()
	m["Ends"] = s.Ends()

	return json.Marshal(m)
}

func (s *Stats) Clone() *Stats {
	n := NewStats()
	n.Duration = time.Since(s.start)
	n.Seed = s.Seed
	n.Tick = int32(s.GetTick())
	n.RunnersCurrent = int32(s.GetRunnersCurrent())
	n.RunnersStarted = int32(s.GetRunnersStarted())
	n.RunnersHalted = int32(s.GetRunnersHalted())
	n.ServicesCurrent = int32(s.GetServicesCurrent())

	n.ServiceStats = s.ServiceStats.Clone()
	n.GroupStats = s.GroupStats.Clone()

	s.groupSizesLock.Lock()
	for m, c := range s.GroupSizes {
		n.GroupSizes[m] = c
	}
	s.groupSizesLock.Unlock()

	s.stateCheckResultsLock.Lock()
	for m, c := range s.StateCheckResults {
		n.StateCheckResults[m] = c
	}
	s.stateCheckResultsLock.Unlock()

	return n
}

type ServiceStats struct {
	serviceErrors     map[string]int
	serviceErrorsLock sync.RWMutex

	serviceEnds     map[string]int
	serviceEnded    int
	serviceEndsLock sync.RWMutex

	ServiceHalt                 *ErrorCounter
	ServiceStart                *ErrorCounter
	ServiceStartWait            *ErrorCounter
	ServiceUnregisterHalt       *ErrorCounter
	ServiceUnregisterUnexpected *ErrorCounter
	ServiceRegisterBeforeStart  *ErrorCounter
	ServiceRegisterAfterStart   *ErrorCounter
}

func NewServiceStats() *ServiceStats {
	return &ServiceStats{
		serviceEnds:   make(map[string]int),
		serviceErrors: make(map[string]int),

		ServiceHalt:                 &ErrorCounter{},
		ServiceStart:                &ErrorCounter{},
		ServiceStartWait:            &ErrorCounter{},
		ServiceUnregisterHalt:       &ErrorCounter{},
		ServiceUnregisterUnexpected: &ErrorCounter{},
		ServiceRegisterBeforeStart:  &ErrorCounter{},
		ServiceRegisterAfterStart:   &ErrorCounter{},
	}
}

func (s *ServiceStats) Map() map[string]interface{} {
	return map[string]interface{}{
		"ServiceEnded":                          s.ServiceEnded(),
		"ServiceHalt.Succeeded":                 s.ServiceHalt.Succeeded(),
		"ServiceHalt.Failed":                    s.ServiceHalt.Failed(),
		"ServiceStart.Succeeded":                s.ServiceStart.Succeeded(),
		"ServiceStart.Failed":                   s.ServiceStart.Failed(),
		"ServiceStartWait.Succeeded":            s.ServiceStartWait.Succeeded(),
		"ServiceStartWait.Failed":               s.ServiceStartWait.Failed(),
		"ServiceUnregisterHalt.Succeeded":       s.ServiceUnregisterHalt.Succeeded(),
		"ServiceUnregisterHalt.Failed":          s.ServiceUnregisterHalt.Failed(),
		"ServiceUnregisterUnexpected.Succeeded": s.ServiceUnregisterUnexpected.Succeeded(),
		"ServiceUnregisterUnexpected.Failed":    s.ServiceUnregisterUnexpected.Failed(),
		"ServiceRegisterBeforeStart.Succeeded":  s.ServiceRegisterBeforeStart.Succeeded(),
		"ServiceRegisterBeforeStart.Failed":     s.ServiceRegisterBeforeStart.Failed(),
		"ServiceRegisterAfterStart.Succeeded":   s.ServiceRegisterAfterStart.Succeeded(),
		"ServiceRegisterAfterStart.Failed":      s.ServiceRegisterAfterStart.Failed(),
	}
}

func (s *ServiceStats) ServiceEnded() (out int) {
	s.serviceEndsLock.RLock()
	out = s.serviceEnded
	s.serviceEndsLock.RUnlock()
	return out
}

func (s *ServiceStats) AddServiceEnd(err error) {
	s.serviceEndsLock.Lock()
	s.serviceEnded++
	for _, msg := range fuzzErrs(err) {
		s.serviceEnds[msg]++
	}
	s.serviceEndsLock.Unlock()
}

func (s *ServiceStats) AddServiceError(err error) {
	s.serviceErrorsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.serviceErrors[msg]++
	}
	s.serviceErrorsLock.Unlock()
}

func (s *ServiceStats) Clone() *ServiceStats {
	n := NewServiceStats()

	n.ServiceHalt = s.ServiceHalt.Clone()
	n.ServiceStart = s.ServiceStart.Clone()
	n.ServiceStartWait = s.ServiceStartWait.Clone()
	n.ServiceUnregisterHalt = s.ServiceUnregisterHalt.Clone()
	n.ServiceUnregisterUnexpected = s.ServiceUnregisterUnexpected.Clone()
	n.ServiceRegisterAfterStart = s.ServiceRegisterAfterStart.Clone()
	n.ServiceRegisterBeforeStart = s.ServiceRegisterBeforeStart.Clone()

	s.serviceEndsLock.Lock()
	n.serviceEnded = s.serviceEnded
	for m, c := range s.serviceEnds {
		n.serviceEnds[m] = c
	}
	s.serviceEndsLock.Unlock()

	s.serviceErrorsLock.Lock()
	for m, c := range s.serviceErrors {
		n.serviceErrors[m] = c
	}
	s.serviceErrorsLock.Unlock()

	return n
}

func listErrs(err error) (out []error) {
	if err == nil {
		return nil
	}
	c := cause(err)
	if c != nil && c != err {
		err = c
	}

	if grp, ok := err.(errorGroup); ok {
		for _, e := range grp.Errors() {
			out = append(out, listErrs(e)...)
		}
	} else {
		out = append(out, err)
	}

	return
}

func fuzzErrs(err error) (out []string) {
	for _, e := range listErrs(err) {
		out = append(out, e.Error())
	}
	return
}

func randDuration(min, max time.Duration) time.Duration {
	if min == 0 && max == 0 {
		return 0
	} else if min == max {
		return min
	}
	return time.Duration(rand.Int63n(int64(max)-int64(min))) + min
}

type TimeRange struct {
	Min time.Duration
	Max time.Duration
}

func (t TimeRange) Rand() time.Duration {
	return randDuration(t.Min, t.Max)
}

type TimeRangeMaker struct {
	Min TimeRange
	Max TimeRange
}

func (t TimeRangeMaker) Rand() TimeRange {
	return TimeRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

type IntRange struct {
	Min int
	Max int
}

func (t IntRange) Rand() int {
	return rand.Intn((t.Max+1)-t.Min) + t.Min
}

type IntRangeMaker struct {
	Min IntRange
	Max IntRange
}

func (t IntRangeMaker) Rand() IntRange {
	return IntRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

type FloatRange struct {
	Min float64
	Max float64
}

func (t FloatRange) Rand() float64 {
	f := rand.Float64()
	top := (t.Max - t.Min) * f
	return top + t.Min
}

type FloatRangeMaker struct {
	Min FloatRange
	Max FloatRange
}

func (t FloatRangeMaker) Rand() FloatRange {
	return FloatRange{Min: t.Min.Rand(), Max: t.Max.Rand()}
}

func should(chance float64) bool {
	if chance <= 0 {
		return false
	} else if chance >= 1 {
		return true
	}
	max := uint64(1000000)
	next := float64(rand.Uint64() % max)
	return next < (chance * float64(max))
}

// condGroup implements a less volatile, more general-purpose waitgroup than
// sync.WaitGroup.
//
// Unlike sync.WaitGroup, new Add calls can occur before all previous waits
// have returned.
//
// This is copy-pastad in from golib, don't export it.
//
type condGroup struct {
	count int
	lock  sync.Mutex
	cond  *sync.Cond
}

func newCondGroup() *condGroup {
	wg := &condGroup{}
	wg.cond = sync.NewCond(&wg.lock)
	return wg
}

func (wg *condGroup) Stop() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count = 0
	wg.cond.Broadcast()
}

func (wg *condGroup) Count() int {
	wg.lock.Lock()
	defer wg.lock.Unlock()
	return wg.count
}

func (wg *condGroup) Done() { wg.Add(-1) }

func (wg *condGroup) Add(n int) {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	wg.count += n
	if wg.count < 0 {
		panic(fmt.Errorf("negative waitgroup counter: %d", wg.count))
	}
	if wg.count == 0 {
		wg.cond.Broadcast()
	}
}

func (wg *condGroup) Wait() {
	wg.lock.Lock()
	defer wg.lock.Unlock()

	for {
		if wg.count > 0 {
			wg.cond.Wait()
		} else {
			return
		}
	}
}

type ErrorCounter struct {
	succeeded int
	failed    int
	errors    map[string]int
	lock      sync.RWMutex
}

func (e *ErrorCounter) MarshalJSON() ([]byte, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	out := map[string]interface{}{}
	if e.succeeded > 0 {
		out["Succeeded"] = e.succeeded
	}
	if e.failed > 0 {
		out["Failed"] = e.failed
	}
	if len(e.errors) > 0 {
		out["Errors"] = e.errors
	}
	return json.Marshal(out)
}

func (e *ErrorCounter) Total() (out int) {
	e.lock.RLock()
	out = e.succeeded + e.failed
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Succeeded() (out int) {
	e.lock.RLock()
	out = e.succeeded
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Failed() (out int) {
	e.lock.RLock()
	out = e.failed
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Percent() (out float64) {
	e.lock.RLock()
	total := e.succeeded + e.failed
	if total > 0 {
		out = float64(e.succeeded) / float64(total) * 100
	}
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Clone() *ErrorCounter {
	e.lock.RLock()
	ec := &ErrorCounter{
		succeeded: e.succeeded,
		failed:    e.failed,
		errors:    make(map[string]int, len(e.errors)),
	}
	for k, v := range e.errors {
		ec.errors[k] = v
	}
	e.lock.RUnlock()
	return ec
}

func (e *ErrorCounter) Add(err error) {
	e.lock.Lock()
	if err == nil {
		e.succeeded++
	} else {
		e.failed++
		if e.errors == nil {
			e.errors = make(map[string]int)
		}
		for _, msg := range fuzzErrs(err) {
			e.errors[msg]++
		}
	}
	e.lock.Unlock()
}

const debugWithin = true

func mustWithin(tt assert.T, tolerance float64, ratio float64, a interface{}, b interface{}) {
	tt.Helper()
	var afl, bfl = coerceFloat(a), coerceFloat(b)
	var min, max = ratio - (ratio * tolerance), ratio + (ratio * tolerance)
	if debugWithin && testing.Verbose() {
		fmt.Printf("a / b = %f, expected %f < n < %f\n", afl/bfl, min, max)
	}
	tt.MustAssert((afl/bfl) >= min && (afl/bfl) <= max, "a / b = %f, expected %f < n < %f", afl/bfl, min, max)
}

func coerceFloat(n interface{}) float64 {
	switch a := n.(type) {
	case int:
		return float64(a)
	case int8:
		return float64(a)
	case int16:
		return float64(a)
	case int32:
		return float64(a)
	case int64:
		return float64(a)
	case uint:
		return float64(a)
	case uint8:
		return float64(a)
	case uint16:
		return float64(a)
	case uint32:
		return float64(a)
	case uint64:
		return float64(a)
	case float64:
		return float64(a)
	case float32:
		return float64(a)
	default:
		panic("cannot coerce to float")
	}
}
