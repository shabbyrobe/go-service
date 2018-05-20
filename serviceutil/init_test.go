package serviceutil

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	beforeCount := pprof.Lookup("goroutine").Count()
	code := m.Run()

	if code == 0 {
		// See notes in service.TestMain
		time.Sleep(20 * time.Millisecond)

		after := pprof.Lookup("goroutine")
		afterCount := after.Count()

		diff := afterCount - beforeCount
		if diff > 0 {
			var buf bytes.Buffer
			after.WriteTo(&buf, 1)
			fmt.Fprintf(os.Stderr, "stray goroutines: %d\n%s\n", diff, buf.String())
			os.Exit(2)
		}
	}

	os.Exit(code)
}
