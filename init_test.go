package service

import (
	"bytes"
	"fmt"
	"os"
	"runtime/pprof"
	"testing"
	"time"
)

const (
	// HACK FEST: This needs to be high enough so that tests that rely on
	// timing don't fail because your computer was too slow
	tscale = 5 * time.Millisecond

	dto = 100 * tscale
)

func TestMain(m *testing.M) {
	beforeCount := pprof.Lookup("goroutine").Count()
	code := m.Run()

	// This little hack gives things like "go OnServiceState" a chance to
	// finish - it routinely shows up in the profile.
	//
	// Also, some calls to the listener that are called with "go" might
	// not have had a chance to finish. This is brittle, true, but some
	// of the tests are hopelessly complicated without it.
	time.Sleep(20 * time.Millisecond)

	if code == 0 {

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
