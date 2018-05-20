package service

// Listener allows you to respond to events raised by the Runner in the
// code that owns the Runner, like premature service failure.
//
// Listeners should not be shared between Runners as it is not possible to
// tell which runner is which from the listener methods with the current API.
//
// Listener methods are called in a goroutine. They should not present
// a blocking risk but care should be taken to ensure they terminate.
//
type Listener interface {
	// OnServiceEnd is called when your service ends. If the service responded
	// because it was Halted, err will be nil, otherwise err MUST be set.
	//
	// Every call to Runner.Start will cause a call to OnServiceEnd, regardless
	// of the outcome of the call to Start.
	OnServiceEnd(stage Stage, service Service, err Error)
}

type ErrorListener interface {
	// OnServiceError is called when an error occurs in your running service
	// that does not cause the service to End; the service MUST continue
	// running after this error occurs.
	//
	// This is basically where you send errors that don't have an immediately
	// obvious method of handling, that don't terminate the service, but you
	// don't want to swallow entirely. Essentially it defers the decision for
	// what to do about the error to the parent context.
	//
	// Errors should be wrapped using service.WrapError(err, yourSvc) so
	// context information can be applied.
	OnServiceError(service Service, err Error)
}

type StateListener interface {
	// OnServiceState is called every time a service transitions from one state
	// to another.
	OnServiceState(service Service, from, to State)
}

type ListenerFull interface {
	Listener
	ErrorListener
	StateListener
}
