package servicetest

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"runtime/pprof"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
	// "github.com/shabbyrobe/golib/synctools"
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

	ServiceCreateChance float64

	// Chance that the fuzzer will randomly halt a service, regardless of
	// whether it is an appropriate time:
	ServiceHaltChance float64

	ServiceStartFailureChance float64
	ServiceRunFailureChance   float64

	// Chance that the fuzzer will attempt to restart a random service at a
	// random moment.
	ServiceRestartChance float64

	// Chance that a service started by the fuzzer will be restartable.
	ServiceRestartableChance float64

	// The amount of time the service will take before it calls ctx.Ready().
	ServiceStartTime TimeRange

	// The amount of time the fuzzer will wait for the service to start. If
	// this is shorter than ServiceStartTime, the fuzzer will experience start
	// failures.
	StartWaitTimeout TimeRange

	ServiceRunTime     TimeRange
	ServiceHaltAfter   TimeRange
	ServiceHaltDelay   TimeRange
	ServiceHaltTimeout TimeRange
	StateCheckChance   float64

	Stats *FuzzStats `json:"-"`

	runners []service.Runner `json:"-"`
	wg      *condGroup       `json:"-"`
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
		ctx, cancel := context.WithTimeout(context.Background(), r.ServiceHaltTimeout.Rand())
		defer cancel()
		if err := runner.Shutdown(ctx); err != nil {
			// FIXME: accumulate as stats:
			panic(err)
		}
		r.Stats.AddRunnersHalted(1)
	}()
}

func (r *RunnerFuzzer) haltService() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	rn := r.runners[idx]

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		svc := randomService(rn)
		if svc != nil {
			err := service.HaltTimeout(r.ServiceHaltTimeout.Rand(), rn, svc)
			r.Stats.Service.ServiceHalt.Add(err)
		}
	}()
}

func (r *RunnerFuzzer) checkState() {
	idx := rand.Intn(r.Stats.GetRunnersCurrent())
	rn := r.runners[idx]

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		svc := randomService(rn)
		if svc != nil {
			state := rn.State(svc)
			r.Stats.AddStateCheckResult(state)
		}
	}()
}

func (r *RunnerFuzzer) restartService() {
	// FIXME: this does not work yet.
	return

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
			if err := service.HaltTimeout(r.ServiceHaltTimeout.Rand(), rn, svc); err != nil {
				r.Stats.Service.ServiceRestart.Add(err)

			} else {
				err := service.StartTimeout(r.StartWaitTimeout.Rand(), rn, svc)

				// FIXME: the fuzzer should get its stats from the state listener,
				// not from the ended listener. These hacks are all in here because
				// the fuzzer can only dynamically react to a service ending, but
				// not when a service is starting.
				if err == nil {
					r.Stats.AddServicesCurrent(1)
				}

				r.Stats.Service.ServiceRestart.Add(err)
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
	r.runService(service, r.Stats.Service)
}

func (r *RunnerFuzzer) runService(runnable service.Runnable, stats *FuzzServiceStats) {
	runner := r.runners[rand.Intn(r.Stats.GetRunnersCurrent())]

	svc := &service.Service{
		Runnable: runnable,
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()

		err := service.StartTimeout(r.StartWaitTimeout.Rand(), runner, svc)

		r.Stats.AddServicesCurrent(1)

		stats.ServiceStart.Add(err)

		r.wg.Add(1)
		time.AfterFunc(r.ServiceHaltAfter.Rand(), func() {
			defer r.wg.Done()

			err := service.HaltTimeout(r.ServiceHaltTimeout.Rand(), runner, svc)
			stats.ServiceHalt.Add(err)
		})
	}()
}

func (r *RunnerFuzzer) doTick() {
	// maybe halt a runnner, but never if it's the last.
	rcur := r.Stats.GetRunnersCurrent()
	if rcur > 1 && should(r.RunnerHaltChance) {
		r.haltRunner()
	}

	// maybe start a runner
	if rcur == 0 || (should(r.RunnerCreateChance)) && rcur < r.RunnerLimit {
		r.startRunner()
	}

	// maybe start a service into one of the existing runners, chosen
	// at random
	scur := r.Stats.GetServicesCurrent()
	if fuzzServices && should(r.ServiceCreateChance) && scur < r.ServiceLimit {
		r.createService()
	}

	// maybe try halt a random service, regardless of its current state
	if fuzzServices && should(r.ServiceHaltChance) {
		r.haltService()
	}

	// maybe check the state of a randomly chosen service
	if should(r.StateCheckChance) {
		r.checkState()
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

	wait := r.ServiceHaltTimeout.Max
	if r.ServiceRunTime.Max > wait {
		wait = r.ServiceRunTime.Max
	}
	if r.ServiceHaltTimeout.Max > wait {
		wait = r.ServiceHaltTimeout.Max
	}

	{ // wait for goroutines
		doneGo := make(chan struct{})
		go func() {
			r.wg.Wait()
			close(doneGo)
		}()

		after := time.After(wait * 2)
		select {
		case <-after:
			pprof.Lookup("goroutine").WriteTo(os.Stderr, 1)
			panic("fuzzer shutdown waited too long for goroutines")
		case <-doneGo:
		}
	}

	{ // OK now we gotta clean up after ourselves.
		timeout := (2 * time.Second) + (wait * 2)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		for _, rn := range r.runners {
			_ = rn.Shutdown(ctx)
		}
	}

	// Need to wait for any stray halt delays - the above Shutdown
	// call may report everything has halted, but it is skipping
	// things that are already Halting and not blocking to wait for them.
	// That may not be ideal, perhaps it should be fixed.
	time.Sleep(r.ServiceHaltDelay.Max)
}

func (r *RunnerFuzzer) OnServiceError() service.OnError {
	return func(stage service.Stage, service *service.Service, err error) {
		r.Stats.Service.AddServiceError(err)
	}
}

func (r *RunnerFuzzer) OnServiceEnd() service.OnEnd {
	return func(stage service.Stage, service *service.Service, err error) {
		var s *FuzzServiceStats
		s = r.Stats.Service
		s.AddServiceEnd(err)

		r.Stats.AddServicesCurrent(-1)
	}
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
		Duration:                  dur,
		RunnerCreateChance:        f.RunnerCreateChance.Rand(),
		RunnerHaltChance:          f.RunnerHaltChance.Rand(),
		RunnerLimit:               fuzzDefaultRunnerLimit,
		ServiceCreateChance:       f.ServiceCreateChance.Rand(),
		ServiceHaltAfter:          f.ServiceHaltAfter.Rand(),
		ServiceHaltDelay:          f.ServiceHaltDelay.Rand(),
		ServiceHaltTimeout:        f.ServiceHaltTimeout.Rand(),
		ServiceLimit:              fuzzDefaultServiceLimit,
		ServiceRunFailureChance:   f.ServiceRunFailureChance.Rand(),
		ServiceRunTime:            f.ServiceRunTime.Rand(),
		ServiceStartFailureChance: f.ServiceStartFailureChance.Rand(),
		ServiceStartTime:          f.ServiceStartTime.Rand(),
		StartWaitTimeout:          f.StartWaitTimeout.Rand(),
		StateCheckChance:          f.StateCheckChance.Rand(),
	}
}

func randomService(rr service.Runner) *service.Service {
	var s service.ServiceInfo

	// FIXME: this cheat relies on internal implementation details of service.Runner()
	for _, s = range rr.Services(service.AnyState, 1, nil) {
		break
	}

	return s.Service
}
