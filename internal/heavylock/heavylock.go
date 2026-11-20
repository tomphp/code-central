// Command heavylock runs a command while holding the machine-wide lock for
// heavy check suites, so `make test`, `make e2e` and a full `make vitest`
// from sessions that overlap run one at a time instead of each taking
// every core:
//
//	go run ./internal/heavylock -name "make test" [-dir <folder>] -- <command> [args...]
//
// The lock is an OS file lock on a file in the temporary folder, so a
// process that dies releases it. While it waits, heavylock prints one line
// naming the run that holds the lock. The command it runs inherits
// CODE_CENTRAL_HEAVY_LOCK, and a heavylock that finds that set runs its
// command at once: a make that holds the lock can call a target that
// also takes it.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// heldEnv marks a process tree that already holds the lock.
	heldEnv = "CODE_CENTRAL_HEAVY_LOCK"
	poll    = 250 * time.Millisecond
)

func main() {
	name := flag.String("name", "a heavy suite", "what this run is, as the line a waiting run prints names it")
	dir := flag.String("dir", "", "the folder to run the command in, if not this one")
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: heavylock -name <what> -- <command> [args...]")
		os.Exit(2)
	}
	cmd := exec.Command(flag.Arg(0), flag.Args()[1:]...)
	cmd.Dir = *dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if os.Getenv(heldEnv) == "" {
		wd, _ := os.Getwd()
		release, err := acquire(filepath.Join(os.TempDir(), "code-central-heavy"), filepath.Base(wd)+"'s "+*name, os.Stderr, poll)
		if err != nil {
			fmt.Fprintln(os.Stderr, "heavylock:", err)
			os.Exit(1)
		}
		defer release()
		cmd.Env = append(os.Environ(), heldEnv+"=1")
	}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "heavylock:", err)
		os.Exit(1)
	}
}

// acquire waits for the lock at base+".lock", printing one line to w
// naming the holder from base+".holder" if it has to wait, then records
// who as the holder. It returns the function that releases the lock.
func acquire(base, who string, w io.Writer, every time.Duration) (func(), error) {
	told := false
	for {
		release, held, err := tryLock(base + ".lock")
		if err != nil {
			return nil, err
		}
		if !held {
			os.WriteFile(base+".holder", []byte(who), 0o666)
			return release, nil
		}
		if !told {
			told = true
			holder, _ := os.ReadFile(base + ".holder")
			if h := strings.TrimSpace(string(holder)); h != "" {
				fmt.Fprintf(w, "waiting for %s\n", h)
			} else {
				fmt.Fprintln(w, "waiting for another heavy suite")
			}
		}
		time.Sleep(every)
	}
}
