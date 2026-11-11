package frontdoor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/codecentraltest"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/frontdoor"
)

// An unknown field's message, as the service of each version case gives
// it for create_spec sent "added_later".
const unknownAddedLater = `invalid JSON: json: unknown field "added_later"`

var acceptedSection = regexp.MustCompile(`: accepted fields are [a-z_]+(, [a-z_]+)*`)

var versionCases =[]struct {
	name, client, want string
}{
	{"a different version", "v1.9", unknownAddedLater + ": the caller is version v1.9 and the service is v1.8; run `code-central service restart`"},
	{"the same version", "v1.8", unknownAddedLater},
	{"an unknown version", "", unknownAddedLater + " (the service is v1.8; if your client is newer, run `code-central service restart`)"},
}

func versionedServer(t *testing.T) *codecentraltest.Server {
	t.Helper()
	srv := codecentraltest.NewServer(t, codecentraltest.WithVersion("v1.8"))
	must(t, func() error {
		_, err := srv.Core.CreateProject(context.Background(), core.CreateProjectInput{Key: "acme"})
		return err
	})
	return srv
}

func checkUnknownAddedLater(t *testing.T, e core.Error, want string) {
	t.Helper()
	// The accepted fields differ by tool: only the version hint is under test.
	got := acceptedSection.ReplaceAllString(e.Message, "")
	if got != want || e.Field != "added_later" || got == e.Message {
		t.Fatalf("got %q field %q, want %q after its accepted fields", e.Message, e.Field, want)
	}
}

func TestMCPUnknownFieldsNameTheVersionsSentAsClientVersion(t *testing.T) {
	srv := versionedServer(t)
	s := connect(t, srv, "acme")
	for _, tc := range versionCases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]any{"title": "S", "added_later": 1}
			if tc.client != "" {
				args["client_version"] = tc.client
			}
			checkUnknownAddedLater(t, mcpRejected(t, s, "create_spec", args, core.Invalid), tc.want)
		})
	}
	// client_version alone is accepted on every tool, and isn't advertised.
	out := mcpOK(t, s, "create_spec", map[string]any{"title": "S", "client_version": "v1.9"})
	field("id", "acme-1")(t, out)
	tools, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		if strings.Contains(string(b), "client_version") {
			t.Errorf("%s's schema lists client_version", tool.Name)
		}
	}
}

func TestMCPUnknownFieldsOnAHookedCallNameTheHooksVersion(t *testing.T) {
	srv := versionedServer(t)
	s := connect(t, srv, "acme")
	// A newer agent's claim, with a field this service doesn't know, as the
	// session-id hook of a v1.9 CLI rewrites it.
	var out bytes.Buffer
	stdin := `{"session_id":"abc","tool_name":"mcp__code-central__claim","tool_input":{"added_later":1}}`
	if exit := cli.Run(context.Background(), []string{"hook", "session-id"}, cli.Env{
		Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: io.Discard,
		Getenv: func(string) string { return "" }, Version: "v1.9",
	}); exit != 0 {
		t.Fatalf("hook exit %d", exit)
	}
	var hook struct {
		HookSpecificOutput struct {
			UpdatedInput map[string]any `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(out.Bytes(), &hook); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	e := mcpRejected(t, s, "claim", hook.HookSpecificOutput.UpdatedInput, core.Invalid)
	checkUnknownAddedLater(t, e, versionCases[0].want)
}

func TestHTTPUnknownFieldsNameTheVersionsSentAsTheHeader(t *testing.T) {
	srv := versionedServer(t)
	for _, tc := range versionCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("POST", srv.URL+"/api/p/acme/create_spec", strings.NewReader(`{"title":"S","added_later":1}`))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set(frontdoor.ActorHeader, string(core.Human))
			if tc.client != "" {
				req.Header.Set(frontdoor.ClientVersionHeader, tc.client)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			checkUnknownAddedLater(t, errorJSON(t, b, core.Invalid), tc.want)
		})
	}
}

// The CLI only sends fields its service knows, unless the service is
// older: newerCLI stands in for a newer CLI's requests by adding a field
// to every JSON body on the way, leaving the CLI's headers as they are.
func newerCLI(t *testing.T, srv *codecentraltest.Server) string {
	t.Helper()
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	direct := proxy.Director
	proxy.Director = func(r *http.Request) {
		direct(r)
		if r.Body == nil || r.Method == http.MethodGet {
			return
		}
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Errorf("%s %s: body is not an object", r.Method, r.URL.Path)
		}
		body["added_later"] = 1
		b, _ := json.Marshal(body)
		r.Body, r.ContentLength = io.NopCloser(bytes.NewReader(b)), int64(len(b))
	}
	hs := httptest.NewServer(proxy)
	t.Cleanup(hs.Close)
	return hs.URL
}

func TestCLIUnknownFieldsNameTheVersionsSentAsTheHeader(t *testing.T) {
	srv := versionedServer(t)
	env := map[string]string{"CODE_CENTRAL_URL": newerCLI(t, srv), "CODE_CENTRAL_PROJECT": "acme"}
	for _, tc := range versionCases[:2] { // the CLI always knows its version
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			exit := cli.Run(context.Background(), []string{"create-spec", "--title", "S"}, cli.Env{
				Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
				Getenv:  func(k string) string { return env[k] },
				Version: tc.client,
			})
			if exit != 2 || out.Len() != 0 {
				t.Fatalf("exit %d, stdout %q; want exit 2 and no output", exit, out.String())
			}
			// A different version also warns on stderr, before the error.
			lines := strings.Split(strings.TrimSpace(errOut.String()), "\n")
			checkUnknownAddedLater(t, errorJSON(t, []byte(lines[len(lines)-1]), core.Invalid), tc.want)
		})
	}
}
