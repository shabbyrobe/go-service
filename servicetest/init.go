package servicetest

import (
	"sync"
	"time"
)

const (
	fuzzFormatJSON = "json"
	fuzzFormatCLI  = "cli"
)

var (
	fuzzEnabled      bool
	fuzzServices     bool
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
