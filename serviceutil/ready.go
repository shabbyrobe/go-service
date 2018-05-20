package serviceutil

import "sync"

// Ready allows these utilities to signal when they are ready to callers.
// service.Context should implement Ready.
type Ready interface {
	Ready() error
}

// ReadyGroup wraps a sync.WaitGroup so it satisfies the Ready interface.
type ReadyGroup struct {
	sync.WaitGroup
}

func NewReadyGroup(cnt int) *ReadyGroup {
	rg := &ReadyGroup{}
	if cnt > 0 {
		rg.Add(cnt)
	}
	return rg
}

func (r *ReadyGroup) Ready() error {
	r.Done()
	return nil
}
