package servicetest

import (
	"context"
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

func (t *runnerWithFailingStart) Start(ctx context.Context, svcs ...*service.Service) (rerr error) {
	if t.failAfter > 0 {
		rerr = t.Runner.Start(ctx, svcs...)
		t.failAfter--
	} else {
		rerr = t.err
	}
	return
}
