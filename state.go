package service

import (
	"fmt"
)

// State should not be used as a flag by external consumers of this package.
const (
	NoState State = 0
	Halting State = 1 << iota
	Halted
	Starting
	Started
)

var States = []State{Halting, Halted, Starting, Started}

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
	case Started | Starting:
		return "started or starting"
	case Halting:
		return "halting"
	default:
		return "(unknown)"
	}
}

func (s State) validFrom() State {
	switch s {
	case Starting:
		return Halted
	case Started:
		return Starting
	case Halting:
		return Started | Starting
	case Halted:
		return Halting
	default:
		panic(fmt.Sprintf("invalid state %d", s))
	}
}

func (s *State) set(next State) (err error) {
	from := next.validFrom()
	sv := *s
	if from&sv != sv {
		return &errState{from, next, sv}
	}
	*s = next
	return nil
}

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
