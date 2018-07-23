package servicetest

import (
	"fmt"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/serviceutil"
	"github.com/shabbyrobe/golib/assert"
)

func TestTimedRestart_Restart(t *testing.T) {
	const restartLim = 2
	restartErr := fmt.Errorf("restart")

	runnables := []service.Runnable{
		service.RunnableFunc(func(ctx service.Context) error {
			if err := ctx.Ready(); err != nil {
				return err
			}
			return restartErr
		}),
		service.RunnableFunc(func(ctx service.Context) error {
			return restartErr
		}),
	}

	for _, rn := range runnables {
		t.Run("", func(t *testing.T) {
			tt := assert.WrapTB(t)

			type restart struct {
				start uint64
				err   error
			}
			restarts := make(chan restart, restartLim)
			trn := func(start uint64, err error) {
				restarts <- restart{start, err}
			}

			runner := service.NewRunner()
			defer service.MustShutdownTimeout(dto, runner)

			failer := service.NewFailureListener(1)
			tr := serviceutil.NewTimedRestart(rn, dto, serviceutil.WaitFixed(100*time.Microsecond),
				serviceutil.TimedRestartLimit(restartLim),
				serviceutil.TimedRestartNotify(trn))
			tt.MustOK(runner.Start(nil, service.New("", tr).WithEndListener(failer)))

			tt.MustAssert(serviceutil.IsRestartLimitExceeded(<-failer.Failures()))
			tt.MustEqual(restart{1, restartErr}, <-restarts)
			tt.MustEqual(restart{2, restartErr}, <-restarts)
			select {
			case <-restarts:
				tt.Error()
			default:
			}
			tt.MustEqual(uint64(2), tr.Starts())
			tt.MustEqual(false, tr.Running())
		})
	}
}

func TestTimedRestart_RunningNormally(t *testing.T) {
	tt := assert.WrapTB(t)
	rn := service.RunnableFunc(func(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	trn := func(start uint64, err error) {
		// Should not happen in this test
		panic(fmt.Sprintf("start:%d err:%v", start, err))
	}

	runner := service.NewRunner()
	// NOTE: No runner shutdown: we want to make sure Halt closes the running goroutines.

	ender := service.NewEndListener(1)
	tr := serviceutil.NewTimedRestart(rn, 1*time.Second, serviceutil.WaitFixed(100*time.Microsecond),
		serviceutil.TimedRestartNotify(trn))
	svc := service.New("", tr).WithEndListener(ender)

	tt.MustOK(runner.Start(nil, svc))
	time.Sleep(1 * time.Millisecond) // Things should have settled, but wait a tic just in case
	tt.MustOK(runner.Halt(nil, svc))

	tt.MustOK(<-ender.Ends())
	tt.MustEqual(uint64(1), tr.Starts())
	tt.MustEqual(false, tr.Running())
}

func TestTimedRestart_SuspendServiceRunningNormally(t *testing.T) {
	tt := assert.WrapTB(t)
	rn := service.RunnableFunc(func(ctx service.Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	stopped := make(chan struct{})
	trn := func(start uint64, err error) {
		close(stopped)
	}

	runner := service.NewRunner()
	defer service.MustShutdownTimeout(dto, runner)

	ender := service.NewEndListener(1)
	tr := serviceutil.NewTimedRestart(rn, 1*time.Second, serviceutil.WaitFixed(100*time.Microsecond),
		serviceutil.TimedRestartNotify(trn))
	svc := service.New("", tr).WithEndListener(ender)

	tt.MustOK(runner.Start(nil, svc))
	tt.MustAssert(tr.Suspend(true))
	<-stopped
	tt.MustEqual(false, tr.Running())
}
