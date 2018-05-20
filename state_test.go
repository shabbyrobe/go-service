package service

import (
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

type changer func(then func(err error) error) (err error)

func TestStateChangerErrorPassthrough(t *testing.T) {
	var matrix = map[State]map[State]bool{
		// FROM:                 // TO:
		Halting:  map[State]bool{Halted: true, Starting: false, Started: false, Halting: true},
		Halted:   map[State]bool{Halted: false, Starting: true, Started: false, Halting: false},
		Starting: map[State]bool{Halted: false, Starting: false, Started: true, Halting: true},
		Started:  map[State]bool{Halted: false, Starting: false, Started: false, Halting: true},
	}

	var froms = map[State]State{
		Halted:   Halting,
		Halting:  Starting | Started | Halting,
		Started:  Starting,
		Starting: Halted,
	}

	states := []State{Halted, Starting, Started, Halting}

	for _, from := range states {
		tos := matrix[from]

		for _, to := range states {
			success := tos[to]

			t.Run("", func(t *testing.T) {
				var tt = assert.WrapTB(t)

				sc := from
				last, result := sc.set(to)
				tt.MustEqual(from, last)

				if !success {
					exp := &errState{Expected: froms[to], To: to, Current: from}
					tt.MustAssert(result != nil, "expected error %q when trying to transition from %q to %q, state=%q", exp, from, to, sc)
					tt.MustEqual(exp.Error(), result.Error())
				} else {
					tt.MustAssert(result == nil, "expected no error, found %q when trying to transition from %q to %q", result, from, to)
				}
			})
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
