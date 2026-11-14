// Command corewasm builds the WASM Core (Spec code-central-580) for
// `make corewasm`: cmd/corewasm built with GOOS=js GOARCH=wasm, stripped,
// into core.wasm in the folder it is given, and beside it the
// wasm_exec.js of the same Go, so the glue always matches the compiler.
// It is Go rather than shell so the recipe runs the same under cmd.exe
// and POSIX sh. Like internal/clean, it is a developer tool for the
// Makefile, not a shipped command. Neither output is committed.
//
//	go run ./internal/corewasm web/src/api/core
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: corewasm <folder>")
		os.Exit(2)
	}
	if err := build(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "corewasm:", err)
		os.Exit(1)
	}
}

// build writes core.wasm and wasm_exec.js into dir.
func build(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", filepath.Join(dir, "core.wasm"), "./cmd/corewasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("building core.wasm: %w", err)
	}
	// The go that built core.wasm names its own GOROOT.
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		return fmt.Errorf("finding GOROOT: %w", err)
	}
	glue, err := os.ReadFile(filepath.Join(strings.TrimSpace(string(out)), "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "wasm_exec.js"), glue, 0o644)
}
