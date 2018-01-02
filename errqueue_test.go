package service

import (
	"errors"
	"testing"

	"github.com/shabbyrobe/golib/assert"
)

func TestErrQueueMany(t *testing.T) {
	tt := assert.WrapTB(t)
	perr := errors.New("perr")

	result := make(chan []error)
	eq := newErrQueue()
	eq.Add(2)

	go func() {
		result <- eq.Wait()
	}()

	eq.Put(perr)
	eq.Put(perr)

	tt.MustEqual([]error{perr, perr}, <-result)
}

func TestErrQueueNils(t *testing.T) {
	tt := assert.WrapTB(t)
	perr := errors.New("perr")

	result := make(chan []error)
	eq := newErrQueue()
	eq.Add(3)

	go func() {
		result <- eq.Wait()
	}()

	eq.Put(nil)
	eq.Put(nil)
	eq.Put(perr)

	tt.MustEqual([]error{perr}, <-result)
}
