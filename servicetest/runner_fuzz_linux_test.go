// +build linux

package servicetest

import (
	"syscall"
	"time"
)

func (r *RunnerFuzzer) tickLoop() {
	start := time.Now().UnixNano()
	dur := int64(r.Duration)
	tick := int64(r.Tick)

	check := dur / tick / 10
	var i int64
	for {
		err := syscall.Nanosleep(&syscall.Timespec{Nsec: dur}, nil)
		if err != nil {
			panic(err)
		}
		r.doTick()
		i++

		if i%check == 0 && time.Duration(time.Now().UnixNano()-start) > r.Duration {
			return
		}
	}
}
