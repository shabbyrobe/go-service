package servicetest

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

func TestRunnerFuzzHappy(t *testing.T) {
	tt := assert.WrapTB(t)
	// Happy config: should yield no errors
	stats := NewFuzzStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		SyncHalt:           true,
		RunnerCreateChance: 0.0,
		RunnerHaltChance:   0.0,

		// ServiceRestartableChance: 1.0,
		// ServiceRestartChance:     0.01,

		ServiceCreateChance:       0.5,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,

		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceRunTime:     TimeRange{10 * time.Second, 10 * time.Second},
		ServiceHaltAfter:   TimeRange{10 * time.Microsecond, 100 * time.Millisecond},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{2 * time.Second, 2 * time.Second},

		// StateCheckChance: 0.2,

		Stats: stats,
	})

	tt.MustEqual(0, stats.GetServicesCurrent())
	for _, s := range []*FuzzServiceStats{stats.Service} {
		tt.MustEqual(0, s.ServiceHalt.Failed())
		tt.MustEqual(0, s.ServiceStart.Failed())
	}
}

func TestRunnerFuzzHappyLowLimitHighTurnover(t *testing.T) {
	// Happy config: should yield no errors. Low maximum service limit,
	// fast ending services. This was added to raise the probability of
	// complex interactions happening between starts and halts.
	//
	// It helped find a bug that the fuzzer tripped where restarting
	// a service could cause a panic because the runner was somehow
	// able to end up in a starting or started state in between the
	// calls to Halting and Halted.

	tt := assert.WrapTB(t)
	stats := NewFuzzStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		SyncHalt:           true,
		RunnerCreateChance: 0.0,
		RunnerHaltChance:   0.0,
		RunnerLimit:        1,

		ServiceLimit: 2,

		ServiceRestartableChance: 1.0,
		ServiceRestartChance:     1.0,

		ServiceCreateChance:       0.1,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,

		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{10 * time.Second, 10 * time.Second},
		ServiceRunTime:     TimeRange{10 * time.Second, 10 * time.Second},
		ServiceHaltAfter:   TimeRange{100 * time.Microsecond, 10000 * time.Microsecond},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{10 * time.Second, 10 * time.Second},

		StateCheckChance: 0,

		Stats: stats,
	})

	tt.MustEqual(0, stats.GetServicesCurrent())
	for _, s := range []*FuzzServiceStats{stats.Service} {
		tt.MustEqual(0, s.ServiceHalt.Failed())
		tt.MustEqual(0, s.ServiceStart.Failed())
	}
}

func TestRunnerFuzzReasonable(t *testing.T) {
	// Happy error path: will yield plenty of errors, but should not yield
	// any unrecoverable ones.
	stats := NewFuzzStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		SyncHalt:           true,
		RunnerCreateChance: 0.001,
		RunnerHaltChance:   0.0,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0.01,
		ServiceRunFailureChance:   0.02,

		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{10 * time.Second, 10 * time.Second},
		ServiceRunTime:     TimeRange{700 * time.Millisecond, 2 * time.Second},
		ServiceHaltAfter:   TimeRange{100 * time.Millisecond, 1 * time.Second},
		ServiceHaltDelay:   TimeRange{0, 100 * time.Microsecond},
		ServiceHaltTimeout: TimeRange{10 * time.Second, 10 * time.Second},

		StateCheckChance: 0.2,

		Stats: stats,
	})

	tt := assert.WrapTB(t)
	tt.MustEqual(0, stats.GetServicesCurrent())
	tt.MustEqual(stats.Starts(), stats.Ends())

	tt.MustAssert(stats.Service.ServiceStart.Succeeded() > stats.Service.ServiceStart.Failed())
	tt.MustAssert(stats.Service.ServiceHalt.Succeeded() > stats.Service.ServiceHalt.Failed())
}

func TestRunnerFuzzMessy(t *testing.T) {
	stats := NewFuzzStats()
	fz := &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0.000,
		RunnerHaltChance:   0.000,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0.05,
		ServiceRunFailureChance:   0.05,

		ServiceStartTime:   TimeRange{0, 21 * time.Millisecond},
		StartWaitTimeout:   TimeRange{20 * time.Millisecond, 1 * time.Second},
		ServiceRunTime:     TimeRange{0, 500 * time.Millisecond},
		ServiceHaltAfter:   TimeRange{0, 500 * time.Millisecond},
		ServiceHaltDelay:   TimeRange{0, 10 * time.Millisecond},
		ServiceHaltTimeout: TimeRange{9 * time.Millisecond, 10 * time.Millisecond},
		StateCheckChance:   0.2,

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

	for _, s := range []*FuzzServiceStats{stats.Service} {
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

		ServiceStartTime:   TimeRange{0, 50 * time.Millisecond},
		StartWaitTimeout:   TimeRange{0, 50 * time.Millisecond},
		ServiceRunTime:     TimeRange{0, 50 * time.Millisecond},
		ServiceHaltAfter:   TimeRange{0, 50 * time.Millisecond},
		ServiceHaltDelay:   TimeRange{0, 50 * time.Millisecond},
		ServiceHaltTimeout: TimeRange{1 * time.Nanosecond, 50 * time.Millisecond},
		StateCheckChance:   0.2,

		Stats: NewFuzzStats(),
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

	stats := NewFuzzStats()
	testFuzz(t, &RunnerFuzzer{
		Tick:               time.Duration(fuzzTickNsec),
		RunnerCreateChance: 0,
		RunnerHaltChance:   0,

		ServiceCreateChance:       0.2,
		ServiceStartFailureChance: 0,
		ServiceRunFailureChance:   0,

		ServiceStartTime:   TimeRange{0, 0},
		StartWaitTimeout:   TimeRange{1 * time.Second, 1 * time.Second},
		ServiceRunTime:     TimeRange{0, 0},
		ServiceHaltAfter:   TimeRange{0, 0},
		ServiceHaltDelay:   TimeRange{0, 0},
		ServiceHaltTimeout: TimeRange{1 * time.Second, 1 * time.Second},

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

		ServiceCreateChance:       FloatRange{0.001, 0.5},
		ServiceStartFailureChance: FloatRange{0, 0.01},
		ServiceRunFailureChance:   FloatRange{0, 0.01},

		StartWaitChance:    FloatRange{0, 1},
		ServiceStartTime:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 10 * time.Millisecond}},
		StartWaitTimeout:   TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},
		ServiceRunTime:     TimeRangeMaker{TimeRange{100 * time.Microsecond, 100 * time.Millisecond}, TimeRange{1 * time.Second, 5 * time.Second}},
		ServiceHaltAfter:   TimeRangeMaker{TimeRange{10 * time.Microsecond, 1000 * time.Millisecond}, TimeRange{1 * time.Second, 2 * time.Second}},
		ServiceHaltDelay:   TimeRangeMaker{TimeRange{0, 0}, TimeRange{0, 0}},
		ServiceHaltTimeout: TimeRangeMaker{TimeRange{1 * time.Second, 1 * time.Second}, TimeRange{1 * time.Second, 1 * time.Second}},

		StateCheckChance: FloatRange{0, 1},
	}

	iterDur := dur / time.Duration(fuzzMetaRolls)
	min := fuzzMetaMin
	i := 0

	for i < min || time.Since(start) < dur {
		t.Run("", func(t *testing.T) {
			stats := NewFuzzStats()
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
				fuzzOutput(fuzzOutputFormat, t.Name(), stats, os.Stdout)
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
		fz.RunnerLimit = fuzzDefaultRunnerLimit
	}
	if fz.ServiceLimit == 0 {
		fz.ServiceLimit = fuzzDefaultServiceLimit
	}

	rand.Seed(fuzzSeed)
	fz.Stats.Seed = fuzzSeed

	setCurrentFuzzer(fz)

	dur := time.Duration(fuzzTimeDur)
	fz.Duration = dur
	fz.Run(assert.WrapTB(t))

	if testing.Verbose() {
		fuzzOutput(fuzzOutputFormat, t.Name(), fz.Stats, os.Stdout)
	}
}
