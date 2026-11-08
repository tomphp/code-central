package factory

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// loop polls until ctx ends: the first poll is at once, the rest every
// --poll. When ctx ends, running turns are stopped and waited for.
func (f *factory) loop(ctx context.Context) {
	defer f.turnsDone.Wait()
	defer f.stopAll()
	for {
		f.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-f.e.After(f.o.poll):
		}
	}
}

// poll reconciles the held Claims, then fills free slots: Answered
// resumes first, then new claims.
func (f *factory) poll(ctx context.Context) {
	answered := f.reconcile(ctx)
	f.sweep(ctx)
	for _, c := range answered {
		if ctx.Err() != nil || f.running() >= f.o.parallel {
			return
		}
		f.resumeAnswered(ctx, c)
	}
	for ctx.Err() == nil && f.running() < f.o.parallel {
		if !f.claimNext(ctx) {
			return
		}
	}
}

// turn is one running `claude -p` process, for one Claim.
type turn struct {
	claim claim
	proc  *os.Process
	done  chan struct{} // closed once the turn has ended and been read
}

// running is the number of turns running: the --parallel slots in use.
// A Claim parked in Needs Answer has no turn, so takes no slot.
func (f *factory) running() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.turns)
}

func (f *factory) turnFor(claimID string) *turn {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.turns[claimID]
}

// reconcile reads every held Claim's Ticket. A Claim that has ended
// (released, moved, Cancelled, or finished) has its turn stopped and is
// dropped, and its Claim ID is never written with again. It returns the
// Answered Claims with no turn running, oldest first.
func (f *factory) reconcile(ctx context.Context) []claim {
	var answered []claim
	for _, c := range f.heldClaims() {
		tk, err := f.client.GetTicket(ctx, f.project, core.Ref(c.Ticket))
		if err != nil {
			if ctx.Err() == nil {
				f.printf("%s: get_ticket: %v", c.Ticket, err)
			}
			continue
		}
		if ended, by := claimEnded(tk, c); ended {
			f.endClaim(c, by)
			continue
		}
		if tk.State == store.Answered && f.turnFor(c.ClaimID) == nil {
			answered = append(answered, c)
		}
	}
	return answered
}

// claimEnded reports whether c is no longer the Ticket's live Claim, and
// the action that ended it. Claim IDs aren't on reads, so the Claim is
// found by its session ID, which the Factory makes for each Claim.
func claimEnded(tk core.TicketView, c claim) (bool, string) {
	for _, v := range tk.Claims {
		if v.SessionID == c.SessionID {
			return v.Ended != nil, v.EndedBy
		}
	}
	return true, "unknown"
}

// endsItsOwnTurn are the actions by which a session ends its own Claim:
// its turn is about to exit, so it isn't stopped.
var endsItsOwnTurn = map[string]bool{"finish": true, "give_up": true, "report_failure": true}

// endClaim stops c's turn, unless the session ended the Claim itself,
// and drops c for good.
func (f *factory) endClaim(c claim, by string) {
	f.mu.Lock()
	f.ended[c.ClaimID] = true
	f.mu.Unlock()
	if t := f.turnFor(c.ClaimID); t != nil && !endsItsOwnTurn[by] {
		f.printf("%s: Claim %s ended (%s): stopping its turn", c.Ticket, c.ClaimID, by)
		f.stopTurn(t)
	}
	if err := f.dropClaim(c.ClaimID); err != nil {
		f.printf("%s: can't write the state file: %v", c.Ticket, err)
	}
}

// live reports whether the Factory may still write with claimID.
func (f *factory) live(claimID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.ended[claimID]
}

// killGrace is how long a turn being stopped has after SIGINT, and then
// after SIGTERM.
func (f *factory) killGrace() time.Duration {
	if f.e.KillGrace > 0 {
		return f.e.KillGrace
	}
	return 10 * time.Second
}

// stopTurn sends t SIGINT, then SIGTERM if it lingers, and waits for it
// to end. A turn that outlives SIGTERM too is killed.
func (f *factory) stopTurn(t *turn) {
	for _, sig := range []os.Signal{syscall.SIGINT, syscall.SIGTERM, os.Kill} {
		_ = t.proc.Signal(sig)
		select {
		case <-t.done:
			return
		case <-time.After(f.killGrace()):
		}
	}
	<-t.done
}

// stopAll stops every running turn at once.
func (f *factory) stopAll() {
	f.mu.Lock()
	var ts []*turn
	for _, t := range f.turns {
		ts = append(ts, t)
	}
	f.mu.Unlock()
	var wg sync.WaitGroup
	for _, t := range ts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.stopTurn(t)
		}()
	}
	wg.Wait()
}

// claimNext claims the Project's next claimable Ticket and starts its
// turn. It reports whether it claimed one.
func (f *factory) claimNext(ctx context.Context) bool {
	session := newSessionID()
	resume := "claude --resume " + session
	// The worktree's path is fixed by the Ticket ID, which isn't known
	// until the claim, so the claim names it afterwards (set_session).
	res, err := f.client.Claim(ctx, f.project, core.ClaimInput{SessionID: session, ResumeCommand: &resume})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("claim: %v", err)
		}
		return false
	}
	if res.Ticket == nil {
		return false
	}
	id := res.Ticket.ID
	stage := stageOf(*res.Ticket)
	guide := f.stageGuide(ctx, res.StageGuide, stage)
	c := claim{Ticket: id, ClaimID: res.ClaimID, SessionID: session, AllowedTools: []string{}}
	if err := f.addClaim(c); err != nil {
		f.printf("%s: can't write the state file: %v", id, err)
	}
	wt := f.worktree(id)
	if _, err := f.client.SetSession(ctx, f.project, core.SetSessionInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, SessionID: session, WorkingDir: &wt, ResumeCommand: &resume}); err != nil {
		f.printf("%s: set_session: %v", id, err)
	}
	setUp := f.implementationWorktree
	if stage != "" {
		setUp = f.stageWorktree
	}
	if err := setUp(id, wt); err != nil {
		if fl := (*failure)(nil); errors.As(err, &fl) {
			f.reportFailure(ctx, c, fl)
			return true
		}
		f.printf("%s: can't set up the worktree: %v", id, err)
		return true
	}
	f.startTurn(c, wt, append(f.startArgs(c), firstPrompt(c, stage, guide)))
	return true
}

// resumeAnswered picks up the answers to c's Round and resumes its
// session with them, unless another process holds the session, when it
// waits a poll.
func (f *factory) resumeAnswered(ctx context.Context, c claim) {
	if !f.live(c.ClaimID) {
		return
	}
	if pid, ok := f.sessionHolder(c.SessionID); ok {
		f.printf("%s: session %s is held by pid %d: waiting a poll before resuming", c.Ticket, c.SessionID, pid)
		return
	}
	res, err := f.client.PickUpAnswers(ctx, f.project, core.PickUpAnswersInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("%s: pick_up_answers: %v", c.Ticket, err)
		}
		return
	}
	f.printf("%s: resuming with the answers to Round %d", c.Ticket, res.Round.Number)
	f.startTurn(c, f.worktree(c.Ticket), append(f.resumeArgs(c), answeredPrompt(c, res)))
}

// sessionHolder looks through the process list for a process other than
// this one whose command line names session, as a claude still running
// that session would.
func (f *factory) sessionHolder(session string) (int, bool) {
	ps, ok := f.e.lookPath("ps")
	if !ok {
		ps = "/bin/ps"
	}
	cmd := exec.Command(ps, "-A", "-o", "pid=,args=")
	cmd.Env = f.e.Environ
	out, err := cmd.Output()
	if err != nil {
		f.printf("can't read the process list: %v", err)
		return 0, false
	}
	for _, line := range strings.Split(string(out), "\n") {
		pidText, args, _ := strings.Cut(strings.TrimSpace(line), " ")
		pid, err := strconv.Atoi(pidText)
		if err != nil || pid == os.Getpid() {
			continue
		}
		if strings.Contains(args, session) {
			return pid, true
		}
	}
	return 0, false
}

// answeredPrompt resumes a session whose Round the human has answered.
// The Factory has already picked the answers up, so they are given here.
func answeredPrompt(c claim, res core.PickUpAnswersResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, `The human has answered your Round %d on %s. code-central-factory has already picked up the answers for you (don't call pick_up_answers again); they are below. Carry on from the answers, under the same Claim.

Ticket: %s
Claim: %s
`, res.Round.Number, c.Ticket, c.Ticket, c.ClaimID)
	for _, q := range res.Round.Questions {
		fmt.Fprintf(&b, "\n## Question %d: %s\n\n", q.Number, q.Title)
		switch q.Status {
		case store.QuestionAnswered:
			fmt.Fprintf(&b, "Answer: %s\n", q.Answer)
		default:
			fmt.Fprintf(&b, "(%s, with no answer)\n", q.Status)
		}
	}
	for _, cm := range res.Comments {
		fmt.Fprintf(&b, "\n## Comment by %s\n\n%s\n", cm.Actor, cm.Body)
	}
	return b.String()
}

// result is the stream-json result message that ends a turn.
type result struct {
	Subtype           string            `json:"subtype"`
	IsError           bool              `json:"is_error"`
	TerminalReason    string            `json:"terminal_reason"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
	Result            string            `json:"result"`
}

// turnEnd is how a turn ended: its exit code and its result message, if
// it printed one.
type turnEnd struct {
	ExitCode int
	Result   *result
}

func (t turnEnd) String() string {
	s := fmt.Sprintf("exit %d", t.ExitCode)
	if t.Result == nil {
		return s + ", no result"
	}
	s += ", " + t.Result.Subtype
	if t.Result.TerminalReason != "" {
		s += " (" + t.Result.TerminalReason + ")"
	}
	return s
}

// startArgs are a new session's flags.
func (f *factory) startArgs(c claim) []string {
	return append([]string{"-p", "--session-id", c.SessionID}, f.turnFlags(c)...)
}

// resumeArgs are the flags that resume c's session.
func (f *factory) resumeArgs(c claim) []string {
	return append([]string{"-p", "--resume", c.SessionID}, f.turnFlags(c)...)
}

// turnFlags are the flags every start and resume passes: the output
// format, the permission mode, the model when set, and the Claim's
// allowed tools.
func (f *factory) turnFlags(c claim) []string {
	args := []string{"--output-format", "stream-json", "--verbose"}
	if f.o.bypass {
		args = append(args, "--permission-mode", "bypassPermissions")
	} else {
		args = append(args, "--permission-mode", "auto", "--permission-prompts", "none")
	}
	if f.o.model != "" {
		args = append(args, "--model", f.o.model)
	}
	if len(c.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(c.AllowedTools, ","))
	}
	return args
}

// startTurn runs `claude args...` in dir for c, taking a slot until it
// ends.
func (f *factory) startTurn(c claim, dir string, args []string) {
	cmd := exec.Command(f.claude, args...)
	cmd.Dir = dir
	cmd.Env = f.e.Environ
	cmd.Stderr = f.e.Stderr
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		f.printf("%s: can't start claude: %v", c.Ticket, err)
		return
	}
	t := &turn{claim: c, proc: cmd.Process, done: make(chan struct{})}
	f.mu.Lock()
	f.turns[c.ClaimID] = t
	f.mu.Unlock()
	f.turnsDone.Add(1)
	go func() {
		defer f.turnsDone.Done()
		end := readTurn(cmd, stdout)
		f.printf("%s: turn ended: %s", c.Ticket, end)
		f.mu.Lock()
		delete(f.turns, c.ClaimID)
		f.mu.Unlock()
		close(t.done)
	}()
}

// readTurn reads a started turn's stream-json to its end and waits for
// it, giving how it ended.
func readTurn(cmd *exec.Cmd, stdout io.Reader) turnEnd {
	var end turnEnd
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		var msg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(sc.Bytes(), &msg) != nil || msg.Type != "result" {
			continue
		}
		var r result
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			end.Result = &r
		}
	}
	err := cmd.Wait()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		end.ExitCode = exit.ExitCode()
	default:
		end.ExitCode = -1
	}
	return end
}

// newSessionID is a random UUID (version 4), as claude --session-id takes.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
