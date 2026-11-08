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
	// The first Ctrl-C drains; a second, SIGTERM or SIGHUP stops the
	// turns. The Factory reads them all from this channel.
	sigs := make(chan os.Signal, 4)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	wd, _ := os.Getwd()
	code := factory.Run(context.Background(), os.Args[1:], factory.Env{
		Dir:     wd,
		Environ: os.Environ(),
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Signals: sigs,
	})
	signal.Stop(sigs)
	os.Exit(code)
}
