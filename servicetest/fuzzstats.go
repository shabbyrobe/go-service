package servicetest

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type FuzzStats struct {
	Duration        time.Duration
	Seed            int64
	Tick            int32
	RunnersStarted  int32
	RunnersCurrent  int32
	RunnersHalted   int32
	ServicesCurrent int32

	ServiceStats *FuzzServiceStats
	GroupStats   *FuzzServiceStats

	StateCheckResults     map[service.State]int
	stateCheckResultsLock sync.Mutex

	GroupSizes     map[int]int
	groupSizesLock sync.Mutex
	start          time.Time
}

func NewFuzzStats() *FuzzStats {
	return &FuzzStats{
		GroupStats:        NewFuzzServiceStats(),
		ServiceStats:      NewFuzzServiceStats(),
		GroupSizes:        make(map[int]int),
		StateCheckResults: make(map[service.State]int),
	}
}

func (s *FuzzStats) StatsForService(svc service.Service) *FuzzServiceStats {
	switch svc.(type) {
	case *service.Group:
		return s.GroupStats
	default:
		return s.ServiceStats
	}
}

func (s *FuzzStats) Starts() int {
	return s.ServiceStats.ServiceStart.Total() +
		s.ServiceStats.ServiceStartWait.Total() +
		s.GroupStats.ServiceStart.Total() +
		s.GroupStats.ServiceStartWait.Total()
}

func (s *FuzzStats) Ends() int {
	return int(s.ServiceStats.ServiceEnded()) + int(s.GroupStats.ServiceEnded())
}

func (s *FuzzStats) Start() {
	s.start = time.Now()
	atomic.StoreInt32(&s.RunnersCurrent, 0)
	atomic.StoreInt32(&s.Tick, 0)
}

func (s *FuzzStats) GetServicesCurrent() int  { return int(atomic.LoadInt32(&s.ServicesCurrent)) }
func (s *FuzzStats) AddServicesCurrent(n int) { atomic.AddInt32(&s.ServicesCurrent, int32(n)) }

func (s *FuzzStats) GetTick() int { return int(atomic.LoadInt32(&s.Tick)) }
func (s *FuzzStats) AddTick()     { atomic.AddInt32(&s.Tick, 1) }

func (s *FuzzStats) GetRunnersCurrent() int  { return int(atomic.LoadInt32(&s.RunnersCurrent)) }
func (s *FuzzStats) AddRunnersCurrent(n int) { atomic.AddInt32(&s.RunnersCurrent, int32(n)) }

func (s *FuzzStats) GetRunnersStarted() int  { return int(atomic.LoadInt32(&s.RunnersStarted)) }
func (s *FuzzStats) AddRunnersStarted(n int) { atomic.AddInt32(&s.RunnersStarted, int32(n)) }

func (s *FuzzStats) GetRunnersHalted() int  { return int(atomic.LoadInt32(&s.RunnersHalted)) }
func (s *FuzzStats) AddRunnersHalted(n int) { atomic.AddInt32(&s.RunnersHalted, int32(n)) }

func (s *FuzzStats) AddGroupSize(size int) {
	s.groupSizesLock.Lock()
	s.GroupSizes[size]++
	s.groupSizesLock.Unlock()
}

func (s *FuzzStats) AddStateCheckResult(state service.State) {
	s.stateCheckResultsLock.Lock()
	s.StateCheckResults[state]++
	s.stateCheckResultsLock.Unlock()
}

func (s *FuzzStats) Map() map[string]interface{} {
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

func (s *FuzzStats) MarshalJSON() ([]byte, error) {
	// Strip off the methods before marshalling to avoid errant recursion:
	type stats FuzzStats
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

func (s *FuzzStats) Clone() *FuzzStats {
	n := NewFuzzStats()
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

type FuzzServiceStats struct {
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
	ServiceRestart              *ErrorCounter
}

func NewFuzzServiceStats() *FuzzServiceStats {
	return &FuzzServiceStats{
		serviceEnds:   make(map[string]int),
		serviceErrors: make(map[string]int),

		ServiceHalt:                 &ErrorCounter{},
		ServiceStart:                &ErrorCounter{},
		ServiceStartWait:            &ErrorCounter{},
		ServiceUnregisterHalt:       &ErrorCounter{},
		ServiceUnregisterUnexpected: &ErrorCounter{},
		ServiceRegisterBeforeStart:  &ErrorCounter{},
		ServiceRegisterAfterStart:   &ErrorCounter{},
		ServiceRestart:              &ErrorCounter{},
	}
}

func (s *FuzzServiceStats) Map() map[string]interface{} {
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
		"ServiceRestart.Succeeded":              s.ServiceRestart.Succeeded(),
		"ServiceRestart.Failed":                 s.ServiceRestart.Failed(),
	}
}

func (s *FuzzServiceStats) ServiceEnded() (out int) {
	s.serviceEndsLock.RLock()
	out = s.serviceEnded
	s.serviceEndsLock.RUnlock()
	return out
}

func (s *FuzzServiceStats) AddServiceEnd(err error) {
	s.serviceEndsLock.Lock()
	s.serviceEnded++
	for _, msg := range FuzzErrs(err) {
		s.serviceEnds[msg]++
	}
	s.serviceEndsLock.Unlock()
}

func (s *FuzzServiceStats) AddServiceError(err error) {
	s.serviceErrorsLock.Lock()
	for _, msg := range FuzzErrs(err) {
		s.serviceErrors[msg]++
	}
	s.serviceErrorsLock.Unlock()
}

func (s *FuzzServiceStats) Clone() *FuzzServiceStats {
	n := NewFuzzServiceStats()

	n.ServiceHalt = s.ServiceHalt.Clone()
	n.ServiceStart = s.ServiceStart.Clone()
	n.ServiceStartWait = s.ServiceStartWait.Clone()
	n.ServiceUnregisterHalt = s.ServiceUnregisterHalt.Clone()
	n.ServiceUnregisterUnexpected = s.ServiceUnregisterUnexpected.Clone()
	n.ServiceRegisterAfterStart = s.ServiceRegisterAfterStart.Clone()
	n.ServiceRegisterBeforeStart = s.ServiceRegisterBeforeStart.Clone()
	n.ServiceRestart = s.ServiceRestart.Clone()

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
