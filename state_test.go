package service

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

type changer func(then func(err error) error) (err error)

func TestStateChangerThen(t *testing.T) {
	var (
		sentinel     = errors.New("")
		sentinelThen = func(err error) error { return sentinel }
	)

	tt := assert.WrapTB(t)
	sc := newStateChanger()

	fns := []func(then func(err error) error) (err error){
		sc.SetStarted, sc.SetHalted, sc.SetHalting,
	}
	for i := 0; i < 1000; i++ {
		idx := rand.Intn(len(fns))

		// Sentinel should always be returned regardless of state
		tt.MustEqual(sentinel, fns[idx](sentinelThen))
	}
}

func TestStateChangerErrorPassthrough(t *testing.T) {
	var tt = assert.WrapTB(t)

	thenners := []func(err error) error{
		func(err error) error { return err },
		nil,
	}

	var matrix = map[State]map[State]bool{
		Halting:  map[State]bool{Halted: true, Starting: false, Started: false, Halting: false},
		Halted:   map[State]bool{Halted: false, Starting: true, Started: false, Halting: false},
		Starting: map[State]bool{Halted: false, Starting: false, Started: true, Halting: true},
		Started:  map[State]bool{Halted: false, Starting: false, Started: false, Halting: true},
	}

	var froms = map[State]State{
		Halted:   Halting,
		Halting:  Starting | Started,
		Started:  Starting,
		Starting: Halted,
	}

	for _, then := range thenners {
		for from, tos := range matrix {
			for to, success := range tos {
				sc := &stateChanger{state: from}
				var result error
				switch to {
				case Halted:
					result = sc.SetHalted(then)
				case Starting:
					result = sc.SetStarting(then)
				case Started:
					result = sc.SetStarted(then)
				case Halting:
					result = sc.SetHalting(then)
				}
				if !success {
					exp := &errState{Expected: froms[to], To: to, Current: from}
					tt.MustEqual(exp.Error(), result.Error())
				} else {
					tt.MustAssert(result == nil)
				}
			}
		}
	}
}

func TestStateQuery(t *testing.T) {
	tt := assert.WrapTB(t)

	tt.MustAssert(AnyState.Match(Started, false))
	tt.MustAssert(AnyState.Match(Started, true))
	tt.MustAssert(AnyState.Match(Starting, false))
	tt.MustAssert(AnyState.Match(Starting, true))
	tt.MustAssert(AnyState.Match(Halted, false))
	tt.MustAssert(AnyState.Match(Halted, true))
	tt.MustAssert(AnyState.Match(Halting, false))
	tt.MustAssert(AnyState.Match(Halting, true))

	tt.MustAssert(!FindStarting.Match(Started, false))
	tt.MustAssert(!FindStarting.Match(Started, true))
	tt.MustAssert(FindStarting.Match(Starting, false))
	tt.MustAssert(FindStarting.Match(Starting, true))
	tt.MustAssert(!FindStarting.Match(Halted, false))
	tt.MustAssert(!FindStarting.Match(Halted, true))
	tt.MustAssert(!FindStarting.Match(Halting, false))
	tt.MustAssert(!FindStarting.Match(Halting, true))

	tt.MustAssert(FindStarted.Match(Started, false))
	tt.MustAssert(FindStarted.Match(Started, true))
	tt.MustAssert(!FindStarted.Match(Starting, false))
	tt.MustAssert(!FindStarted.Match(Starting, true))
	tt.MustAssert(!FindStarted.Match(Halted, false))
	tt.MustAssert(!FindStarted.Match(Halted, true))
	tt.MustAssert(!FindStarted.Match(Halting, false))
	tt.MustAssert(!FindStarted.Match(Halting, true))

	tt.MustAssert(!FindHalted.Match(Started, false))
	tt.MustAssert(!FindHalted.Match(Started, true))
	tt.MustAssert(!FindHalted.Match(Starting, false))
	tt.MustAssert(!FindHalted.Match(Starting, true))
	tt.MustAssert(FindHalted.Match(Halted, false))
	tt.MustAssert(FindHalted.Match(Halted, true))
	tt.MustAssert(!FindHalted.Match(Halting, false))
	tt.MustAssert(!FindHalted.Match(Halting, true))

	tt.MustAssert(!FindHalting.Match(Started, false))
	tt.MustAssert(!FindHalting.Match(Started, true))
	tt.MustAssert(!FindHalting.Match(Starting, false))
	tt.MustAssert(!FindHalting.Match(Starting, true))
	tt.MustAssert(!FindHalting.Match(Halted, false))
	tt.MustAssert(!FindHalting.Match(Halted, true))
	tt.MustAssert(FindHalting.Match(Halting, false))
	tt.MustAssert(FindHalting.Match(Halting, true))

	tt.MustAssert(FindRunning.Match(Started, false))
	tt.MustAssert(FindRunning.Match(Started, true))
	tt.MustAssert(FindRunning.Match(Starting, false))
	tt.MustAssert(FindRunning.Match(Starting, true))
	tt.MustAssert(!FindRunning.Match(Halted, false))
	tt.MustAssert(!FindRunning.Match(Halted, true))
	tt.MustAssert(!FindRunning.Match(Halting, false))
	tt.MustAssert(!FindRunning.Match(Halting, true))

	tt.MustAssert(!FindNotRunning.Match(Started, false))
	tt.MustAssert(!FindNotRunning.Match(Started, true))
	tt.MustAssert(!FindNotRunning.Match(Starting, false))
	tt.MustAssert(!FindNotRunning.Match(Starting, true))
	tt.MustAssert(FindNotRunning.Match(Halted, false))
	tt.MustAssert(FindNotRunning.Match(Halted, true))
	tt.MustAssert(FindNotRunning.Match(Halting, false))
	tt.MustAssert(FindNotRunning.Match(Halting, true))

	tt.MustAssert(!(FindStarted | FindRegistered).Match(Started, false))
	tt.MustAssert((FindStarted | FindRegistered).Match(Started, true))
	tt.MustAssert((FindStarted | FindRegistered | FindUnregistered).Match(Started, false))
	tt.MustAssert((FindStarted | FindRegistered | FindUnregistered).Match(Started, true))
	tt.MustAssert((FindStarted | FindUnregistered).Match(Started, false))
	tt.MustAssert(!(FindStarted | FindUnregistered).Match(Started, true))

	tt.MustAssert(FindRegistered.Match(Started, true))
	tt.MustAssert(!FindRegistered.Match(Started, false))
	tt.MustAssert(FindUnregistered.Match(Started, false))
	tt.MustAssert(!FindUnregistered.Match(Started, true))
}
