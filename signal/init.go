package signal

var (
	// closedErr is a permanently closed channel (see init())
	closedErr = make(chan error)
)

func init() {
	close(closedErr)
}
