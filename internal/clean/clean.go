// Command clean deletes the paths it is given, and everything under
// them, for `make clean`. It is Go rather than `rm -rf` so the recipe
// runs the same under cmd.exe and POSIX sh. A missing path is not an
// error. Like internal/testrun, it is a developer tool for the Makefile,
// not a shipped command.
//
//	go run ./internal/clean bin web/dist
package main

import (
	"fmt"
	"os"
)

func main() {
	failed := false
	for _, path := range os.Args[1:] {
		if err := os.RemoveAll(path); err != nil {
			fmt.Fprintln(os.Stderr, "clean:", err)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}
