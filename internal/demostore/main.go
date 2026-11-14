// Command demostore runs a scratch Code Central filled with a demo Project,
// so the UI has something on every Board lane, Focus Item and Inbox list
// (`make demo-store`). It starts `code-central serve --store <dir>
// --no-notify` from the binary named by -bin on a loopback port that is
// never 7733, seeds it through the HTTP API, prints the URL to open and the
// CODE_CENTRAL_URL for the Vite dev server, and stays in the foreground
// until it is interrupted, then stops the service. A store that already
// holds the demo Project is reused, not seeded again.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

const projectKey = "demo"

func main() {
	bin := flag.String("bin", "bin/code-central", "the code-central binary")
	port := flag.Int("port", 7801, "the loopback port to serve on (not 7733)")
	store := flag.String("store", filepath.Join(os.TempDir(), "code-central-demo-store"), "the scratch store; reused when it holds the demo Project")
	flag.Parse()
	if err := run(*bin, *port, *store); err != nil {
		fmt.Fprintln(os.Stderr, "demostore:", err)
		os.Exit(1)
	}
}

func run(bin string, port int, store string) error {
	if port == 7733 {
		return fmt.Errorf("port 7733 is the real service's; pick another")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// The service kills nothing it did not start: a port in use is an error.
	cmd := exec.CommandContext(ctx, bin, "serve", "--store", store, "--no-notify")
	cmd.Env = append(os.Environ(), "CODE_CENTRAL_LISTEN="+"127.0.0.1:"+strconv.Itoa(port))
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	listening := make(chan string, 1)
	go func() {
		re := regexp.MustCompile(`listening on \[([^\] ]+)`)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				listening <- "http://" + m[1]
			}
		}
	}()
	var url string
	select {
	case url = <-listening:
	case err := <-exited:
		return fmt.Errorf("serve exited before listening: %v", err)
	}

	d := &demo{url: url}
	seeded, err := d.seed()
	if err != nil {
		stop()
		<-exited
		return err
	}
	verb := "Reusing"
	if seeded {
		verb = "Seeded"
	}
	fmt.Printf("%s the demo Project %q in %s\n\n", verb, projectKey, store)
	fmt.Printf("Open the embedded UI:        %s/p/%s\n", url, projectKey)
	fmt.Printf("Or serve the source on 5173: CODE_CENTRAL_URL=%s npm --prefix web run dev\n", url)
	fmt.Println("Interrupt to stop the service.")

	select {
	case <-ctx.Done():
		return <-exited
	case err := <-exited:
		return fmt.Errorf("serve exited: %v", err)
	}
}

type demo struct{ url string }

// post makes one POST as actor ("agent" or "human") and decodes the answer.
func (d *demo) post(actor, path string, body any) (map[string]any, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", d.url+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Code-Central-Actor", actor)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", path, data)
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("%s: %v: %s", path, err, data)
	}
	return v, nil
}

// seeder carries the first error, so the seeding below reads as a script.
type seeder struct {
	d   *demo
	err error
}

func (s *seeder) agent(path string, body any) map[string]any { return s.as("agent", path, body) }
func (s *seeder) human(path string, body any) map[string]any { return s.as("human", path, body) }

func (s *seeder) as(actor, path string, body any) map[string]any {
	if s.err != nil {
		return nil
	}
	v, err := s.d.post(actor, "/api/p/"+projectKey+path, body)
	s.err = err
	return v
}

// n is the number of a Ticket's ID, as the routes name it.
func n(id string) string { return id[strings.LastIndex(id, "-")+1:] }

// str digs a string out of an Acknowledgement or record by its path.
func str(v map[string]any, path ...string) string {
	var cur any = v
	for _, p := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[p]
	}
	s, _ := cur.(string)
	return s
}

// seed fills a store with the demo Project, and reports whether it did:
// false when the store already had it.
func (d *demo) seed() (bool, error) {
	res, err := http.Get(d.url + "/api/projects/" + projectKey)
	if err != nil {
		return false, err
	}
	res.Body.Close()
	if res.StatusCode == http.StatusOK {
		return false, nil
	}
	if _, err := d.post("agent", "/api/projects/create_project", map[string]any{"key": projectKey, "display_name": "Demo"}); err != nil {
		return false, err
	}
	s := &seeder{d: d}

	// A Requirement in each Stage. The first two Tickets of a Requirement
	// are its Stage Tickets: the grilling Stage's, when it has one, then the
	// spec Stage's.
	s.agent("/submit_requirement", map[string]any{"text": "Let people sign in with a magic link\n\nNo password: we email a link that signs them in.", "full": true})
	s.agent("/submit_requirement", map[string]any{"text": "Show a usage chart on the account page\n\nA small chart of the last 30 days.", "skip_grilling": true})
	atTickets := s.agent("/submit_requirement", map[string]any{"text": "Export a Project's tickets as CSV\n\nOne row per Ticket.", "skip_grilling": true, "full": true})
	if s.err == nil {
		specStage := stageTicket(atTickets, "spec")
		claim := s.agent("/claim", map[string]any{"ticket": specStage, "session_id": "demo-spec-stage"})
		s.agent("/tickets/"+n(specStage)+"/finish", map[string]any{"claim_id": str(claim, "claim_id"), "summary": "The Spec is written.", "recommendation": "split"})
		s.human("/tickets/"+n(specStage)+"/accept", map[string]any{})
	}

	// A Map, and a Small Change.
	s.agent("/submit_map", map[string]any{"text": "Where should notifications live?\n\nChart the options before building any."})
	s.agent("/submit_small_change", map[string]any{"text": "Fix the typo on the settings page\n\n\"Recieve\" should be \"Receive\"."})

	// A Spec with a Ticket in each State.
	spec := str(s.agent("/create_spec", map[string]any{"title": "Dark mode", "body": "Add a dark theme the human can switch to.", "full": true}), "id")
	ticket := func(title string, state string) string {
		return str(s.agent("/create_ticket", map[string]any{"spec": spec, "title": title, "state": state, "description": title + "."}), "id")
	}
	claim := func(id, session string) string {
		return str(s.agent("/claim", map[string]any{"ticket": id, "session_id": session}), "claim_id")
	}
	ticket("Sketch the palette", "Draft")
	ticket("Add the theme switch", "Ready")
	claim(ticket("Theme the Board", "Ready"), "demo-in-progress")
	asking := ticket("Theme the Inbox", "Ready")
	s.agent("/tickets/"+n(asking)+"/post_round", map[string]any{"claim_id": claim(asking, "demo-asking"), "questions": []map[string]string{
		{"title": "Follow the system setting?", "body": "Should the theme follow the operating system's, or only the switch?", "recommended": "Follow the system, with the switch as an override."},
	}})
	review := ticket("Theme the Ticket page", "Ready")
	s.agent("/tickets/"+n(review)+"/finish", map[string]any{"claim_id": claim(review, "demo-review"), "summary": "Themed.", "branch": "demo-theme-ticket", "commit": "abc1234"})
	done := ticket("Pick the colours", "Ready")
	s.agent("/tickets/"+n(done)+"/finish", map[string]any{"claim_id": claim(done, "demo-done"), "summary": "Picked."})
	s.human("/tickets/"+n(done)+"/accept", map[string]any{})
	failed := ticket("Theme the charts", "Ready")
	s.agent("/tickets/"+n(failed)+"/report_failure", map[string]any{
		"claim_id": claim(failed, "demo-failed"), "what_broke": "The chart library has no dark palette.", "why": "It hard-codes its colours.",
		"what_to_fix": "Wrap it, or replace it.", "where_work_stands": "Nothing is written.",
	})

	// A Long-lived Spec: its Tickets land one by one.
	long := str(s.agent("/create_spec", map[string]any{"title": "Dependency updates", "body": "Standing work that never finishes.", "full": true}), "id")
	s.agent("/add_labels", map[string]any{"target": long, "labels": []string{"long-lived"}})
	s.agent("/create_ticket", map[string]any{"spec": long, "title": "Bump the Go toolchain", "state": "Ready"})

	return s.err == nil, s.err
}

// stageTicket finds the Ticket of the named Stage in a submit_requirement
// result, by its stage:<name> Label.
func stageTicket(v map[string]any, stage string) string {
	tickets, _ := v["stages"].([]any)
	for _, t := range tickets {
		m, _ := t.(map[string]any)
		labels, _ := m["labels"].([]any)
		for _, l := range labels {
			if l == "stage:"+stage {
				return str(m, "id")
			}
		}
	}
	return ""
}
