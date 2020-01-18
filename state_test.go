package service

import (
	"testing"

	"github.com/shabbyrobe/go-service/internal/assert"
)

func TestStateString(t *testing.T) {
	for _, tc := range []struct {
		state State
		out   string
	}{
		{Halting, "halting"},
		{Halted | Ended, "(ended or halted)"},
		{Halted | Ended | Starting, "(ended or starting or halted)"},
		{NoState, "<none>"},
	} {
		t.Run("", func(t *testing.T) {
			tt := assert.WrapTB(t)
			tt.MustEqual(tc.out, tc.state.String())
		})
	}
}
