//go:build e2e

// Package e2e is the end-to-end tier: the real binary, a real store in a
// temporary folder and real processes. Run it with `make e2e`, which
// builds the binaries and passes them in CODE_CENTRAL_BIN and
// CODE_CENTRAL_FACTORY_BIN, and the fake gh in CODE_CENTRAL_FAKE_GH_BIN.
package e2e

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

var bin, factoryBin, fakeGHBin string

// TestMain finds the binaries, building any not passed in, and runs the
// tests. Run by the Factory as claude, it is the fake claude instead.
func TestMain(m *testing.M) {
	if cc := os.Getenv(fakeClaudeEnv); cc != "" {
		os.Exit(fakeClaude(cc, os.Args[1:]))
	}
	bin = os.Getenv("CODE_CENTRAL_BIN")
	factoryBin = os.Getenv("CODE_CENTRAL_FACTORY_BIN")
	fakeGHBin = os.Getenv("CODE_CENTRAL_FAKE_GH_BIN")
	var dir string
	build := func(pkg, name string) string {
		if dir == "" {
			var err error
			if dir, err = os.MkdirTemp("", "code-central-e2e"); err != nil {
				panic(err)
			}
		}
		out := filepath.Join(dir, name+exeSuffix)
		cmd := exec.Command("go", "build", "-o", out, pkg)
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		if err := cmd.Run(); err != nil {
			panic(err)
		}
		return out
	}
	if bin == "" {
		bin = build("../cmd/code-central", "code-central")
	}
	if factoryBin == "" {
		factoryBin = build("../cmd/code-central-factory", "code-central-factory")
	}
	if fakeGHBin == "" {
		fakeGHBin = build("../internal/fakegh/gh", "fake-gh")
	}
	code := m.Run()
	if dir != "" {
		os.RemoveAll(dir)
	}
	os.Exit(code)
}

// service is a running `code-central serve` on a free loopback port.
type service struct {
	url string
	env []string
}

var listening = regexp.MustCompile(`listening on \[([^\] ]+)`)

func serve(t *testing.T) *service {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "serve", "--store", t.TempDir(), "--no-notify") // tests never show a real OS Notification
	cmd.Env = append(os.Environ(), "CODE_CENTRAL_LISTEN=127.0.0.1:0")
	startInOwnGroup(cmd)
	cmd.Cancel = func() error { return interrupt(cmd.Process) }
	cmd.WaitDelay = 15 * time.Second // then killed
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("serve printed %q: %v", line, err)
	}
	m := listening.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("serve printed %q", line)
	}
	go io.Copy(io.Discard, stdout)
	u := "http://" + m[1]
	return &service{url: u, env: append(os.Environ(), "CODE_CENTRAL_URL="+u, "CODE_CENTRAL_PROJECT=", "CLAUDE_CODE_SESSION_ID=")}
}

// cli runs one command in dir and returns its exit code and stdout.
func (s *service) cli(t *testing.T, dir, stdin string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir, cmd.Env, cmd.Stdin = dir, s.env, strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Logf("%v: exit %d, stderr: %s", args, code, errOut.String())
	}
	return code, out.String()
}

// human makes one HTTP request as the human, as the UI does.
func (s *service) human(t *testing.T, path, body string) map[string]any {
	t.Helper()
	req, _ := http.NewRequest("POST", s.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Code-Central-Actor", "human")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode != 200 {
		t.Fatalf("POST %s: %d %v", path, resp.StatusCode, out)
	}
	return out
}

// tool calls one MCP tool and decodes its JSON result.
func tool(t *testing.T, s *sdk.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()
	res, err := s.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(*sdk.TextContent).Text
	if res.IsError {
		t.Fatalf("%s: %s", name, text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// 1. The agent loop.
func TestTheAgentLoop(t *testing.T) {
	svc := serve(t)
	repo := filepath.Join(t.TempDir(), "acme")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}

	// init sets the repository up as Project acme and writes .mcp.json.
	if code, out := svc.cli(t, repo, "\n", "init", "--key", "acme"); code != 0 {
		t.Fatalf("init: exit %d: %s", code, out)
	}
	var mcpFile struct {
		MCPServers map[string]struct{ URL string } `json:"mcpServers"`
	}
	data, err := os.ReadFile(filepath.Join(repo, ".mcp.json"))
	if err != nil || json.Unmarshal(data, &mcpFile) != nil {
		t.Fatalf(".mcp.json: %v %s", err, data)
	}
	endpoint := strings.ReplaceAll(mcpFile.MCPServers["code-central"].URL, "${CODE_CENTRAL_URL:-http://localhost:7733}", svc.url)
	if !strings.HasSuffix(endpoint, "/p/acme/mcp") {
		t.Fatalf("MCP URL %q", endpoint)
	}

	// Some work to claim, made from the repository (the key comes from .mcp.json).
	if code, _ := svc.cli(t, repo, "", "create-spec", "--title", "Login"); code != 0 {
		t.Fatal("create-spec failed")
	}
	if code, _ := svc.cli(t, repo, "", "create-ticket", "--spec", "1", "--title", "Login form", "--state", "Ready"); code != 0 {
		t.Fatal("create-ticket failed")
	}

	// The agent claims and asks over MCP.
	agent, err := sdk.NewClient(&sdk.Implementation{Name: "e2e-agent", Version: "v0"}, nil).Connect(context.Background(),
		&sdk.StreamableClientTransport{Endpoint: endpoint, DisableStandaloneSSE: true, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer agent.Close()
	claimed := tool(t, agent, "claim", map[string]any{"session_id": "sess-e2e"})
	claimID, _ := claimed["claim_id"].(string)
	if id := claimed["ticket"].(map[string]any)["id"]; id != "acme-2" || claimID == "" {
		t.Fatalf("claim: %v", claimed)
	}
	posted := tool(t, agent, "post_round", map[string]any{"ticket": "acme-2", "claim_id": claimID, "questions": []map[string]any{
		{"title": "Which database?", "body": "Postgres or SQLite?", "recommended": "SQLite"},
		{"title": "Remember me?", "body": "Add a remember-me box?"},
	}})
	if !strings.Contains(posted["next_step"].(string), "code-central wait acme-2") {
		t.Fatalf("post_round: %v", posted)
	}

	// The agent's background wait, as its own process.
	type exit struct {
		code int
		out  string
	}
	waited := make(chan exit, 1)
	go func() {
		code, out := svc.cli(t, repo, "", "wait", "acme-2")
		waited <- exit{code, out}
	}()

	// The human answers over HTTP, one at a time.
	if v := svc.human(t, "/api/p/acme/tickets/2/accept_recommended", `{"round":1}`); v["state"] != "Needs Answer" {
		t.Fatalf("accept_recommended left Question 2 open but moved to %v", v["state"])
	}
	svc.human(t, "/api/p/acme/tickets/2/answer", `{"question":2,"text":"No."}`)

	var w exit
	select {
	case w = <-waited:
	case <-time.After(30 * time.Second):
		t.Fatal("wait did not exit after the last answer")
	}
	if w.code != 0 || !strings.Contains(w.out, "pick_up_answers") {
		t.Fatalf("wait: exit %d: %s", w.code, w.out)
	}

	// The same session picks up, finishes, and the human accepts.
	picked := tool(t, agent, "pick_up_answers", map[string]any{"ticket": "acme-2", "claim_id": claimID})
	qs := picked["round"].(map[string]any)["questions"].([]any)
	if qs[0].(map[string]any)["answer"] != "SQLite" || qs[1].(map[string]any)["answer"] != "No." {
		t.Fatalf("pick_up_answers: %v", picked)
	}
	finished := tool(t, agent, "finish", map[string]any{"ticket": "acme-2", "claim_id": claimID, "summary": "Built on branch acme-2."})
	if finished["state"] != "In Review" {
		t.Fatalf("finish: %v", finished)
	}
	if done := svc.human(t, "/api/p/acme/tickets/2/accept", `{}`); done["state"] != "Done" {
		t.Fatalf("accept: %v", done)
	}
}
