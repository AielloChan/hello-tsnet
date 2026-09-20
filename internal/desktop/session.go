package desktop

import "context"

type Runner interface {
	Run(context.Context) error
}

// Shell runs on the main OS thread. Finish must be safe from another goroutine.
type Shell interface {
	Run(quit func())
	Finish(error)
}

func Run(ctx context.Context, gateway Runner, shell Shell) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		err := gateway.Run(ctx)
		shell.Finish(err)
		done <- err
	}()
	shell.Run(cancel)
	cancel()
	return <-done
}
