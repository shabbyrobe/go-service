package service

type Stage int

const (
	StageReady Stage = 1
	StageRun   Stage = 2
)

type RunnerState int

const (
	RunnerEnabled   RunnerState = 0
	RunnerSuspended RunnerState = 1
	RunnerShutdown  RunnerState = 2
)

type State int

// State should not be used as a flag by external consumers of this package.
const (
	NoState  State = 0
	AnyState State = 0 // AnyState is a symbolic name used to make queries more readable.
	Halted   State = 1 << iota
	Starting
	Started
	Halting
	Ended

	Running    = Starting | Started | Halting
	NotRunning = NoState | Halted | Ended
)

var States = []State{Halting, Halted, Starting, Started}

func (s State) IsRunning() bool { return s == Starting || s == Started }

func (s State) name() string {
	switch s {
	case Halted:
		return "halted"
	case Starting:
		return "starting"
	case Started:
		return "started"
	case Halting:
		return "halting"
	case Ended:
		return "ended"
	case NoState:
		return "<none>"
	}
	return ""
}

func (s State) String() string {
	out := s.name()
	if out == "" {
		out = "("
		first := true
		for i := Ended; i > 0; i >>= 1 {
			if i&s != i {
				continue
			}
			if !first {
				out += " or "
			} else {
				first = false
			}
			out += i.name()
		}
		out += ")"
	}
	return out
}

func (s State) Match(q State) bool {
	if q == AnyState {
		return true
	}
	return q&s == q
}
