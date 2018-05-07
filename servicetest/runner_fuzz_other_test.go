// +build !linux

package servicetest

import "time"

func (r *RunnerFuzzer) tickLoop() {
	done := make(chan struct{})
	tick := time.NewTicker(r.Tick)
	end := time.After(r.Duration)

	go func() {
		for {
			select {
			case <-tick.C:
				r.doTick()
			case <-end:
				close(done)
				return
			}
		}
	}()
	<-done
}
