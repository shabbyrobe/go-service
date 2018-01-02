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

	_ "expvar"

	"github.com/shabbyrobe/golib/assert"
	"github.com/shabbyrobe/golib/errtools"
)

// TODO:
// - should attempt to restart certain services
// - should attempt to unregister services

func TestRunnerFuzzEverythingHappy(t *testing.T) {
	// Happy config: should yield no errors
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.01,
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
		ServiceRunTime:     TimeRange{5 * time.Second, 5 * time.Second},
		ServiceHaltAfter:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{1 * time.Second, 1 * time.Second},
		Stats:              NewStats(),
	})
}

func TestRunnerFuzzServiceHappy(t *testing.T) {
	// Happy config: should yield no errors
	testFuzz(t, &RunnerFuzzer{
		Tick:                      time.Duration(fuzzTickNsec),
		RunnerCreateChance:        0.01,
		RunnerHaltChance:          0,
		ServiceCreateChance:       0.2,
		StartWaitChance:           0.2,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,
		ServiceStartTime:          TimeRange{0, 0},
		StartWaitTimeout:          TimeRange{1 * time.Second, 1 * time.Second},
		ServiceRunTime:            TimeRange{5 * time.Second, 5 * time.Second},
		ServiceHaltAfter:          TimeRange{1 * time.Second, 1 * time.Second},
		ServiceHaltDelay:          TimeRange{0, 0},
		ServiceHaltTimeout:        TimeRange{1 * time.Second, 1 * time.Second},
		Stats:                     NewStats(),
	})
}

func TestRunnerFuzzGroupHappy(t *testing.T) {
	// Happy config: should yield no errors
	testFuzz(t, &RunnerFuzzer{
		Tick:                time.Duration(fuzzTickNsec),
		RunnerCreateChance:  0.01,
		RunnerHaltChance:    0,
		ServiceCreateChance: 0,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0,
		GroupServiceRunFailureChance:   0,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceRunTime:     TimeRange{5 * time.Second, 5 * time.Second},
		ServiceHaltAfter:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{1 * time.Second, 1 * time.Second},
		Stats:              NewStats(),
	})
}

func TestRunnerFuzzServiceMessy(t *testing.T) {
	testFuzz(t, &RunnerFuzzer{
		Tick:                      time.Duration(fuzzTickNsec),
		RunnerCreateChance:        0.005,
		RunnerHaltChance:          0.001,
		ServiceCreateChance:       0.2,
		StartWaitChance:           0.2,
		ServiceStartFailureChance: 0.05,
		ServiceRunFailureChance:   0.05,
		ServiceStartTime:          TimeRange{0, 21 * time.Millisecond},
		StartWaitTimeout:          TimeRange{20 * time.Millisecond, 1 * time.Second},
		ServiceRunTime:            TimeRange{0, 500 * time.Millisecond},
		ServiceHaltAfter:          TimeRange{0, 500 * time.Millisecond},
		ServiceHaltDelay:          TimeRange{0, 10 * time.Millisecond},
		ServiceHaltTimeout:        TimeRange{9 * time.Millisecond, 10 * time.Millisecond},
		Stats:                     NewStats(),
	})
}

func TestRunnerFuzzGroupMessy(t *testing.T) {
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.005,
		RunnerHaltChance:   0.001,

		ServiceCreateChance:            0,
		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.01,
		GroupServiceRunFailureChance:   0.01,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 21 * time.Millisecond},
		StartWaitTimeout:   TimeRange{300 * time.Millisecond, 1 * time.Second},
		ServiceRunTime:     TimeRange{0, 500 * time.Millisecond},
		ServiceHaltAfter:   TimeRange{0, 500 * time.Millisecond},
		ServiceHaltDelay:   TimeRange{0, 100 * time.Millisecond},
		ServiceHaltTimeout: TimeRange{99 * time.Millisecond, 100 * time.Millisecond},
		Stats:              NewStats(),
	})
}

func TestRunnerFuzzEverythingOutrage(t *testing.T) {
	// Pathological configuration - should fail far more often than it succeeds,
	// but should not leave any stray crap lying around.
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.02,
		RunnerHaltChance:   0.01,

		ServiceCreateChance:       0.3,
		ServiceStartFailureChance: 0.1,
		ServiceRunFailureChance:   0.2,

		GroupCreateChance:              0.2,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.05,
		GroupServiceRunFailureChance:   0.05,

		StartWaitChance:    0.2,
		ServiceStartTime:   TimeRange{0, 50 * time.Millisecond},
		StartWaitTimeout:   TimeRange{0, 50 * time.Millisecond},
		ServiceRunTime:     TimeRange{0, 50 * time.Millisecond},
		ServiceHaltAfter:   TimeRange{0, 50 * time.Millisecond},
		ServiceHaltDelay:   TimeRange{0, 50 * time.Millisecond},
		ServiceHaltTimeout: TimeRange{0, 50 * time.Millisecond},
		Stats:              NewStats(),
	})
}

func TestRunnerFuzzServiceOutrage(t *testing.T) {
	// Pathological configuration - should fail far more often than it succeeds,
	// but should not leave any stray crap lying around.
	testFuzz(t, &RunnerFuzzer{
		Tick:                      time.Duration(fuzzTickNsec),
		RunnerCreateChance:        0.02,
		RunnerHaltChance:          0.01,
		ServiceCreateChance:       0.3,
		StartWaitChance:           0.2,
		ServiceStartFailureChance: 0.1,
		ServiceRunFailureChance:   0.2,
		ServiceStartTime:          TimeRange{0, 50 * time.Millisecond},
		StartWaitTimeout:          TimeRange{0, 50 * time.Millisecond},
		ServiceRunTime:            TimeRange{0, 50 * time.Millisecond},
		ServiceHaltAfter:          TimeRange{0, 50 * time.Millisecond},
		ServiceHaltDelay:          TimeRange{0, 50 * time.Millisecond},
		ServiceHaltTimeout:        TimeRange{0, 50 * time.Millisecond},
		Stats:                     NewStats(),
	})
}

func TestRunnerFuzzGroupOutrage(t *testing.T) {
	// Pathological configuration - should fail far more often than it succeeds,
	// but should not leave any stray crap lying around.
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.02,
		RunnerHaltChance:   0.01,

		ServiceCreateChance:            0,
		GroupCreateChance:              0.3,
		GroupSize:                      IntRange{2, 4},
		GroupServiceStartFailureChance: 0.05,
		GroupServiceRunFailureChance:   0.05,

		StartWaitChance:           0.2,
		ServiceStartFailureChance: 0.1,
		ServiceRunFailureChance:   0.2,
		ServiceStartTime:          TimeRange{0, 50 * time.Millisecond},
		StartWaitTimeout:          TimeRange{0, 50 * time.Millisecond},
		ServiceRunTime:            TimeRange{0, 50 * time.Millisecond},
		ServiceHaltAfter:          TimeRange{0, 50 * time.Millisecond},
		ServiceHaltDelay:          TimeRange{0, 50 * time.Millisecond},
		ServiceHaltTimeout:        TimeRange{0, 50 * time.Millisecond},
		Stats:                     NewStats(),
	})
}

func TestRunnerMetaFuzzInsanity(t *testing.T) {
	// OK, we've fuzzed the Runner. Now let's fuzz fuzzing the runner.
	//            ,--.!,       ,--.!,       ,--.!,       ,--.!,
	//         __/   -*-    __/   -*-    __/   -*-    __/   -*-
	//       ,d08b.  '|`  ,d08b.  '|`  ,d08b.  '|`  ,d08b.  '|`
	//       0088MM       0088MM       0088MM       0088MM
	//       `9MMP'       `9MMP'       `9MMP'       `9MMP'
	rand.Seed(fuzzSeed)

	start := time.Now()
	dur := time.Duration(fuzzTimeSec * float64(time.Second))

	builder := &RunnerFuzzerBuilder{
		GroupCreateChance:  FloatRange{0.00, 0.05},
		RunnerCreateChance: FloatRange{0.001, 0.02},
		RunnerHaltChance:   FloatRange{0, 0.001},

		ServiceCreateChance:       FloatRange{0.001, 0.05},
		ServiceStartFailureChance: FloatRange{0, 0.001},
		ServiceRunFailureChance:   FloatRange{0, 0.001},

		GroupSize:                      IntRangeMaker{IntRange{2, 3}, IntRange{3, 5}},
		GroupServiceStartFailureChance: FloatRange{0, 0.001},
		GroupServiceRunFailureChance:   FloatRange{0, 0.001},

		StartWaitChance:    FloatRange{0, 1},
		ServiceStartTime:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 10 * time.Millisecond}},
		StartWaitTimeout:   TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},
		ServiceRunTime:     TimeRangeMaker{TimeRange{100 * time.Microsecond, 100 * time.Millisecond}, TimeRange{1 * time.Second, 5 * time.Second}},
		ServiceHaltAfter:   TimeRangeMaker{TimeRange{10 * time.Microsecond, 1000 * time.Millisecond}, TimeRange{1 * time.Second, 2 * time.Second}},
		ServiceHaltDelay:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 0}},
		ServiceHaltTimeout: TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},
	}

	iterDur := dur / time.Duration(fuzzMetaRolls)

	for time.Since(start) < dur {
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
				e := json.NewEncoder(os.Stdout)
				e.SetIndent("", "  ")
				e.Encode(stats.Clone())
			}
		})
	}
}

func testFuzz(t *testing.T, fz *RunnerFuzzer) {
	if !fuzzEnabled {
		t.Skip("skipping fuzz test")
	}

	rand.Seed(fuzzSeed)
	fz.Stats.Seed = fuzzSeed

	dur := time.Duration(fuzzTimeSec * float64(time.Second))
	fz.Duration = dur
	fz.Run(assert.WrapTB(t))
	if testing.Verbose() {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		e.Encode(fz.Stats.Clone())
	}
}

type RunnerFuzzer struct {
	Duration time.Duration
	Tick     time.Duration

	RunnerCreateChance float64
	RunnerHaltChance   float64

	ServiceCreateChance       float64
	ServiceStartFailureChance float64
	ServiceRunFailureChance   float64
	StartWaitChance           float64

	ServiceStartTime   TimeRange
	StartWaitTimeout   TimeRange
	ServiceRunTime     TimeRange
	ServiceHaltAfter   TimeRange
	ServiceHaltDelay   TimeRange
	ServiceHaltTimeout TimeRange

	GroupCreateChance              float64
	GroupSize                      IntRange
	GroupServiceStartFailureChance float64
	GroupServiceRunFailureChance   float64

	Stats *Stats `json:"-"`

	runners []Runner   `json:"-"`
	wg      *CondGroup `json:"-"`
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
		runner.HaltAll(r.ServiceHaltTimeout.Rand())
		r.Stats.AddRunnersHalted(1)
	}()
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

	runner := r.runners[rand.Intn(r.Stats.GetRunnersCurrent())]

	r.scheduleHalt(runner, service, r.Stats.ServiceStats)
	r.startService(runner, service, r.Stats.ServiceStats)
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

	group := NewGroup(n, services)

	runner := r.runners[rand.Intn(r.Stats.GetRunnersCurrent())]

	r.scheduleHalt(runner, group, r.Stats.GroupStats)
	r.startService(runner, group, r.Stats.GroupStats)
}

func (r *RunnerFuzzer) startService(runner Runner, service Service, stats *ServiceStats) {
	if should(r.StartWaitChance) {
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			if err := runner.StartWait(service, r.StartWaitTimeout.Rand()); err != nil {
				stats.AddServiceStartWaitFailed(1)
				stats.AddServiceStartWaitError(err) // errtools.Cause(err).Error())
			} else {
				stats.AddServiceStartWaited(1)
			}
		}()
	} else {
		if err := runner.Start(service); err != nil {
			stats.AddServiceStartFailed(1)
			stats.AddServiceStartError(err)
		} else {
			stats.AddServiceStarted(1)
		}
	}
}

func (r *RunnerFuzzer) scheduleHalt(runner Runner, service Service, stats *ServiceStats) {
	r.wg.Add(1)

	// After a while, we will halt the service, but only if it hasn't ended
	// first.
	time.AfterFunc(r.ServiceHaltAfter.Rand(), func() {
		defer r.wg.Done()
		err := runner.Halt(service, r.ServiceHaltTimeout.Rand())
		if err != nil {
			stats.AddServiceHaltFailed(1)
			stats.AddServiceHaltError(err)
		} else {
			stats.AddServiceHalted(1)
		}
	})

}

func (r *RunnerFuzzer) doTick() {
	// maybe halt a runnner, but never if it's the last.
	if r.Stats.GetRunnersCurrent() > 1 && should(r.RunnerHaltChance) {
		r.haltRunner()
	}

	// maybe start a runner
	if r.Stats.GetTick() == 0 || should(r.RunnerCreateChance) {
		r.startRunner()
	}

	// maybe start a service into one of the existing runners, chosen
	// at random
	if should(r.ServiceCreateChance) {
		r.createService()
	}

	// maybe start a group of services into one of the existing runners, chosen
	// at random
	if should(r.GroupCreateChance) {
		r.createGroup()
	}

	r.Stats.AddTick()
}

func (r *RunnerFuzzer) Run(tt assert.T) {
	tt.Helper()
	r.wg = NewCondGroup()

	r.Stats.Start()

	if r.Tick < 50*time.Microsecond {
		r.hotLoop()
	} else {
		r.tickLoop()
	}

	r.wg.Wait()

	// OK now we gotta clean up after ourselves.
	for _, rn := range r.runners {
		tt.MustOK(rn.HaltAll(r.ServiceHaltDelay.Max * 10))
	}

	// Need to wait for any stray halt delays - the above HaltAll
	// call may report everything has halted, but it is skipping
	// things that are already Halting and not blocking to wait for them.
	// That may not be ideal, perhaps it should be fixed.
	time.Sleep(r.ServiceHaltDelay.Max)
}

func (r *RunnerFuzzer) OnServiceError(service Service, err Error) {
	switch service.(type) {
	case *Group:
		r.Stats.GroupStats.AddServiceError(err)
	default:
		r.Stats.ServiceStats.AddServiceError(err)
	}
}

func (r *RunnerFuzzer) OnServiceEnd(service Service, err Error) {
	if err != nil {
		var s *ServiceStats
		switch service.(type) {
		case *Group:
			s = r.Stats.GroupStats
		default:
			s = r.Stats.ServiceStats
		}
		s.AddServiceEnded(1)
		s.AddServiceEnd(err)
	}
}

func (r *RunnerFuzzer) OnServiceState(service Service, state State) {}

func (r *RunnerFuzzer) hotLoop() {
	start := time.Now()
	for time.Since(start) < r.Duration {
		r.doTick()
	}
}

func (r *RunnerFuzzer) tickLoop() {
	done := make(chan struct{})
	tick := time.NewTicker(r.Tick)
	end := time.After(r.Duration)

	go func() {
		for {
			select {
			case <-tick.C:
				r.doTick()
			case <-end:
				close(done)
				return
			}
		}
	}()
	<-done
}

type RunnerFuzzerBuilder struct {
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

	ServiceStartTime   TimeRangeMaker
	StartWaitTimeout   TimeRangeMaker
	ServiceRunTime     TimeRangeMaker
	ServiceHaltAfter   TimeRangeMaker
	ServiceHaltDelay   TimeRangeMaker
	ServiceHaltTimeout TimeRangeMaker
}

func (f RunnerFuzzerBuilder) Next(dur time.Duration) *RunnerFuzzer {
	return &RunnerFuzzer{
		Duration:                       dur,
		GroupCreateChance:              f.GroupCreateChance.Rand(),
		GroupServiceRunFailureChance:   f.GroupServiceRunFailureChance.Rand(),
		GroupServiceStartFailureChance: f.GroupServiceStartFailureChance.Rand(),
		GroupSize:                      f.GroupSize.Rand(),
		RunnerCreateChance:             f.RunnerCreateChance.Rand(),
		RunnerHaltChance:               f.RunnerHaltChance.Rand(),
		ServiceCreateChance:            f.ServiceCreateChance.Rand(),
		ServiceHaltAfter:               f.ServiceHaltAfter.Rand(),
		ServiceHaltDelay:               f.ServiceHaltDelay.Rand(),
		ServiceHaltTimeout:             f.ServiceHaltTimeout.Rand(),
		ServiceRunFailureChance:        f.ServiceRunFailureChance.Rand(),
		ServiceRunTime:                 f.ServiceRunTime.Rand(),
		ServiceStartFailureChance:      f.ServiceStartFailureChance.Rand(),
		ServiceStartTime:               f.ServiceStartTime.Rand(),
		StartWaitChance:                f.StartWaitChance.Rand(),
		StartWaitTimeout:               f.StartWaitTimeout.Rand(),
	}
}

type Stats struct {
	Seed           int64
	Tick           int32
	RunnersStarted int32
	RunnersCurrent int32
	RunnersHalted  int32

	ServiceStats *ServiceStats
	GroupStats   *ServiceStats

	GroupSizes     map[int]int
	groupSizesLock sync.Mutex
}

func NewStats() *Stats {
	return &Stats{
		GroupStats:   NewServiceStats(),
		ServiceStats: NewServiceStats(),
		GroupSizes:   make(map[int]int),
	}
}

func (s *Stats) Start() {
	atomic.StoreInt32(&s.RunnersCurrent, 0)
	atomic.StoreInt32(&s.Tick, 0)
}

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

func (s *Stats) Clone() *Stats {
	n := NewStats()
	n.Seed = s.Seed
	n.Tick = int32(s.GetTick())
	n.RunnersCurrent = int32(s.GetRunnersCurrent())
	n.RunnersStarted = int32(s.GetRunnersStarted())
	n.RunnersHalted = int32(s.GetRunnersHalted())

	n.ServiceStats = s.ServiceStats.Clone()
	n.GroupStats = s.GroupStats.Clone()

	s.groupSizesLock.Lock()
	for m, c := range s.GroupSizes {
		n.GroupSizes[m] = c
	}
	s.groupSizesLock.Unlock()

	return n
}

type ServiceStats struct {
	ServiceEnded           int32
	ServiceHalted          int32
	ServiceHaltFailed      int32
	ServiceStartWaitFailed int32
	ServiceStartWaited     int32
	ServiceStarted         int32
	ServiceStartFailed     int32

	ServiceErrors     map[string]int
	serviceErrorsLock sync.Mutex

	ServiceEnds     map[string]int
	serviceEndsLock sync.Mutex

	ServiceHaltErrors     map[string]int
	serviceHaltErrorsLock sync.Mutex

	ServiceStartErrors     map[string]int
	serviceStartErrorsLock sync.Mutex

	ServiceStartWaitErrors     map[string]int
	serviceStartWaitErrorsLock sync.Mutex
}

func NewServiceStats() *ServiceStats {
	return &ServiceStats{
		ServiceEnds:            make(map[string]int),
		ServiceErrors:          make(map[string]int),
		ServiceHaltErrors:      make(map[string]int),
		ServiceStartErrors:     make(map[string]int),
		ServiceStartWaitErrors: make(map[string]int),
	}
}

func (s *ServiceStats) GetServiceEnded() int  { return int(atomic.LoadInt32(&s.ServiceEnded)) }
func (s *ServiceStats) AddServiceEnded(n int) { atomic.AddInt32(&s.ServiceEnded, int32(n)) }

func (s *ServiceStats) GetServiceHalted() int  { return int(atomic.LoadInt32(&s.ServiceHalted)) }
func (s *ServiceStats) AddServiceHalted(n int) { atomic.AddInt32(&s.ServiceHalted, int32(n)) }

func (s *ServiceStats) GetServiceHaltFailed() int  { return int(atomic.LoadInt32(&s.ServiceHaltFailed)) }
func (s *ServiceStats) AddServiceHaltFailed(n int) { atomic.AddInt32(&s.ServiceHaltFailed, int32(n)) }

func (s *ServiceStats) GetServiceStarted() int  { return int(atomic.LoadInt32(&s.ServiceStarted)) }
func (s *ServiceStats) AddServiceStarted(n int) { atomic.AddInt32(&s.ServiceStarted, int32(n)) }

func (s *ServiceStats) GetServiceStartFailed() int {
	return int(atomic.LoadInt32(&s.ServiceStartFailed))
}
func (s *ServiceStats) AddServiceStartFailed(n int) { atomic.AddInt32(&s.ServiceStartFailed, int32(n)) }

func (s *ServiceStats) GetServiceStartWaited() int {
	return int(atomic.LoadInt32(&s.ServiceStartWaited))
}
func (s *ServiceStats) AddServiceStartWaited(n int) { atomic.AddInt32(&s.ServiceStartWaited, int32(n)) }

func (s *ServiceStats) GetServiceStartWaitFailed() int {
	return int(atomic.LoadInt32(&s.ServiceStartWaitFailed))
}
func (s *ServiceStats) AddServiceStartWaitFailed(n int) {
	atomic.AddInt32(&s.ServiceStartWaitFailed, int32(n))
}

func (s *ServiceStats) AddServiceEnd(err error) {
	s.serviceEndsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.ServiceEnds[msg]++
	}
	s.serviceEndsLock.Unlock()
}

func (s *ServiceStats) AddServiceError(err error) {
	s.serviceErrorsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.ServiceErrors[msg]++
	}
	s.serviceErrorsLock.Unlock()
}

func (s *ServiceStats) AddServiceHaltError(err error) {
	s.serviceHaltErrorsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.ServiceHaltErrors[msg]++
	}
	s.serviceHaltErrorsLock.Unlock()
}

func (s *ServiceStats) AddServiceStartError(err error) {
	s.serviceStartErrorsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.ServiceStartErrors[msg]++
	}
	s.serviceStartErrorsLock.Unlock()
}

func (s *ServiceStats) AddServiceStartWaitError(err error) {
	s.serviceStartWaitErrorsLock.Lock()
	for _, msg := range fuzzErrs(err) {
		s.ServiceStartWaitErrors[msg]++
	}
	s.serviceStartWaitErrorsLock.Unlock()
}

func (s *ServiceStats) Clone() *ServiceStats {
	n := NewServiceStats()
	n.ServiceEnded = int32(s.GetServiceEnded())
	n.ServiceHalted = int32(s.GetServiceHalted())
	n.ServiceHaltFailed = int32(s.GetServiceHaltFailed())
	n.ServiceStarted = int32(s.GetServiceStarted())
	n.ServiceStartFailed = int32(s.GetServiceStartFailed())
	n.ServiceStartWaited = int32(s.GetServiceStartWaited())
	n.ServiceStartWaitFailed = int32(s.GetServiceStartWaitFailed())

	s.serviceEndsLock.Lock()
	for m, c := range s.ServiceEnds {
		n.ServiceEnds[m] = c
	}
	s.serviceEndsLock.Unlock()

	s.serviceErrorsLock.Lock()
	for m, c := range s.ServiceErrors {
		n.ServiceErrors[m] = c
	}
	s.serviceErrorsLock.Unlock()

	s.serviceHaltErrorsLock.Lock()
	for m, c := range s.ServiceHaltErrors {
		n.ServiceHaltErrors[m] = c
	}
	s.serviceHaltErrorsLock.Unlock()

	s.serviceStartErrorsLock.Lock()
	for m, c := range s.ServiceStartErrors {
		n.ServiceStartErrors[m] = c
	}
	s.serviceStartErrorsLock.Unlock()

	s.serviceStartWaitErrorsLock.Lock()
	for m, c := range s.ServiceStartWaitErrors {
		n.ServiceStartWaitErrors[m] = c
	}
	s.serviceStartWaitErrorsLock.Unlock()
	return n
}

func listErrs(err error) (out []error) {
	c := errtools.Cause(err)
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
