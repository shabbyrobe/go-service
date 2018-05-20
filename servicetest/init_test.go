package servicetest

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
	"sync"
	"testing"
	"time"
)

const (
	fuzzFormatJSON = "json"
	fuzzFormatCLI  = "cli"
)

var (
	fuzzEnabled      bool
	fuzzTimeStr      string
	fuzzOutputFormat string
	fuzzTimeDur      time.Duration
	fuzzTickNsec     int64
	fuzzSeed         int64
	fuzzDebugHost    string
	fuzzMetaRolls    int64
	fuzzMetaMin      int

	fuzzDefaultRunnerLimit  int
	fuzzDefaultServiceLimit int
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

func TestMain(m *testing.M) {
	flag.BoolVar(&fuzzEnabled, "service.fuzz", false,
		"Fuzz? Nope by default.")
	flag.StringVar(&fuzzTimeStr, "service.fuzztime", "1s",
		"Run the fuzzer for this duration")
	flag.Int64Var(&fuzzTickNsec, "service.fuzzticknsec", 0,
		"How frequently to tick in the fuzzer's loop (0 for a hot loop).")
	flag.Int64Var(&fuzzSeed, "service.fuzzseed", -1,
		"Randomise the fuzz tester with this non-negative seed prior to every fuzz test")
	flag.StringVar(&fuzzDebugHost, "service.debughost", "",
		"Start a debug server at this host to allow expvars/pprof")
	flag.Int64Var(&fuzzMetaRolls, "service.fuzzmetarolls", 20,
		"Re-roll the meta fuzz tester this many times")
	flag.IntVar(&fuzzMetaMin, "service.fuzzmetamin", 5,
		"Minimum number of times to run the meta fuzzer regardless of duration")
	flag.StringVar(&fuzzOutputFormat, "service.fuzzoutfmt", "cli",
		"Fuzz verbose output format (cli or json)")
	flag.IntVar(&fuzzDefaultRunnerLimit, "service.fuzzrunnerlim", 20000,
		"Limit the number of runners that can concurrently exist if the fuzz test does not explicitly declare a limit")
	flag.IntVar(&fuzzDefaultServiceLimit, "service.fuzzservicelim", 100000,
		"Limit the number of services that can concurrently exist if the fuzz test does not explicitly declare a limit")

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
