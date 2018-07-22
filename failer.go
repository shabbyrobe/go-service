package service

// FailureListener provides a channel that emits errors when services end
// unexpectedly.
//
// WARNING: this API is not stable.
//
// If the failure channel would block, errors are discarded.
//
// If you expect a certain number of errors, you should pass at least that
// number to cap. If not, you may find that any errors that occur between
// a service stopping unexpectedly and the channel being read from get
// dropped.
//
type FailureListener struct {
	failures chan error
}

var _ OnEnd = (&FailureListener{}).OnEnd

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

func (f *FailureListener) AttachEnd(svc *Service)  { svc.OnEnd = f.OnEnd }
func (f *FailureListener) ForRunner() RunnerOption { return RunnerOnEnd(f.OnEnd) }

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

// Send sends an arbitrary error through the failure channel. It can send nil.
// err is discarded if Send woudl block.
func (f *FailureListener) Send(err error) {
	select {
	case f.failures <- err:
	default:
	}
}

func (f *FailureListener) OnEnd(stage Stage, service *Service, err error) {
	if err != nil {
		select {
		case f.failures <- err:
		default:
		}
	}
}

// EndListener provides a channel that emits errors when services end
// unexpectedly, and nil when they end expectedly.
//
// WARNING: this API is not stable.
//
// If the ends channel would block, items are discarded.
//
// If you expect a certain number of ends, you should pass at least that
// number to cap. If not, you may find that any ends that occur between
// a service stopping unexpectedly and the channel being read from get
// dropped.
//
type EndListener struct {
	ends chan error
}

var _ OnEnd = (&EndListener{}).OnEnd

func NewEndListener(cap int) *EndListener {
	if cap < 1 {
		cap = 1
	}
	return &EndListener{
		ends: make(chan error, cap),
	}
}

func (e *EndListener) Ends() <-chan error {
	return e.ends
}

func (e *EndListener) AttachEnd(svc *Service)  { svc.OnEnd = e.OnEnd }
func (e *EndListener) ForRunner() RunnerOption { return RunnerOnEnd(e.OnEnd) }

// Send sends an arbitrary error through the failure channel. It can send nil.
// err is discarded if Send woudl block.
func (e *EndListener) Send(err error) {
	select {
	case e.ends <- err:
	default:
	}
}

// SendNonNil sends an arbitrary error through the failure channel if it is not nil.
// Use it if you want to mix arbitrary goroutine error handling with service failure.
func (e *EndListener) SendNonNil(err error) {
	if err != nil {
		select {
		case e.ends <- err:
		default:
		}
	}
}

func (e *EndListener) OnEnd(stage Stage, service *Service, err error) {
	select {
	case e.ends <- err:
	default:
	}
}
