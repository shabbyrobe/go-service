package servicetest

import (
	"errors"
	"sort"
)

var errStartLimit = errors.New("start limit exceeded")

func ErrorListSorted(err error) (out []error) {
	if eg, ok := err.(errorGroup); ok {
		out = eg.Errors()
		sort.Slice(out, func(i, j int) bool {
			return out[i].Error() < out[j].Error()
		})
		return
	} else {
		return []error{err}
	}
}

func CauseListSorted(err error) (out []error) {
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

func ListErrs(err error) (out []error) {
	if err == nil {
		return nil
	}
	c := cause(err)
	if c != nil && c != err {
		err = c
	}

	if grp, ok := err.(errorGroup); ok {
		for _, e := range grp.Errors() {
			out = append(out, ListErrs(e)...)
		}
	} else {
		out = append(out, err)
	}

	return
}

func FuzzErrs(err error) (out []string) {
	for _, e := range ListErrs(err) {
		out = append(out, e.Error())
	}
	return
}

type errorGroup interface {
	Errors() []error
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
