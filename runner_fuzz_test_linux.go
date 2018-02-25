// +build linux

package service

import (
	"syscall"
	"time"
)

func (r *RunnerFuzzer) tickLoop() {
	start := time.Now()
	dur := int64(r.Duration)
	tick := int64(r.Tick)
	ts := &syscall.Timespec{Nsec: tick}

	check := dur / tick / 10
	i := int64(0)
	for {
		err := syscall.Nanosleep(ts, nil)
		if err != nil {
			panic(nil)
		}
		r.doTick()
		i++

		if i%check == 0 && time.Since(start) > r.Duration {
			return
		}
	}
}
