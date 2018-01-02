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
	sc := NewStateChanger()

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
				sc := &StateChanger{state: from}
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
