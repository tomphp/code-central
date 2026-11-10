// Command gh is the fake gh as a program, for the UI's end-to-end tests:
// it keeps its Script and calls in the folder FAKE_GH_DIR names.
package main

import (
	"os"

	"github.com/tomoram/code-central/internal/fakegh"
)

func main() {
	os.Exit(fakegh.Run(os.Getenv("FAKE_GH_DIR"), os.Args[0], os.Args[1:], os.Stdout, os.Stderr))
}
