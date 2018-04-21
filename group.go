package service

import (
	"fmt"
	"time"
)

const (
	GroupHaltTimeout  = 10 * time.Second
	GroupReadyTimeout = 10 * time.Second
)

// Group wraps a set of services which should all start and stop at the same
// time. If a service Ends, all will be halted.
//
// If the group does not halt properly, you should panic as it is likely
// there is no way to recover the lost resources.
//
type Group struct {
	name         Name
	services     []Service
	haltTimeout  time.Duration
	readyTimeout time.Duration

	// This is here mainly for testing purposes, so we can swap out
	// the Runner for a broken one.
	runnerBuilder func(l Listener) Runner
}

func NewGroup(name Name, services ...Service) *Group {
	return &Group{
		name:          name,
		services:      services,
		haltTimeout:   GroupHaltTimeout,
		readyTimeout:  GroupReadyTimeout,
		runnerBuilder: NewRunner,
	}
}

type groupListener struct {
	errs chan Error
	ends chan Error
	done <-chan struct{}
}

func newGroupListener(sz int) *groupListener {
	return &groupListener{
		errs: make(chan Error),
		ends: make(chan Error, sz),
	}
}

func (l *groupListener) OnServiceError(service Service, err Error) {
	select {
	case l.errs <- err:
	case <-l.done:
	}
}

func (l *groupListener) OnServiceEnd(stage Stage, service Service, err Error) {
	if stage == StageRun {
		select {
		case l.ends <- err:
		case <-l.done:
		}
	}
}

func (l *groupListener) OnServiceState(service Service, state State) {}

func (g *Group) ServiceName() Name { return g.name }

func (g *Group) Run(ctx Context) error {
	listener := newGroupListener(len(g.services))
	listener.done = ctx.Done()
	runner := g.runnerBuilder(listener)

	var err error

	ready := NewMultiReadySignal(len(g.services))

	for _, s := range g.services {
		_ = runner.Register(s)
		if err = runner.Start(s, ready); err != nil {
			if _, herr := runner.HaltAll(g.haltTimeout, 0); herr != nil {
				return &errGroupHalt{name: g.ServiceName(), haltError: herr, cause: err}
			}
			goto done
		}
	}

	if err = WhenReady(g.readyTimeout, ready); err != nil {
		goto done
	}
	if err = ctx.Ready(); err != nil {
		goto done
	}

	for {
		select {
		case <-ctx.Done():
			goto done
		case lerr := <-listener.errs:
			ctx.OnError(WrapError(lerr, g))
		case err = <-listener.ends:
			goto done
		}
	}

done:
	_, herr := runner.HaltAll(g.haltTimeout, 0)
	if herr == nil {
		return err
	} else if err == nil {
		return herr
	} else {
		return &errGroupHalt{name: g.ServiceName(), haltError: herr, cause: err}
	}
}

func IsErrGroupHalt(err error) bool {
	_, ok := err.(*errGroupHalt)
	return ok
}

type errGroupHalt struct {
	name      Name
	haltError error
	cause     error
}

func (e *errGroupHalt) Error() string {
	return fmt.Sprintf("group halt error in service %s: %v, caused by: %v", e.name, e.haltError, e.cause)
}

func (e *errGroupHalt) Cause() error {
	return e.cause
}
