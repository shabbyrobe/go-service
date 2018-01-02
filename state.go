package service

import "sync"

type State int

func (s State) IsRunning() bool { return s&Running == s }

func (s State) String() string {
	switch s {
	case Halted:
		return "halted"
	case Running:
		return "running (starting or started)"
	case Starting:
		return "starting"
	case Started:
		return "started"
	case Halting:
		return "halting"
	default:
		return "(unknown)"
	}
}

const (
	NoState  = 0
	AnyState = 0

	Halted State = 1 << iota
	Starting
	Started
	Halting

	Running = Starting | Started
)

type StateChanger struct {
	state State
	lock  sync.RWMutex
}

func NewStateChanger() *StateChanger {
	return &StateChanger{state: Halted}
}

func (s *StateChanger) State() State {
	s.lock.RLock()
	ret := s.state
	s.lock.RUnlock()
	return ret
}

func (s *StateChanger) Lock()   { s.lock.Lock() }
func (s *StateChanger) Unlock() { s.lock.Unlock() }

func (s *StateChanger) SetHalting(then func(err error) error) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.state != Started && s.state != Starting {
		err = &errState{Starting | Started, Halting, s.state}
	} else {
		s.state = Halting
	}
	if then != nil {
		err = then(err)
	}
	return
}

func (s *StateChanger) SetStarting(then func(err error) error) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.state != Halted {
		err = &errState{Halted, Starting, s.state}
	} else {
		s.state = Starting
	}
	if then != nil {
		err = then(err)
	}
	return
}

func (s *StateChanger) SetStarted(then func(err error) error) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.state != Starting {
		err = &errState{Starting, Started, s.state}
	} else {
		s.state = Started
	}
	if then != nil {
		err = then(err)
	}
	return
}

func (s *StateChanger) SetHalted(then func(err error) error) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// if s.state != Halting && s.state != Started {
	if s.state != Halting {
		err = &errState{Halting, Halted, s.state}
	} else {
		s.state = Halted
	}
	if then != nil {
		err = then(err)
	}
	return
}
