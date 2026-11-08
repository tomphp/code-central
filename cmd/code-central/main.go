// Command code-central is the entry point: it wires the real environment
// into the cli package.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/notify"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Run(ctx, os.Args[1:], cli.Env{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,
		Program:     os.Args[0],
		Notifier:    notify.OS(),
	})
	stop()
	os.Exit(code)
}
