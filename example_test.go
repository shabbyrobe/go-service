package service

import (
	"context"
	"fmt"
	"time"
)

func Example() {
	msg := make(chan string)

	// Runnables must implement service.Runnable:
	runnable := RunnableFunc(func(ctx Context) error {
		defer fmt.Println("halted")

		// Set up your stuff:
		after := time.After(10 * time.Millisecond)

		// Notify the Runner that we are 'ready', which will unblock the call
		// Runner.Start().
		//
		// If you omit this, Start() will never unblock; failing to call Ready()
		// in a Runnable is an error.
		if err := ctx.Ready(); err != nil {
			return err
		}

		// Run the service, awaiting an instruction from the runner to Halt:
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-after:
				msg <- "stop me!"
			}
		}

		return nil
	})

	runner := NewRunner()

	// Ensure that every service is shut down within 10 seconds, or panic
	// if the deadline is exceeded:
	defer MustShutdownTimeout(10*time.Second, runner)

	// If you want to be notified if the service ends prematurely, attach
	// an EndListener.
	failer := NewFailureListener(1)
	svc := New("my-service", runnable).WithEndListener(failer)

	// Start a service in the background. The call to Start will unblock when
	// the Runnable calls ctx.Ready():
	if err := runner.Start(nil, svc); err != nil {
		panic(err)
	}

	select {
	case s := <-msg:
		fmt.Println(s)
		panic(nil)

		// Halt a service and wait for it to signal it finished:
		if err := runner.Halt(context.TODO(), svc); err != nil {
			panic(err)
		}

	case err := <-failer.Failures():
		// If something goes wrong and MyRunnable ends prematurely,
		// the error returned by MyRunnable.Run() will be sent to the
		// FailureListener.Failures() channel.
		panic(err)
	}

	// Output:
	// stop me!
	// halted
}
