package servicetest

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/shabbyrobe/golib/assert"
)

const (
	DefaultRunnerLimit  int = 20000
	DefaultServiceLimit int = 100000
)

func TestRunnerFuzzHappy(t *testing.T) {
	tt := assert.WrapTB(t)
	// Happy config: should yield no errors
	stats := NewFuzzStats()
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

	tt.MustEqual(0, stats.GetServicesCurrent())
	for _, s := range []*FuzzServiceStats{stats.ServiceStats, stats.GroupStats} {
		tt.MustEqual(0, s.ServiceHalt.Failed())
		tt.MustEqual(0, s.ServiceStart.Failed())
		tt.MustEqual(0, s.ServiceStartWait.Failed())
		tt.MustEqual(0, s.ServiceRegisterBeforeStart.Failed())
		tt.MustEqual(0, s.ServiceUnregisterHalt.Failed())
		tt.MustEqual(0, s.ServiceUnregisterUnexpected.Failed())
	}
}

func TestRunnerFuzzGlobalHappy(t *testing.T) {
	tt := assert.WrapTB(t)
	// Happy config: should yield no errors
	stats := NewFuzzStats()
	testFuzz(t, &RunnerFuzzer{
		RunnerSource: &ServiceManagerRunnerSource{},

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

	tt.MustEqual(0, stats.GetServicesCurrent())
	for _, s := range []*FuzzServiceStats{stats.ServiceStats, stats.GroupStats} {
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
	stats := NewFuzzStats()
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
	stats := NewFuzzStats()
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

	for _, s := range []*FuzzServiceStats{stats.ServiceStats, stats.GroupStats} {
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
		fuzzOutput(fuzzOutputFormat, t.Name(), fz.Stats, os.Stdout)
	}
}
