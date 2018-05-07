package servicetest

import (
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type TimedService struct {
	Name         service.Name
	StartFailure error
	StartDelay   time.Duration
	RunFailure   error
	RunTime      time.Duration
	HaltDelay    time.Duration
	HaltingSleep bool

	init   bool
	starts int32
	halts  int32
}

func (d *TimedService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *TimedService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *TimedService) Init() *TimedService {
	d.init = true
	if d.Name == "" {
		d.Name.AppendUnique()
	}
	return d
}

func (d *TimedService) ServiceName() service.Name {
	return d.Name
}

func (d *TimedService) Run(ctx service.Context) error {
	if !d.init {
		panic("call Init()!")
	}

	atomic.AddInt32(&d.starts, 1)

	if d.StartDelay > 0 {
		time.Sleep(d.StartDelay)
	}
	if d.StartFailure != nil {
		return d.StartFailure
	}
	if err := ctx.Ready(); err != nil {
		return err
	}

	defer atomic.AddInt32(&d.halts, 1)

	if d.RunTime > 0 {
		if d.HaltingSleep {
			service.Sleep(ctx, d.RunTime)
		} else {
			time.Sleep(d.RunTime)
		}
	}
	if service.IsDone(ctx) {
		if d.HaltDelay > 0 {
			time.Sleep(d.HaltDelay)
		}
		return nil
	} else {
		if d.RunFailure == nil {
			return service.ErrServiceEnded
		}
		return d.RunFailure
	}
}
