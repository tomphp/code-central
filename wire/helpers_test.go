package wire_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	_ "github.com/tomoram/code-central/wire"
)

type result struct {
	code           int
	stdout, stderr string
}

// run runs one command in-process with only the given environment, from
// an empty temporary folder so no real .mcp.json is found.
func run(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	return runIn(t, t.TempDir(), env, stdin, args...)
}

func runIn(t *testing.T, dir string, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin:       strings.NewReader(stdin),
		Stdout:      &out,
		Stderr:      &errOut,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: userHomeDir(env),
		Dir:         dir,
	})
	return result{code, out.String(), errOut.String()}
}

// userHomeDir is os.UserHomeDir on a Windows account: the profile folder
// is env's USERPROFILE, and there is none when that isn't set.
func userHomeDir(env map[string]string) func() (string, error) {
	return func() (string, error) {
		if d := env["USERPROFILE"]; d != "" {
			return d, nil
		}
		return "", errors.New("%USERPROFILE% is not defined")
	}
}
