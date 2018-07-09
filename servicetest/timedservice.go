package servicetest

import (
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

type TimedService struct {
	StartFailure    error
	StartDelay      time.Duration
	StartLimit      int
	RunFailure      error
	RunTime         time.Duration
	HaltDelay       time.Duration
	UnhaltableSleep bool

	init   bool
	starts int32
	halts  int32
}

func (d *TimedService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *TimedService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *TimedService) Init() *TimedService {
	d.init = true
	return d
}

func (d *TimedService) Run(ctx service.Context) error {
	if !d.init {
		panic("call Init()!")
	}

	starts := atomic.AddInt32(&d.starts, 1)
	if d.StartLimit > 0 && int(starts) > d.StartLimit {
		return errStartLimit
	}

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
		if d.UnhaltableSleep {
			time.Sleep(d.RunTime)
		} else {
			service.Sleep(ctx, d.RunTime)
		}
	}
	if ctx.ShouldHalt() {
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
