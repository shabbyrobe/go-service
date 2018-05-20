package servicemgr

// Stats is a regrettable and hopefully interim solution to the problem
// of needing to get internal state information in the servicetest
// package.
//
// Deprecated. This API is not guaranteed to be even the slightest bit stable.
// It should not be considered part of the public API.
func Stats() (stats struct {
	Listeners      int
	ListenersError int
	ListenersState int
	Retained       int
}) {
	l := getListener()
	l.lock.Lock()
	stats.Listeners = len(l.listeners)
	stats.ListenersError = len(l.listenersError)
	stats.ListenersState = len(l.listenersState)
	stats.Retained = len(l.retained)

	l.lock.Unlock()
	return
}
