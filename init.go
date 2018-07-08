package service

// closed will return a permanently closed channel. It allows <-Ready()
// to be called multiple times for a completed signal that can't be re-used.
var closed = make(chan error)

func init() {
	close(closed)

	GlobalReset(nil)
}
