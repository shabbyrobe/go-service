package servicetest

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

type RunnerFuzzer struct {
	RunnerSource RunnerSource

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

	// Chance that the fuzzer will attempt to restart a random service at a
	// random moment.
	ServiceRestartChance float64

	// Chance that a service started by the fuzzer will be restartable.
	ServiceRestartableChance float64

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

	Stats *FuzzStats `json:"-"`

	runners []service.Runner `json:"-"`
	wg      *condGroup       `json:"-"`

	services     []service.Service `json:"-"`
	servicesLock sync.Mutex
}

var (
	errStartFailure = errors.New("start failure")
	errRunFailure   = errors.New("run failure")
)

func (r *RunnerFuzzer) startRunner() {
	if !r.RunnerSource.CanCreateRunner() {
		return
	}

	runner := r.RunnerSource.CreateRunner(r)
	r.Stats.AddRunnersCurrent(1)
	r.Stats.AddRunnersStarted(1)
	r.runners = append(r.runners, runner)
}

func (r *RunnerFuzzer) haltRunner() {
	if !r.RunnerSource.CanHaltRunner() {
		return
	}

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

	if rr, ok := rn.(service.Runner); ok {
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
	if rn == nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		svc := randomService(rn)
		if svc != nil {
			_, err := rn.Unregister(svc)
			r.Stats.StatsForService(svc).ServiceUnregisterUnexpected.Add(err)
		}
	}()
}

func (r *RunnerFuzzer) restartService() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	rn := r.runners[idx]
	if rn == nil {
		return
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		svc := randomService(rn)
		if svc != nil {
			if err := service.EnsureHalt(rn, r.ServiceHaltTimeout.Rand(), svc); err != nil {
				fmt.Println(err)
				r.Stats.StatsForService(svc).ServiceRestart.Add(err)

			} else {
				err := rn.Start(svc, nil)

				// FIXME: the fuzzer should get its stats from the state listener,
				// not from the ended listener. These hacks are all in here because
				// the fuzzer can only dynamically react to a service ending, but
				// not when a service is starting.
				if err == nil {
					r.Stats.StatsForService(svc).ServiceStart.Add(nil)
					r.Stats.AddServicesCurrent(1)
				}

				r.Stats.StatsForService(svc).ServiceRestart.Add(err)
			}
		}
	}()
}

func (r *RunnerFuzzer) createService() {
	service := (&TimedService{
		StartDelay: r.ServiceStartTime.Rand(),
		RunTime:    r.ServiceRunTime.Rand(),
		HaltDelay:  r.ServiceHaltDelay.Rand(),
	}).Init()

	service.StartLimit = 1
	if should(r.ServiceRestartableChance) {
		service.StartLimit = 0
	}

	if should(r.ServiceStartFailureChance) {
		service.StartFailure = errStartFailure
	} else if should(r.ServiceRunFailureChance) {
		service.RunFailure = errRunFailure
	}
	r.runService(service, r.Stats.ServiceStats)
}

func (r *RunnerFuzzer) createGroup() {
	n := service.Name(fmt.Sprintf("%d", rand.Int()))

	var services []service.Service

	cs := r.GroupSize.Rand()
	r.Stats.AddGroupSize(cs)

	for i := 0; i < cs; i++ {
		service := (&TimedService{
			StartDelay: r.ServiceStartTime.Rand(),
			RunTime:    r.ServiceRunTime.Rand(),
			HaltDelay:  r.ServiceHaltDelay.Rand(),
		}).Init()

		if should(r.GroupServiceStartFailureChance) {
			service.StartFailure = errStartFailure
		} else if should(r.GroupServiceRunFailureChance) {
			service.RunFailure = errRunFailure
		}
		services = append(services, service)
	}

	group := service.NewGroup(n, services...)

	if !should(r.ServiceRestartableChance) {
		group.Restartable(false)
	}

	r.runService(group, r.Stats.GroupStats)
}

func (r *RunnerFuzzer) runService(svc service.Service, stats *FuzzServiceStats) {
	runner := r.runners[rand.Intn(r.Stats.GetRunnersCurrent())]

	r.servicesLock.Lock()
	r.services = append(r.services, svc)
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

		err := runner.Halt(r.ServiceHaltTimeout.Rand(), svc)
		stats.ServiceHalt.Add(err)

		if err == nil && willRegister {
			r.wg.Add(1)
			go func() {
				defer r.wg.Done()
				if should(r.ServiceUnregisterHaltChance) {
					_, err := runner.Unregister(svc)
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
			_ = runner.Register(svc)
			stats.ServiceRegisterBeforeStart.Add(nil)
		}

		var err error
		if should(r.StartWaitChance) {
			err = runner.StartWait(r.StartWaitTimeout.Rand(), svc)
			stats.ServiceStartWait.Add(err)
		} else {
			err = runner.Start(svc, nil)
			stats.ServiceStart.Add(err)
		}

		if syncHalt != nil {
			close(syncHalt)
		}

		r.Stats.AddServicesCurrent(1)

		if willRegisterAfter {
			_ = runner.Register(svc)
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

	// maybe restart a random service
	if should(r.ServiceRestartChance) {
		r.restartService()
	}

	r.Stats.AddTick()
}

func (r *RunnerFuzzer) Init(tt assert.T) {
	tt.Helper()
	if r.RunnerSource == nil {
		r.RunnerSource = &ServiceRunnerSource{}
	}

	runner := r.RunnerSource.FirstRunner(r)
	r.Stats.AddRunnersCurrent(1)
	r.Stats.AddRunnersStarted(1)
	r.runners = append(r.runners, runner)
}

func (r *RunnerFuzzer) Run(tt assert.T) {
	tt.Helper()

	// stats start must be called before r.Init() because it resets the runner count
	r.Stats.Start()

	r.Init(tt)
	defer r.RunnerSource.End()

	r.wg = newCondGroup()

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

func (r *RunnerFuzzer) Service(svc service.Service) *FuzzServiceStats {
	switch svc.(type) {
	case *service.Group:
		return r.Stats.GroupStats
	default:
		return r.Stats.ServiceStats
	}
}

func (r *RunnerFuzzer) OnServiceError(svc service.Service, err service.Error) {
	switch svc.(type) {
	case *service.Group:
		r.Stats.GroupStats.AddServiceError(err)
	default:
		r.Stats.ServiceStats.AddServiceError(err)
	}
}

func (r *RunnerFuzzer) OnServiceEnd(stage service.Stage, svc service.Service, err service.Error) {
	var s *FuzzServiceStats
	switch svc.(type) {
	case *service.Group:
		s = r.Stats.GroupStats
	default:
		s = r.Stats.ServiceStats
	}
	s.AddServiceEnd(err)

	r.Stats.AddServicesCurrent(-1)
}

func (r *RunnerFuzzer) hotLoop() {
	start := time.Now()
	for time.Since(start) < r.Duration {
		r.doTick()
	}
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
		RunnerLimit:                       fuzzDefaultRunnerLimit,
		ServiceCreateChance:               f.ServiceCreateChance.Rand(),
		ServiceHaltAfter:                  f.ServiceHaltAfter.Rand(),
		ServiceHaltDelay:                  f.ServiceHaltDelay.Rand(),
		ServiceHaltTimeout:                f.ServiceHaltTimeout.Rand(),
		ServiceLimit:                      fuzzDefaultServiceLimit,
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

func randomService(rr service.Runner) service.Service {
	var s service.Service

	// FIXME: this cheat relies on internal implementation details of service.Runner()
	for _, s = range rr.Services(service.AnyState, 1) {
		break
	}

	return s
}
