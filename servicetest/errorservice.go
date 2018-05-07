package servicetest

import (
	"time"

	service "github.com/shabbyrobe/go-service"
)

type ErrorService struct {
	Name       service.Name
	StartDelay time.Duration
	errc       chan error
	buf        int
	init       bool
}

func (d *ErrorService) Init() *ErrorService {
	d.init = true
	if d.buf <= 0 {
		d.buf = 10
	}
	d.errc = make(chan error, d.buf)
	if d.Name == "" {
		d.Name.AppendUnique()
	}
	return d
}

func (d *ErrorService) ServiceName() service.Name {
	return d.Name
}

func (d *ErrorService) Run(ctx service.Context) error {
	if !d.init {
		panic("call Init()!")
	}
	if d.StartDelay > 0 {
		after := time.After(d.StartDelay)
		for {
			select {
			case err := <-d.errc:
				ctx.OnError(err)
			case <-after:
				goto startDone
			}
		}
	startDone:
	}
	if err := ctx.Ready(); err != nil {
		return err
	}
	for {
		select {
		case err := <-d.errc:
			ctx.OnError(err)
		case <-ctx.Done():
			return nil
		}
	}
}
