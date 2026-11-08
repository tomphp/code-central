// Command code-central-factory is the local Factory (ADR 0005): run it in
// a repository set up as a Project and it claims that Project's Tickets
// and runs one `claude -p` session per Claim. It wires the real
// environment into the factory package.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/tomoram/code-central/factory"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	wd, _ := os.Getwd()
	code := factory.Run(ctx, os.Args[1:], factory.Env{
		Dir:     wd,
		Environ: os.Environ(),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
	})
	stop()
	os.Exit(code)
}
