package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tomoram/code-central/cli"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
)

// recorder is a notify.Sender that keeps what it was asked to show.
type recorder struct{ sent chan notify.Notification }

func (r recorder) Send(n notify.Notification) error {
	r.sent <- n
	return nil
}

// storeInReview is a store whose acme-2 is In Review, so Needs Me is not
// empty when serve starts on it.
func storeInReview(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	c := core.New(st, clock.System{}, ids.Random{})
	ctx := context.Background()
	c.CreateProject(ctx, core.CreateProjectInput{Key: "acme"})
	c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login", Body: "b\n"})
	c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: store.Ready})
	res, err := c.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "done"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// serveNotifying runs serve with a recording Sender and env, waits until
// it listens (by when the startup summary has been sent), stops it, and
// returns what it sent plus serve's result.
func serveNotifying(t *testing.T, storeDir string, env map[string]string, args ...string) ([]notify.Notification, result, []string) {
	t.Helper()
	rec := recorder{sent: make(chan notify.Notification, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listening := make(chan []string, 1)
	done := make(chan result, 1)
	go func() {
		var out, errOut bytes.Buffer
		code := cli.Run(ctx, append([]string{"serve", "--store", storeDir}, args...), cli.Env{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
			Getenv:      func(k string) string { return env[k] },
			ServeAddrs:  []string{"127.0.0.1:0"},
			OnListening: func(a []string) { listening <- a },
			Notifier:    rec,
		})
		done <- result{code, out.String(), errOut.String()}
	}()
	var bound []string
	select {
	case bound = <-listening:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("serve neither listened nor exited")
	case r := <-done:
		return nil, r, nil
	}
	r := <-done
	close(rec.sent)
	var sent []notify.Notification
	for n := range rec.sent {
		sent = append(sent, n)
	}
	return sent, r, bound
}

func TestServeSendsTheStartupSummaryWhenNeedsMeIsNotEmpty(t *testing.T) {
	t.Parallel()
	sent, r, bound := serveNotifying(t, storeInReview(t), nil)
	if r.code != 0 {
		t.Fatalf("serve exited %d: %s", r.code, r.stderr)
	}
	port := bound[0][strings.LastIndex(bound[0], ":")+1:]
	want := notify.Notification{Title: "Code Central", Body: "1 Ticket needs you in acme", URL: "http://localhost:" + port + "/?view=inbox"}
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("sent %+v, want %+v", sent, want)
	}
}

func TestServeNoNotifyTurnsNotificationsOffForOneRun(t *testing.T) {
	t.Parallel()
	sent, r, _ := serveNotifying(t, storeInReview(t), nil, "--no-notify")
	if r.code != 0 || len(sent) != 0 {
		t.Fatalf("exit %d, sent %+v: %s", r.code, sent, r.stderr)
	}
}

// writeSettings writes the local settings file under a config home and
// returns the environment that finds it.
func writeSettings(t *testing.T, body string) (map[string]string, string) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "code-central", "settings.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"XDG_CONFIG_HOME": home}, path
}

func TestTheNotificationsSettingTurnsThemOff(t *testing.T) {
	t.Parallel()
	env, _ := writeSettings(t, "notifications: off\n")
	sent, r, _ := serveNotifying(t, storeInReview(t), env)
	if r.code != 0 || len(sent) != 0 {
		t.Fatalf("exit %d, sent %+v: %s", r.code, sent, r.stderr)
	}

	env, _ = writeSettings(t, "notifications: on\n")
	if sent, r, _ := serveNotifying(t, storeInReview(t), env); r.code != 0 || len(sent) != 1 {
		t.Fatalf("on: exit %d, sent %+v: %s", r.code, sent, r.stderr)
	}
}

func TestServeRefusesABadSettingNamingTheFile(t *testing.T) {
	t.Parallel()
	env, path := writeSettings(t, "notifications: sometimes\n")
	_, r, _ := serveNotifying(t, t.TempDir(), env)
	if r.code != 1 || !strings.Contains(r.stderr, path) || !strings.Contains(r.stderr, "notifications") {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
}
