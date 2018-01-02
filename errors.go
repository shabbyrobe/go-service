package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shabbyrobe/golib/errtools"
)

type (
	errWaitTimeout    int
	errHaltTimeout    int
	errServiceUnknown int
	errNotRestartable int
)

var ErrServiceEnded = errors.New("service ended")

func (errWaitTimeout) Error() string    { return "signal wait timeout" }
func (errHaltTimeout) Error() string    { return "signal halt timeout" }
func (errServiceUnknown) Error() string { return "service unknown" }
func (errNotRestartable) Error() string { return "service not restartable" }

func IsErrWaitTimeout(err error) bool    { _, ok := errtools.Cause(err).(errWaitTimeout); return ok }
func IsErrHaltTimeout(err error) bool    { _, ok := errtools.Cause(err).(errHaltTimeout); return ok }
func IsErrServiceUnknown(err error) bool { _, ok := errtools.Cause(err).(errServiceUnknown); return ok }
func IsErrNotRestartable(err error) bool { _, ok := errtools.Cause(err).(errNotRestartable); return ok }

func IsErrNotRunning(err error) bool {
	serr, ok := errtools.Cause(err).(*errState)
	return ok && !serr.Current.IsRunning()
}

type Error interface {
	error
	errtools.Causer
	Name() Name
}

type errorGroup interface {
	Errors() []error
}

type serviceErrors struct {
	errors []error
}

func (s *serviceErrors) Cause() error {
	if len(s.errors) == 1 {
		return s.errors[0]
	} else {
		return s
	}
}

func (s *serviceErrors) Errors() []error { return s.errors }

func (s *serviceErrors) Error() string {
	if len(s.errors) == 1 {
		return s.errors[0].Error()
	} else {
		var b strings.Builder
		b.WriteString(fmt.Sprintf("%d service error(s) occurred:\n", len(s.errors)))
		for _, e := range s.errors {
			b.WriteString(" - ")
			b.WriteString(e.Error())
			b.WriteString("\n")
		}
		return b.String()
	}
}

type serviceError struct {
	cause error
	name  Name
}

func (s *serviceError) Cause() error { return s.cause }
func (s *serviceError) Name() Name   { return s.name }

func (s *serviceError) Error() string {
	return fmt.Sprintf("service %s error: %v", s.name, s.cause)
}

func WrapError(err error, svc Service) Error {
	if err == nil {
		return nil
	}
	return &serviceError{cause: err, name: svc.ServiceName()}
}

type errState struct {
	Expected, To, Current State
}

func (e *errState) Error() string {
	return fmt.Sprintf(
		"state error: expected %s, found %s when transitioning to %s",
		e.Expected, e.Current, e.To)
}
