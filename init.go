package service

// closedErr is a permanently closed channel (see init())
var (
	closedErr   = make(chan error)
	closedBlank = make(chan struct{})
)

func init() {
	close(closedErr)
	close(closedBlank)
}
