package tools

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

var (
	onlyOneSignalHandler = make(chan struct{})
)

// SetupSignalHandler registered for SIGTERM and SIGINT
func SetupSignalHandler() context.Context {
	// panic when called twice
	close(onlyOneSignalHandler)

	c := make(chan os.Signal, 2)
	ctx, cancel := context.WithCancel(context.Background())

	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		cancel()
		<-c
		// signal
		os.Exit(1)
	}()

	return ctx
}
