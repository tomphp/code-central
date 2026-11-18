package app_test

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

	"github.com/tomoram/code-central/app"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/clock"
	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/domain"
	"github.com/tomoram/code-central/ids"
	"github.com/tomoram/code-central/notify"
	"github.com/tomoram/code-central/store"
)

// running is a service started by app.Run in-process.
type running struct {
	bound          []string
	stdout, stderr *bytes.Buffer
	cancel         context.CancelFunc
	done           chan error
}

// filesAt is the File Store at dir, as the service opens it.
func filesAt(dir string) store.Location { return store.Location{Kind: store.Files, Path: dir} }

// start runs the service with o, filling in a test store, a test-chosen
// address (never 7733) and output buffers where o leaves them out, and
// waits until it listens. It returns Run's error instead if Run stops
// first.
func start(t *testing.T, o app.Options) (*running, error) {
	t.Helper()
	if o.Store == (store.Location{}) {
		o.Store = filesAt(t.TempDir())
	}
	if o.Addrs == nil {
		o.Addrs = []string{"127.0.0.1:0"}
	}
	r := &running{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}, done: make(chan error, 1)}
	o.Stdout, o.Stderr = r.stdout, r.stderr
	listening := make(chan []string, 1)
	o.OnListening = func(a []string) { listening <- a }
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	go func() { r.done <- app.Run(ctx, o) }()
	select {
	case r.bound = <-listening:
		t.Cleanup(func() { r.stop(t) })
		return r, nil
	case err := <-r.done:
		cancel()
		return nil, err
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the service neither listened nor stopped")
		return nil, nil
	}
}

// stop ends the service and fails the test if it stopped with an error.
func (r *running) stop(t *testing.T) {
	if r.cancel == nil {
		return
	}
	r.cancel()
	r.cancel = nil
	select {
	case err := <-r.done:
		if err != nil {
			t.Errorf("the service stopped with %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Error("the service did not stop")
	}
}

func (r *running) url() string { return "http://" + r.bound[0] }

func TestRunServesTheAPIAndSaysWhereItListens(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := start(t, app.Options{Store: filesAt(dir), Version: "v9.9.9"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(r.url() + "/api/workflow")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("X-Code-Central-Version") != "v9.9.9" {
		t.Fatalf("got %d, version %q", resp.StatusCode, resp.Header.Get("X-Code-Central-Version"))
	}
	want := fmt.Sprintf("Code Central v9.9.9 listening on %v (store: files at %s)\n", r.bound, dir)
	if r.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", r.stdout, want)
	}
	// Shutting down stops the Watchers' supervision too, so Run returns.
	r.stop(t)
}

func TestATicketSurvivesARestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	r, err := start(t, app.Options{Store: filesAt(dir)})
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(r.url())
	if _, err := c.CreateProject(ctx, core.CreateProjectInput{Key: "acme", DisplayName: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Login"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: domain.Ready}); err != nil {
		t.Fatal(err)
	}
	r.stop(t)

	for _, f := range []string{"store.yaml", "projects/acme/project.yaml", "projects/acme/specs/1.md", "projects/acme/tickets/2.yaml"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}

	r2, err := start(t, app.Options{Store: filesAt(dir)})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	c = client.New(r2.url())
	got, err := c.GetTicket(ctx, "acme", "acme-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Form" || got.State != "Ready" {
		t.Fatalf("after restart: %+v", got)
	}
	// the counter is derived again at startup
	next, err := c.CreateSpec(ctx, "acme", core.CreateSpecInput{Title: "Next", Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if next.Record == nil || next.Record.ID != "acme-3" {
		t.Fatalf("after restart, next %+v", next)
	}
}

func TestRunRefusesATakenPort(t *testing.T) {
	t.Parallel()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, err = start(t, app.Options{Addrs: []string{l.Addr().String()}})
	if err == nil {
		t.Fatal("the service started on a taken port")
	}
	if !strings.Contains(err.Error(), l.Addr().String()) || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("error %q", err)
	}
}

// Run opens the store before it binds: a store it can't open (here, one
// already open elsewhere) stops it before it listens. Why a store can't
// be opened is the store's to test.
func TestRunStopsBeforeListeningWhenItCannotOpenTheStore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	held, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	_, err = start(t, app.Options{Store: filesAt(dir)})
	if err == nil {
		t.Fatal("the service listened on a store it could not open")
	}
	if !strings.Contains(err.Error(), "cannot open the store") {
		t.Fatalf("error %q", err)
	}
}

func TestBindAddsOneListenerAndWarnsAgainstLANAddresses(t *testing.T) {
	t.Parallel()
	r, err := start(t, app.Options{Bind: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.bound) != 2 {
		t.Fatalf("bound %v, want the default and the --bind address", r.bound)
	}
	resp, err := http.Get("http://" + r.bound[1] + "/api/workflow")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("--bind listener: %d", resp.StatusCode)
	}
	if !strings.Contains(r.stderr.String(), "--bind: also listening on "+r.bound[1]) || !strings.Contains(r.stderr.String(), "LAN") {
		t.Fatalf("no LAN warning on stderr: %q", r.stderr)
	}
}

// recorder is a notify.Sender that keeps what it was asked to show.
type recorder struct{ sent chan notify.Notification }

func (r recorder) Send(n notify.Notification) error {
	r.sent <- n
	return nil
}

// storeInReview is a store whose acme-2 is In Review, so Needs Me is not
// empty when the service starts on it.
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
	c.CreateTicket(ctx, "acme", core.CreateTicketInput{Spec: "1", Title: "Form", State: domain.Ready})
	res, err := c.Claim(ctx, "acme", core.ClaimInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Finish(ctx, "acme", core.FinishInput{Ticket: "2", ClaimID: res.ClaimID, Summary: "done"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The startup summary has been sent by the time the service says it
// listens.
func TestRunSendsTheStartupSummaryWhenNeedsMeIsNotEmpty(t *testing.T) {
	t.Parallel()
	rec := recorder{sent: make(chan notify.Notification, 8)}
	r, err := start(t, app.Options{Store: filesAt(storeInReview(t)), Notifier: rec})
	if err != nil {
		t.Fatal(err)
	}
	r.stop(t)
	close(rec.sent)
	var sent []notify.Notification
	for n := range rec.sent {
		sent = append(sent, n)
	}
	port := r.bound[0][strings.LastIndex(r.bound[0], ":")+1:]
	want := notify.Notification{Title: "Code Central", Body: "1 Ticket needs you in acme", URL: "http://localhost:" + port + "/?view=inbox"}
	if len(sent) != 1 || sent[0] != want {
		t.Fatalf("sent %+v, want %+v", sent, want)
	}
}
