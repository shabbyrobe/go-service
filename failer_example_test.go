package service

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func ExampleFailureListener() {
	var wg sync.WaitGroup
	wg.Add(3) // there are 3 services we should receive ends for

	onEnd := func(stage Stage, service *Service, err error) {
		fmt.Println(service.Name, err)
		wg.Done()
	}
	runner := NewRunner(RunnerOnEnd(onEnd))

	// This Runnable will run until it is halted:
	runningChild := RunnableFunc(func(ctx Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		<-ctx.Done()
		return nil
	})

	// This Runnable will fail as soon as it is Ready():
	failingChild := RunnableFunc(func(ctx Context) error {
		if err := ctx.Ready(); err != nil {
			return err
		}
		return fmt.Errorf("BOOM!")
	})

	// This service starts two child services and ensures they are halted
	// before it returns. It will fail when the first child service fails:
	parent := New("parent", RunnableFunc(func(ctx Context) error {
		failer := NewFailureListener(1)

		s1 := New("s1", runningChild).WithEndListener(failer)
		s2 := New("s2", failingChild).WithEndListener(failer)
		if err := ctx.Runner().Start(nil, s1, s2); err != nil {
			return err
		}
		defer MustHalt(context.Background(), ctx.Runner(), s1, s2)

		if err := ctx.Ready(); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return nil
		case err := <-failer.Failures():
			return err
		}
	}))

	MustStartTimeout(1*time.Second, runner, parent)
	wg.Wait()

	// Output:
	// s2 BOOM!
	// s1 <nil>
	// parent BOOM!
}
