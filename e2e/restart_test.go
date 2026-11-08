//go:build e2e

package e2e

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// process is one `code-central serve` the test stops and starts itself.
type process struct {
	cmd    *exec.Cmd
	addr   string
	exited chan error
}

// startServe runs serve on store, listening on listen (never 7733).
func startServe(t *testing.T, store, listen string) (*process, string) {
	t.Helper()
	return startServeBin(t, bin, store, listen)
}

// startServeBin is startServe with another binary, such as a migrationtest build.
func startServeBin(t *testing.T, binary, store, listen string) (*process, string) {
	t.Helper()
	unixOnly(t, "os.Interrupt to stop serve")
	cmd := exec.Command(binary, "serve", "--store", store, "--no-notify")
	cmd.Env = append(os.Environ(), "CODE_CENTRAL_LISTEN="+listen)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &process{cmd: cmd, exited: make(chan error, 1)}
	line, _ := bufio.NewReader(stdout).ReadString('\n')
	go func() { io.Copy(io.Discard, stdout); p.exited <- cmd.Wait() }()
	m := listening.FindStringSubmatch(line)
	if m == nil {
		select {
		case <-p.exited:
		case <-time.After(10 * time.Second):
			cmd.Process.Kill()
		}
		return nil, stderr.String()
	}
	p.addr = m[1]
	t.Cleanup(func() { cmd.Process.Kill() })
	return p, ""
}

// stop interrupts serve, as launchd and systemd do, and waits for a clean exit.
func (p *process) stop(t *testing.T) {
	t.Helper()
	p.cmd.Process.Signal(os.Interrupt)
	select {
	case err := <-p.exited:
		if err != nil {
			t.Fatalf("serve did not stop cleanly: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
	}
}

// 2. Restart during a wait: wait resumes, state survives, and a second
// serve on the same store refuses to start.
func TestRestartDuringAWait(t *testing.T) {
	store := t.TempDir()
	first, msg := startServe(t, store, "127.0.0.1:0")
	if first == nil {
		t.Fatalf("serve: %s", msg)
	}
	svc := &service{url: "http://" + first.addr, env: append(os.Environ(),
		"CODE_CENTRAL_URL=http://"+first.addr, "CODE_CENTRAL_PROJECT=", "CLAUDE_CODE_SESSION_ID=")}

	repo := filepath.Join(t.TempDir(), "acme")
	os.Mkdir(repo, 0o755)
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if code, out := svc.cli(t, repo, "\n", "init", "--key", "acme"); code != 0 {
		t.Fatalf("init: exit %d: %s", code, out)
	}
	svc.cli(t, repo, "", "create-spec", "--title", "Login")
	svc.cli(t, repo, "", "create-ticket", "--spec", "1", "--title", "Login form", "--state", "Ready")
	code, out := svc.cli(t, repo, "", "claim", "--session", "sess-restart")
	var claimed struct {
		ClaimID string `json:"claim_id"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &claimed) != nil || claimed.ClaimID == "" {
		t.Fatalf("claim: exit %d: %s", code, out)
	}
	if code, out := svc.cli(t, repo, "", "post-round", "acme-2", "--claim", claimed.ClaimID,
		"--questions", `[{"title":"Which database?","body":"Postgres or SQLite?","recommended":"SQLite"}]`); code != 0 {
		t.Fatalf("post-round: exit %d: %s", code, out)
	}

	type exit struct {
		code int
		out  string
	}
	waited := make(chan exit, 1)
	go func() {
		code, out := svc.cli(t, repo, "", "wait", "acme-2")
		waited <- exit{code, out}
	}()

	// The service stops while the agent waits; wait keeps going.
	first.stop(t)
	select {
	case w := <-waited:
		t.Fatalf("wait exited %d while the service was down: %s", w.code, w.out)
	default:
	}

	// It starts again on the same address and store.
	second, msg := startServe(t, store, first.addr)
	if second == nil {
		t.Fatalf("restart: %s", msg)
	}

	// A second serve on the same store refuses to start.
	if third, msg := startServe(t, store, "127.0.0.1:0"); third != nil || !strings.Contains(msg, "locked") {
		t.Fatalf("a second serve on the same store started, or said %q", msg)
	}

	// The state survived the restart.
	code, out = svc.cli(t, repo, "", "get-ticket", "acme-2")
	var tk struct {
		State  string `json:"state"`
		Rounds []struct {
			Questions []struct{ Title string } `json:"questions"`
		} `json:"rounds"`
		Claim *struct {
			ID string `json:"id"`
		} `json:"claim"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &tk) != nil || tk.State != "Needs Answer" ||
		len(tk.Rounds) != 1 || tk.Rounds[0].Questions[0].Title != "Which database?" {
		t.Fatalf("after restart: exit %d: %s", code, out)
	}

	// The human answers; the same wait wakes.
	svc.human(t, "/api/p/acme/tickets/2/accept_recommended", `{"round":1}`)
	select {
	case w := <-waited:
		if w.code != 0 || !strings.Contains(w.out, "pick_up_answers") {
			t.Fatalf("wait: exit %d: %s", w.code, w.out)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("wait did not resume after the restart")
	}
	second.stop(t)
}
