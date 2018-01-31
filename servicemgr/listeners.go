package servicemgr

import service "github.com/shabbyrobe/go-service"

// FailureListener provides a channel that emits errors when services end
// unexpectedly.
//
// If the failure channel would block, errors are discarded.
type FailureListener struct {
	failures chan error
}

var _ Listener = &FailureListener{}

func NewFailureListener(cap int) *FailureListener {
	if cap < 1 {
		cap = 1
	}
	return &FailureListener{
		failures: make(chan error, cap),
	}
}

func (f *FailureListener) Failures() <-chan error {
	return f.failures
}

func (f *FailureListener) OnServiceEnd(service service.Service, err service.Error) {
	if err != nil {
		select {
		case f.failures <- err:
		default:
		}
	}
}
