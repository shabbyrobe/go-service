package service

import (
	"sync"
)

type State int

func (s State) IsRunning() bool { return s == Starting || s == Started }

func (s State) String() string {
	switch s {
	case Halted:
		return "halted"
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
	NoState State = iota
	Halted
	Starting
	Started
	Halting
)

type StateQuery int

func (q StateQuery) Match(state State, registered bool) bool {
	svcMatch := true
	if q != AnyState && (q&(FindRunning|FindNotRunning) > 0) {
		switch state {
		case Halting:
			svcMatch = q&FindHalting != 0
		case Halted:
			svcMatch = q&FindHalted != 0
		case Starting:
			svcMatch = q&FindStarting != 0
		case Started:
			svcMatch = q&FindStarted != 0
		}
	}

	regMatch := false
	findReg, findUnreg := (q&FindRegistered != 0), (q&FindUnregistered != 0)
	if (findReg && registered) || (findUnreg && !registered) {
		regMatch = true
	} else if !findReg && !findUnreg {
		regMatch = true
	}

	return svcMatch && regMatch
}

const (
	AnyState StateQuery = 0

	FindHalted StateQuery = 1 << iota
	FindStarting
	FindStarted
	FindHalting

	FindRegistered
	FindUnregistered

	FindRunning    = FindStarting | FindStarted
	FindNotRunning = FindHalting | FindHalted
)

type stateChanger struct {
	state State
	lock  sync.RWMutex
}

func newStateChanger() *stateChanger {
	return &stateChanger{state: Halted}
}

func (s *stateChanger) State() State {
	s.lock.RLock()
	ret := s.state
	s.lock.RUnlock()
	return ret
}

func (s *stateChanger) Lock()   { s.lock.Lock() }
func (s *stateChanger) Unlock() { s.lock.Unlock() }

func (s *stateChanger) SetHalting(then func(err error) error) (err error) {
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

func (s *stateChanger) SetStarting(then func(err error) error) (err error) {
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

func (s *stateChanger) SetStarted(then func(err error) error) (err error) {
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

func (s *stateChanger) SetHalted(then func(err error) error) (err error) {
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
