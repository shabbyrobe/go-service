package servicetest

import (
	"encoding/json"
	"sync"
)

type ErrorCounter struct {
	succeeded int
	failed    int
	errors    map[string]int
	lock      sync.RWMutex
}

func (e *ErrorCounter) MarshalJSON() ([]byte, error) {
	e.lock.Lock()
	defer e.lock.Unlock()

	out := map[string]interface{}{}
	if e.succeeded > 0 {
		out["Succeeded"] = e.succeeded
	}
	if e.failed > 0 {
		out["Failed"] = e.failed
	}
	if len(e.errors) > 0 {
		out["Errors"] = e.errors
	}
	return json.Marshal(out)
}

func (e *ErrorCounter) Total() (out int) {
	e.lock.RLock()
	out = e.succeeded + e.failed
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Succeeded() (out int) {
	e.lock.RLock()
	out = e.succeeded
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Failed() (out int) {
	e.lock.RLock()
	out = e.failed
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Percent() (out float64) {
	e.lock.RLock()
	total := e.succeeded + e.failed
	if total > 0 {
		out = float64(e.succeeded) / float64(total) * 100
	}
	e.lock.RUnlock()
	return out
}

func (e *ErrorCounter) Clone() *ErrorCounter {
	e.lock.RLock()
	ec := &ErrorCounter{
		succeeded: e.succeeded,
		failed:    e.failed,
		errors:    make(map[string]int, len(e.errors)),
	}
	for k, v := range e.errors {
		ec.errors[k] = v
	}
	e.lock.RUnlock()
	return ec
}

func (e *ErrorCounter) Add(err error) {
	e.lock.Lock()
	if err == nil {
		e.succeeded++
	} else {
		e.failed++
		if e.errors == nil {
			e.errors = make(map[string]int)
		}
		for _, msg := range FuzzErrs(err) {
			e.errors[msg]++
		}
	}
	e.lock.Unlock()
}
