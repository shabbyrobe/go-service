package service

var (
	closedBlank = make(chan struct{})
)

func init() {
	close(closedBlank)
}
