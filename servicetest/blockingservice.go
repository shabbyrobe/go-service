package servicetest

import (
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

// BlockingService is a testing service that does no work, but blocks until it
// is Halted.
type BlockingService struct {
	Name         service.Name
	StartFailure error
	RunFailure   error
	StartDelay   time.Duration
	HaltDelay    time.Duration

	starts int32
	halts  int32
	init   bool
}

func (d *BlockingService) Starts() int { return int(atomic.LoadInt32(&d.starts)) }
func (d *BlockingService) Halts() int  { return int(atomic.LoadInt32(&d.halts)) }

func (d *BlockingService) Init() *BlockingService {
	d.init = true
	if d.Name == "" {
		d.Name.AppendUnique()
	}
	return d
}

func (d *BlockingService) ServiceName() service.Name { return d.Name }

func (d *BlockingService) Run(ctx service.Context) error {
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

	<-ctx.Done()
	if d.HaltDelay > 0 {
		time.Sleep(d.HaltDelay)
	}

	atomic.AddInt32(&d.halts, 1)
	return d.RunFailure
}
