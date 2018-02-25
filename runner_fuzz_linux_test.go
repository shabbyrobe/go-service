// +build linux

package service

import (
	"syscall"
	"time"
)

func (r *RunnerFuzzer) tickLoop() {
	start := time.Now().UnixNano()
	dur := int64(r.Duration)
	tick := int64(r.Tick)

	check := dur / tick / 10
	i := 0
	for {
		err := syscall.Nanosleep(syscall.Timespec{Nsec: dur}, nil)
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
