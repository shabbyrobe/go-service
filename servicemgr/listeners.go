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

// SendNonNil sends an arbitrary error through the failure channel if it is not nil.
// Use it if you want to mix arbitrary goroutine error handling with service failure.
func (f *FailureListener) SendNonNil(err error) {
	if err != nil {
		select {
		case f.failures <- err:
		default:
		}
	}
}

func (f *FailureListener) OnServiceEnd(service service.Service, err service.Error) {
	if err != nil {
		select {
		case f.failures <- err:
		default:
		}
	}
}
