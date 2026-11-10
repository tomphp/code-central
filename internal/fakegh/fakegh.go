// Package fakegh is a fake gh, the GitHub CLI, for tests. It follows a
// Script of Answers kept in its folder and records each call there, so
// a test sees what the CI Watcher asked GitHub. ciwatchertest runs the
// test binary as it; `make e2e` builds it as a program (./gh) for the
// UI's end-to-end tests, which put it on the service's PATH.
package fakegh

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// Answer is how the fake answers a call whose args start with Args.
type Answer struct {
	// Args is the start of the args answered, such as auth, status.
	Args   []string `json:"args"`
	Stdout string   `json:"stdout,omitempty"`
	Stderr string   `json:"stderr,omitempty"`
	Exit   int      `json:"exit,omitempty"`
}

// Call is one run of the fake.
type Call struct {
	// Program is the fake as it was run, so a test sees which gh was found.
	Program string   `json:"program"`
	Args    []string `json:"args"`
}

const (
	scriptFile = "answers.json"
	callsDir   = "calls"
)

// Run is the fake gh run as program with args, its Script and calls in
// dir. The latest Answer that matches is given; with none, it exits 0
// and prints nothing, as a logged-in gh reading a readable repository
// would need no more. With dir empty it records nothing.
func Run(dir, program string, args []string, stdout, stderr io.Writer) int {
	if dir == "" {
		return 0
	}
	answers, err := readAnswers(dir)
	if err == nil {
		err = record(dir, Call{Program: program, Args: args})
	}
	if err != nil {
		fmt.Fprintln(stderr, "fake gh:", err)
		return 70
	}
	for _, a := range slices.Backward(answers) {
		if len(args) >= len(a.Args) && slices.Equal(args[:len(a.Args)], a.Args) {
			fmt.Fprint(stdout, a.Stdout)
			fmt.Fprint(stderr, a.Stderr)
			return a.Exit
		}
	}
	return 0
}

// Script adds answers to the fake's Script in dir, after those it has.
func Script(dir string, answers ...Answer) error {
	have, err := readAnswers(dir)
	if err != nil {
		return err
	}
	b, err := json.Marshal(append(have, answers...))
	if err != nil {
		return err
	}
	return writeAtomic(dir, scriptFile, b)
}

// Calls is every call of the fake in dir so far, oldest first.
func Calls(dir string) ([]Call, error) {
	entries, err := os.ReadDir(filepath.Join(dir, callsDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []Call
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, callsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		var c Call
		if err := json.Unmarshal(b, &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func readAnswers(dir string) ([]Answer, error) {
	b, err := os.ReadFile(filepath.Join(dir, scriptFile))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var answers []Answer
	return answers, json.Unmarshal(b, &answers)
}

// record writes c into the calls folder, named so they sort oldest first.
func record(dir string, c Call) error {
	if err := os.MkdirAll(filepath.Join(dir, callsDir), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeAtomic(dir, filepath.Join(callsDir, fmt.Sprintf("%020d-%d.json", time.Now().UnixNano(), os.Getpid())), b)
}

// writeAtomic writes name in dir through a file beside it and a rename,
// so a reader never sees half of it.
func writeAtomic(dir, name string, b []byte) error {
	tmp, err := os.CreateTemp(dir, ".fakegh-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(b)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
