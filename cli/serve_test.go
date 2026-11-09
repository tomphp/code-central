package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
)

// serveWith runs serve with the given environment, flags and addresses
// (nil addrs: serve picks them, from the settings) and waits until it
// listens or exits.
func serveWith(t *testing.T, env map[string]string, addrs []string, args ...string) (bound []string, stderr *bytes.Buffer, failed *result) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	listening := make(chan []string, 1)
	done := make(chan result, 1)
	var errOut bytes.Buffer
	go func() {
		var out bytes.Buffer
		code := cli.Run(ctx, append([]string{"serve", "--store", t.TempDir()}, args...), cli.Env{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
			Getenv:      func(k string) string { return env[k] },
			UserHomeDir: userHomeDir(env),
			ServeAddrs:  addrs,
			OnListening: func(a []string) { listening <- a },
		})
		done <- result{code, out.String(), errOut.String()}
	}()
	select {
	case a := <-listening:
		t.Cleanup(func() { cancel(); <-done })
		return a, &errOut, nil
	case r := <-done:
		cancel()
		return nil, nil, &r
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("serve neither listened nor exited")
		return nil, nil, nil
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func writeHomeSettings(t *testing.T, home, text string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "code-central")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTheLocalPortSettingMovesTheService(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	port := freePort(t)
	writeHomeSettings(t, home, fmt.Sprintf("port: %d\n", port))
	bound, _, failed := serveWith(t, map[string]string{"HOME": home}, nil)
	if failed != nil {
		t.Fatalf("serve: %+v", *failed)
	}
	if want := fmt.Sprintf("127.0.0.1:%d", port); bound[0] != want {
		t.Fatalf("bound %v, want %s first", bound, want)
	}
	// CODE_CENTRAL_URL then points the CLI at it.
	r := run(t, map[string]string{"CODE_CENTRAL_URL": fmt.Sprintf("http://localhost:%d", port)}, "", "get-workflow")
	if r.code != 0 {
		t.Fatalf("get-workflow: exit %d: %s", r.code, r.stderr)
	}
}

func TestXDGConfigHomeHoldsTheSettings(t *testing.T) {
	t.Parallel()
	cfg := t.TempDir()
	os.MkdirAll(filepath.Join(cfg, "code-central"), 0o755)
	os.WriteFile(filepath.Join(cfg, "code-central", "settings.yaml"), []byte("colour: red\n"), 0o644)
	_, _, failed := serveWith(t, map[string]string{"XDG_CONFIG_HOME": cfg, "HOME": t.TempDir()}, []string{"127.0.0.1:0"})
	if failed == nil || failed.code != 1 || !strings.Contains(failed.stderr, "settings.yaml") || !strings.Contains(failed.stderr, "colour") {
		t.Fatalf("serve accepted an unknown setting: %+v", failed)
	}
}

func TestWithNoHOMETheProfileFolderHoldsTheSettings(t *testing.T) {
	t.Parallel()
	profile := t.TempDir()
	writeHomeSettings(t, profile, "port: 70000\n")
	_, _, failed := serveWith(t, map[string]string{"USERPROFILE": profile}, nil)
	want := filepath.Join(profile, ".config", "code-central", "settings.yaml")
	if failed == nil || failed.code != 1 || !strings.Contains(failed.stderr, want) {
		t.Fatalf("serve didn't read %s: %+v", want, failed)
	}
}

func TestABadPortSettingRefusesToStart(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	writeHomeSettings(t, home, "port: 70000\n")
	_, _, failed := serveWith(t, map[string]string{"HOME": home}, nil)
	if failed == nil || failed.code != 1 || !strings.Contains(failed.stderr, "port") {
		t.Fatalf("serve accepted port 70000: %+v", failed)
	}
}

func TestLogAppendsServesOutputAndTheErrorItStopsWith(t *testing.T) {
	t.Parallel()
	log := filepath.Join(t.TempDir(), "code-central.log")
	os.WriteFile(log, []byte("an earlier run\n"), 0o644)
	bound, stderr, failed := serveWith(t, nil, []string{"127.0.0.1:0"}, "--log", log, "--bind", "127.0.0.1:0")
	if failed != nil {
		t.Fatalf("serve: %+v", *failed)
	}
	b, _ := os.ReadFile(log)
	for _, want := range []string{"an earlier run\n", "--bind: also listening on", "listening on " + fmt.Sprint(bound)} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("log lacks %q:\n%s", want, b)
		}
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr %q; want it all in the log", stderr)
	}

	home := t.TempDir()
	writeHomeSettings(t, home, "port: 70000\n")
	if _, _, failed = serveWith(t, map[string]string{"HOME": home}, nil, "--log", log); failed == nil || failed.code != 1 {
		t.Fatalf("serve accepted port 70000: %+v", failed)
	}
	if b, _ = os.ReadFile(log); !strings.Contains(string(b), "code-central serve: ") || !strings.Contains(string(b), "70000") {
		t.Fatalf("log lacks the error serve stopped with:\n%s", b)
	}
}

func TestBindAddsOneListenerAndWarnsAgainstLANAddresses(t *testing.T) {
	t.Parallel()
	bound, stderr, failed := serveWith(t, nil, []string{"127.0.0.1:0"}, "--bind", "127.0.0.1:0")
	if failed != nil {
		t.Fatalf("serve: %+v", *failed)
	}
	if len(bound) != 2 {
		t.Fatalf("bound %v, want the default and the --bind address", bound)
	}
	resp, err := http.Get("http://" + bound[1] + "/api/workflow")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Code-Central-Version") == "" {
		t.Fatalf("--bind listener: %d, version %q", resp.StatusCode, resp.Header.Get("X-Code-Central-Version"))
	}
	if !strings.Contains(stderr.String(), "LAN") {
		t.Fatalf("no LAN warning on stderr: %q", stderr.String())
	}
}

func TestBindRefusesEverythingButOneSpecificAddress(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"0.0.0.0", "::", "[::]:7733", "0.0.0.0:0", "example.com", ""} {
		_, _, failed := serveWith(t, nil, []string{"127.0.0.1:0"}, "--bind", bad)
		if failed == nil || failed.code != 1 {
			t.Fatalf("--bind %q: serve started", bad)
		}
	}
}

func TestServeUsageWarnsAgainstALANAddress(t *testing.T) {
	t.Parallel()
	r := run(t, nil, "", "help")
	if !strings.Contains(r.stdout, "--bind") || !strings.Contains(r.stdout, "never a LAN address") {
		t.Fatalf("help: %s", r.stdout)
	}
}
