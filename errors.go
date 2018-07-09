package service

import (
	"errors"
	"fmt"
	"strings"
)

type (
	errRunnerSuspended int
)

// ErrServiceEnded is a sentinel error used to indicate that a service ended
// prematurely but no obvious error that could be returned.
//
// It is a part of the public API so that consumers of this package can return
// it from their own services.
var ErrServiceEnded = errors.New("service ended")

func (errRunnerSuspended) Error() string { return "service: runner was shut down" }

func IsRunnerSuspended(err error) bool { _, ok := cause(err).(errRunnerSuspended); return ok }
func IsServiceEnded(err error) bool    { return cause(err) == ErrServiceEnded }

type Error interface {
	error
	causer
	Name() Name
}

func Errors(err error) []error {
	if err == nil {
		return nil
	}
	if errs, ok := err.(errorGroup); ok {
		return errs.Errors()
	}
	return []error{err}
}

func WrapError(err error, svc *Service) Error {
	if err == nil {
		return nil
	}
	sname := svc.Name
	if serr, ok := err.(*serviceError); ok {
		if serr.Name() == sname {
			return serr
		}
	}
	return &serviceError{cause: err, name: sname}
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
		return nil
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
	return fmt.Sprintf("service %q error: %v", s.name, s.cause)
}

type errState struct {
	Expected, To, Current State
}

func (e *errState) Error() string {
	return fmt.Sprintf(
		"service state error: expected %q; found %q when transitioning to %q",
		e.Expected, e.Current, e.To)
}

type causer interface {
	Cause() error
}

func cause(err error) error {
	var last = err
	var rerr = err

	for rerr != nil {
		cause, ok := rerr.(causer)
		if !ok {
			break
		}
		rerr = cause.Cause()
		if rerr == nil {
			rerr = last
			break
		}
		if rerr == last {
			break
		}

		last = rerr
	}
	if rerr == nil {
		rerr = err
	}
	return rerr
}
