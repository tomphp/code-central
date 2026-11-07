package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
)

// runVersion runs one command as a CLI of the given version.
func runVersion(t *testing.T, version string, env map[string]string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	code := cli.Run(context.Background(), args, cli.Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
		Getenv:  func(k string) string { return env[k] },
		Dir:     t.TempDir(),
		Version: version,
	})
	return result{code, out.String(), errOut.String()}
}

func TestVersionFlagPrintsTheStampedVersion(t *testing.T) {
	r := runVersion(t, "v1.2.0-3-gabc123-dirty", nil, "--version")
	if r.code != 0 || r.stdout != "code-central v1.2.0-3-gabc123-dirty\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func TestTheServiceSendsItsVersionInAResponseHeader(t *testing.T) {
	srv := codecentraltest.NewServer(t, codecentraltest.WithVersion("v1.4.0"))
	resp, err := srv.Client.HTTP.Get(srv.URL + "/api/workflow")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := resp.Header.Get("X-Code-Central-Version"); got != "v1.4.0" {
		t.Fatalf("version header %q", got)
	}
}

func TestTheCLIWarnsWithoutFailingWhenVersionsDiffer(t *testing.T) {
	for _, tc := range []struct {
		name, cli, service, want string
	}{
		{"service older", "v1.3.0", "v1.2.9", "run `code-central service restart`"},
		{"cli older", "v1.2.0", "v1.10.0", "this CLI is older than the service"},
		{"not both clean tags", "v1.2.0-4-gdeadbee", "v1.2.0", "differs"},
		{"dirty", "v1.3.0-dirty", "v1.2.0", "differs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := codecentraltest.NewServer(t, codecentraltest.WithVersion(tc.service))
			if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
				t.Fatal(err)
			}
			env := map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}
			r := runVersion(t, tc.cli, env, "create-spec", "--title", "Login")
			spec := decode[core.SpecView](t, r)
			if spec.ID != "acme-1" {
				t.Fatalf("spec %+v", spec)
			}
			if strings.Count(r.stderr, "\n") != 1 || !strings.Contains(r.stderr, tc.want) ||
				!strings.Contains(r.stderr, tc.cli) || !strings.Contains(r.stderr, tc.service) {
				t.Fatalf("stderr %q, want one line containing %q and both versions", r.stderr, tc.want)
			}
			if tc.want == "differs" && (strings.Contains(r.stderr, "older") || strings.Contains(r.stderr, "restart")) {
				t.Fatalf("named an older side for non-tag versions: %q", r.stderr)
			}
		})
	}
}

func TestTheCLIIsQuietWhenVersionsMatch(t *testing.T) {
	srv := codecentraltest.NewServer(t, codecentraltest.WithVersion("v1.2.0"))
	if _, err := srv.Client.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"}); err != nil {
		t.Fatal(err)
	}
	r := runVersion(t, "v1.2.0", map[string]string{"CODE_CENTRAL_URL": srv.URL, "CODE_CENTRAL_PROJECT": "acme"}, "get-ticket", "1")
	if strings.Contains(r.stderr, "warning") {
		t.Fatalf("stderr %q", r.stderr)
	}
}

func TestPingStaysSilentOnAVersionMismatch(t *testing.T) {
	srv := codecentraltest.NewServer(t, codecentraltest.WithVersion("v9.0.0"))
	r := runVersion(t, "v1.0.0", map[string]string{"CODE_CENTRAL_URL": srv.URL}, "ping", "--session", "s1")
	if r.code != 0 || r.stdout != "" || r.stderr != "" {
		t.Fatalf("ping: exit %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}
