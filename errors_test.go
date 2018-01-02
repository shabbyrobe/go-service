package service

import (
	"errors"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func TestIsErrorNotRunning(t *testing.T) {
	tt := assert.WrapTB(t)
	tt.MustAssert(!IsErrNotRunning(errors.New("1")))
	tt.MustAssert(!IsErrNotRunning(&serviceError{cause: errors.New("1")}))

	tt.MustAssert(IsErrNotRunning(&errState{Current: Halted}))
	tt.MustAssert(IsErrNotRunning(&errState{Current: Halting}))
	tt.MustAssert(!IsErrNotRunning(&errState{Current: Started}))
	tt.MustAssert(!IsErrNotRunning(&errState{Current: Starting}))

	tt.MustAssert(IsErrNotRunning(&serviceError{cause: &errState{Current: Halted}}))
	tt.MustAssert(IsErrNotRunning(&serviceError{cause: &errState{Current: Halting}}))
	tt.MustAssert(!IsErrNotRunning(&serviceError{cause: &errState{Current: Started}}))
	tt.MustAssert(!IsErrNotRunning(&serviceError{cause: &errState{Current: Starting}}))
}
