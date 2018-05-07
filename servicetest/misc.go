package servicetest

import (
	"sort"
	"time"

	service "github.com/shabbyrobe/go-service"
)

const (
	// HACK FEST: This needs to be high enough so that tests that rely on
	// timing don't fail because your computer was too slow
	tscale = 5 * time.Millisecond

	dto = 100 * tscale
)

func causeListSorted(err error) (out []error) {
	if eg, ok := err.(errorGroup); ok {
		out = eg.Errors()
		for i := 0; i < len(out); i++ {
			out[i] = cause(out[i])
		}
		sort.Slice(out, func(i, j int) bool {
			return out[i].Error() < out[j].Error()
		})
		return
	} else {
		return []error{err}
	}
}

type runnerWithFailingStart struct {
	service.Runner

	// start this many services, then fail
	failAfter int

	err error
}

func (t *runnerWithFailingStart) Start(svc service.Service, ready service.ReadySignal) (err error) {
	if t.failAfter > 0 {
		err = t.Runner.Start(svc, ready)
		t.failAfter--
	} else {
		err = t.err
	}
	return
}
