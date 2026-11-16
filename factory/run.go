// The Factory's poll loop: signals and draining, the turns running in
// its slots, ending Claims that ended elsewhere, and claiming or resuming
// the next Ticket. turn.go runs each turn's claude process.

package factory

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// loop polls until ctx ends: the first poll is at once, the rest on a
// wake or else every --poll, timed afresh after each poll. When ctx ends,
// running turns are stopped and waited for.
func (f *factory) loop(ctx context.Context) {
	defer f.stopPolling()
	// woke ends the wake that started the poll, once the poll has.
	woke := noop
	for {
		f.polls++
		f.polling.busy(f.pollName())
		woke()
		woke = noop
		f.poll(ctx)
		next, waited := f.work.pollWait(f.e.Clock, f.o.poll)
		f.polling.then(waited)
		select {
		case <-ctx.Done():
			return
		case <-f.drainCh:
			f.drain(ctx)
			return
		case <-next:
			f.releaseHeldBack()
		case woke = <-f.wakeCh:
			// A stopping or draining Factory ignores wakes.
			if ctx.Err() != nil {
				woke()
				return
			}
			if f.draining() {
				f.drainWork.give(woke)
				f.drain(ctx)
				return
			}
		}
	}
}

// pollName names the poll in progress.
func (f *factory) pollName() string { return "poll " + strconv.Itoa(f.polls) }

// watchSignals turns the stop signals into a drain or a stop. The first
// Ctrl-C drains; a second Ctrl-C, SIGTERM or SIGHUP stops the running
// turns and exits. Either way every Claim is kept.
func (f *factory) watchSignals(ctx context.Context, s *stint) {
	defer s.stop()
	for {
		s.rest(SignalWait)
		select {
		case <-ctx.Done():
			return
		case sig := <-f.e.Signals:
			s.busy("taking the signal ", sig.String())
			if sig == os.Interrupt && !f.draining() {
				f.printf("draining: no more claiming, resuming or landing; Ctrl-C again to stop the running turns (Claims are kept)")
				f.drainOnce.Do(func() {
					f.drainWork.give(f.work.busy("starting to drain"))
					close(f.drainCh)
				})
				f.heartbeat(core.HeartbeatDraining, f.drainNote())
				continue
			}
			f.stopMu.Lock()
			f.stopSignal = sig
			f.stopMu.Unlock()
			f.printf("stopping on %v: stopping the running turns (Claims are kept)", sig)
			f.halt()
			return
		}
	}
}

// draining reports whether the first Ctrl-C has come.
func (f *factory) draining() bool {
	select {
	case <-f.drainCh:
		return true
	default:
		return false
	}
}

// untilDraining is ctx, ended also by the first Ctrl-C: a long-poll's
// context, as draining closes the long-polls.
func (f *factory) untilDraining(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-f.drainCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// halted reports whether the Factory must start nothing more: it is
// stopping or draining.
func (f *factory) halted(ctx context.Context) bool {
	return ctx.Err() != nil || f.draining()
}

// drain waits for the running turns to end, naming them, unless a stop
// comes first. The loop has stopped, so no turn starts while it waits,
// except one for a claim whose answer was lost.
func (f *factory) drain(ctx context.Context) {
	f.polling.busy("draining")
	f.drainWork.take()
	f.endPolls()
	if !f.drainLostClaims(ctx) {
		return
	}
	f.mu.Lock()
	var names []string
	for _, t := range f.turns {
		names = append(names, t.claim.Ticket)
	}
	f.mu.Unlock()
	if len(names) == 0 {
		f.printf("drained: no turns running")
		f.setDrained()
		return
	}
	sort.Strings(names)
	f.printf("draining: waiting on %d running turn(s): %s", len(names), strings.Join(names, ", "))
	done := make(chan struct{})
	go func() {
		f.turnsDone.Wait()
		close(done)
	}()
	for {
		beat := f.polling.after(f.e.Clock, f.o.poll, "draining: waiting on ", strings.Join(names, ", "))
		select {
		case <-done:
			f.polling.busy("draining")
			f.printf("drained: every turn has ended")
			f.setDrained()
			return
		case <-ctx.Done():
			return
		case <-beat:
			f.polling.busy("draining")
			f.heartbeat(core.HeartbeatDraining, f.drainNote())
		}
	}
}

// drainLostClaims sends the claims whose answer was lost again at each
// poll until Code Central answers them: one that went through has put
// its Ticket In Progress already, so it gets its turn, which the drain
// then waits on. It reports false if a stop came first, which abandons
// them.
func (f *factory) drainLostClaims(ctx context.Context) bool {
	f.claimLostAgain(ctx)
	if len(f.lostClaims) == 0 || f.claimingHeld() {
		return ctx.Err() == nil
	}
	var names []string
	for _, in := range f.lostClaims {
		names = append(names, string(in.Ticket))
	}
	// The draining Heartbeat's usual note counts running turns, which
	// may be none yet.
	note := fmt.Sprintf("draining: claiming %s again", strings.Join(names, ", "))
	f.printf("%s at each poll first: the answer was lost", note)
	for len(f.lostClaims) > 0 && !f.claimingHeld() {
		poll := f.polling.after(f.e.Clock, f.o.poll, note)
		select {
		case <-ctx.Done():
			return false
		case <-poll:
			f.polling.busy(note)
			f.heartbeat(core.HeartbeatDraining, note)
			f.claimLostAgain(ctx)
		}
	}
	return ctx.Err() == nil
}

func (f *factory) setDrained() {
	f.stopMu.Lock()
	f.drained = true
	f.stopMu.Unlock()
}

// poll reconciles the held Claims, starts the next decision of each idle
// guided Map, then fills free slots: Answered resumes first, then saved
// Claims a stop cut off, then new claims.
func (f *factory) poll(ctx context.Context) {
	defer func() {
		if !f.halted(ctx) {
			f.heartbeat(core.HeartbeatOnline, f.pollNote())
		}
	}()
	answered := f.reconcile(ctx)
	f.landAndSweep(ctx)
	if f.halted(ctx) {
		return
	}
	if f.paused() && !f.endIdlePause() {
		for _, c := range f.duePausedResumes(f.o.parallel - f.slotsTaken()) {
			if !f.resumeCutOff(ctx, c) && f.live(c.ClaimID) {
				f.requeueCutOff(c)
			}
		}
		return
	}
	f.startGuidedMaps(ctx)
	for _, c := range answered {
		if !f.slotFree(ctx) {
			return
		}
		f.resumeAnswered(ctx, c)
	}
	f.resumeRestarted(ctx)
	// A turn this poll started may have paused the Factory, which then
	// starts no turn: the lost claims wait for the pause to end, holding
	// their slots.
	if !f.halted(ctx) && !f.paused() {
		f.claimLostAgain(ctx)
	}
	for f.slotFree(ctx) {
		if !f.claimNext(ctx) {
			return
		}
	}
}

// slotFree reports whether the poll may start another turn: a slot is
// free and the Factory is neither halted nor paused. A turn this poll
// started may have been cut off since, freeing its slot, so the pause is
// read after the slot: a turn records its outage before giving its slot
// up, so a slot freed by a cut-off turn is always seen with the pause.
func (f *factory) slotFree(ctx context.Context) bool {
	return !f.halted(ctx) && f.slotsTaken() < f.o.parallel && !f.paused()
}

// slotsTaken is the --parallel slots in use: the running turns, and the
// claims whose answer was lost, each of which may yet start one.
func (f *factory) slotsTaken() int {
	return f.running() + len(f.lostClaims)
}

// turn is one running Turn of claude, for one Claim.
type turn struct {
	claim   claim
	dir     string
	choice  turnChoice // the model and effort it was started with
	started time.Time
	// cancel ends the Turn's stop context, which has the Turn runner stop
	// claude; stopWork hands it the stop's busy work.
	cancel   context.CancelFunc
	stopWork handoff
	done     chan struct{} // closed once the turn has ended and been read
	// stopped is set when the Factory stops the turn, so its end is the
	// Factory's doing, not an outcome to act on.
	stopped atomic.Bool
	// ending is set, under the factory's mu, as the turn's goroutine
	// closes done; until then a poll waiting on it joins waiters, which
	// the goroutine waits for before its own work ends.
	ending  bool
	waiters sync.WaitGroup
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
				f.claimf(c, "%s: get_ticket: %v", c.Ticket, err)
			}
			continue
		}
		if ended, by := claimEnded(tk, c); ended {
			f.endClaim(c, by)
			continue
		}
		f.noteTicket(c, tk)
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
// and drops c for good, with what the lines say of it.
func (f *factory) endClaim(c claim, by string) {
	f.mu.Lock()
	f.ended[c.ClaimID] = true
	f.mu.Unlock()
	f.dropCutOff(c.ClaimID)
	t := f.turnFor(c.ClaimID)
	if t != nil && !endsItsOwnTurn[by] {
		f.claimf(c, "%s: Claim %s ended (%s): stopping its turn", c.Ticket, c.ClaimID, by)
		f.stopTurn(t)
	}
	if err := f.dropClaim(c.ClaimID); err != nil {
		f.claimf(c, "%s: can't write the state file: %v", c.Ticket, err)
	}
	if t == nil {
		f.forgetEnded(c)
	}
}

// forgetEnded drops what the lines say of c once it has ended. A
// running turn's own end forgets it, after its turn ended line.
func (f *factory) forgetEnded(c claim) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ended[c.ClaimID] {
		delete(f.info, c.ClaimID)
	}
}

// live reports whether the Factory may still write with claimID.
func (f *factory) live(claimID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.ended[claimID]
}

// stopTurn stops t, which the Turn runner does through its stop ladder,
// and waits for it to end. The poll, which calls it, rests meanwhile, and
// is busy again before the turn's own work ends.
func (f *factory) stopTurn(t *turn) {
	f.beginStop(t)
	f.mu.Lock()
	waiting := !t.ending
	if waiting {
		t.waiters.Add(1)
	}
	f.mu.Unlock()
	if !waiting {
		<-t.done
		return
	}
	f.polling.rest(f.pollName(), ": waiting for turn ", t.claim.Ticket, " to stop")
	<-t.done
	f.polling.busy(f.pollName())
	t.waiters.Done()
}

// beginStop has the Turn runner stop t, handing it the stop's busy work,
// which it ends once the first stop step is under way, or t's goroutine
// ends as the turn ends.
func (f *factory) beginStop(t *turn) {
	t.stopped.Store(true)
	f.mu.Lock()
	if !t.ending {
		t.stopWork.give(f.work.busy("stopping turn ", t.claim.Ticket))
	}
	f.mu.Unlock()
	t.cancel()
}

// endTurn closes t.done once t's turn has ended, and waits until each
// poll waiting on it is busy again, so the hand-over leaves no moment
// that looks settled.
func (f *factory) endTurn(t *turn) {
	f.mu.Lock()
	t.ending = true
	f.mu.Unlock()
	t.stopWork.take()
	close(t.done)
	t.waiters.Wait()
}

// stopPolling ends the poll loop: it stops every running turn at once,
// takes no more wakes, and waits for the turns to end, resting meanwhile.
func (f *factory) stopPolling() {
	f.mu.Lock()
	var ts []*turn
	for _, t := range f.turns {
		ts = append(ts, t)
	}
	f.mu.Unlock()
	for _, t := range ts {
		f.beginStop(t)
	}
	f.stopWork.take()
	f.endPolls()
	f.polling.stop()
	for _, t := range ts {
		<-t.done
	}
	f.turnsDone.Wait()
}

// nextRunnable is the Project's first claimable Ticket, in claim order,
// whose guide the Factory runs, whose claim isn't lost and that isn't
// held back: a claim lost before it reached Code Central leaves its
// Ticket claimable. ok is false when there is none.
func (f *factory) nextRunnable(ctx context.Context) (id core.Ref, ok bool) {
	claimable := true
	rows, err := f.client.FindTickets(ctx, f.project, core.FindTicketsInput{Claimable: &claimable})
	if err != nil {
		if ctx.Err() == nil {
			f.printf("find_tickets: %v", err)
		}
		return "", false
	}
	for _, r := range rows {
		lost := slices.ContainsFunc(f.lostClaims, func(in core.ClaimInput) bool { return in.Ticket == core.Ref(r.ID) })
		if runs(core.GuideName(r.Labels)) && !lost && !f.heldBack(r.ID) {
			return core.Ref(r.ID), true
		}
	}
	return "", false
}

// claimNext claims the Project's next claimable Ticket that it runs and
// starts its turn. It reports whether it claimed one.
func (f *factory) claimNext(ctx context.Context) bool {
	if f.claimingHeld() {
		return false
	}
	next, ok := f.nextRunnable(ctx)
	if !ok {
		return false
	}
	session := newSessionID()
	resume := "claude --resume " + session
	// The worktree's path is fixed by the Ticket ID, which isn't known
	// until the claim, so the claim names it afterwards (set_session).
	in := core.ClaimInput{Ticket: next, SessionID: session, ResumeCommand: &resume}
	res, err := f.claimRecovering(ctx, in)
	switch {
	case ctx.Err() != nil:
		// A stop abandons a claim whose answer was lost.
		return false
	case answerLost(err):
		f.lostClaims = append(f.lostClaims, in)
		return false
	case err != nil:
		f.printf("claim: %v", err)
		return false
	}
	return f.startClaimed(ctx, in, res)
}

// claimingHeld reports whether nothing may be claimed, as every claim
// cuts a branch or a worktree from the base: the base disagrees with the
// host's, is missing, or lacks .mcp.json.
func (f *factory) claimingHeld() bool {
	return f.baseDiverged || f.baseMissing || f.mcpHeld
}

// claimLostAgain sends each claim whose answer was lost again, once, so
// no poll is held up while Code Central is down. One Code Central
// answers with a Claim starts its turn; one it refuses is dropped; one
// still lost waits for the next poll, holding its slot.
func (f *factory) claimLostAgain(ctx context.Context) {
	if f.claimingHeld() {
		return
	}
	lost := f.lostClaims
	f.lostClaims = nil
	for _, in := range lost {
		res, err := f.client.Claim(ctx, f.project, in)
		switch {
		case ctx.Err() != nil:
			// A stop abandons them.
			return
		case answerLost(err):
			f.lostClaims = append(f.lostClaims, in)
		case err != nil:
			f.printf("%s: claim: %v", in.Ticket, err)
		default:
			f.printf("%s: claim: the answer came back on claiming it again", in.Ticket)
			f.startClaimed(ctx, in, res)
		}
	}
}

// startClaimed carries on from res, the answer to claiming with in: it
// records the Claim, names its worktree and starts its turn. It reports
// whether there was a Claim.
func (f *factory) startClaimed(ctx context.Context, in core.ClaimInput, res core.ClaimResult) bool {
	session, resume := in.SessionID, *in.ResumeCommand
	if res.Ticket == nil {
		return false
	}
	id := res.Ticket.ID
	stage := core.GuideName(res.Ticket.Labels)
	conflicts := f.conflictFiles(stage, *res.Ticket)
	var guide core.StageGuideView
	if len(conflicts) == 0 {
		guide = f.stageGuide(ctx, res.StageGuide, stage)
	}
	wt := f.worktree(id)
	c := claim{Ticket: id, ClaimID: res.ClaimID, SessionID: session, AllowedTools: []string{}}
	c.SystemPrompt = systemPrompt(c, stage, wt, guide, conflicts)
	f.noteTicket(c, *res.Ticket)
	if err := f.addClaim(c); err != nil {
		f.claimf(c, "%s: can't write the state file: %v", id, err)
	}
	if _, err := f.client.SetSession(ctx, f.project, core.SetSessionInput{Ticket: core.Ref(id), ClaimID: res.ClaimID, SessionID: session, WorkingDir: &wt, ResumeCommand: &resume}); err != nil {
		f.claimf(c, "%s: set_session: %v", id, err)
	}
	f.claimed(c, *res.Ticket, wt)
	settingUp := f.work.busy("setting up the worktree of ", id)
	var err error
	if branch := workBranch(stage, id); branch != "" {
		err = f.cutBranchWorktree(*res.Ticket, branch, wt)
	} else {
		err = f.stageWorktree(wt)
	}
	settingUp()
	if err != nil {
		fl := (*failure)(nil)
		if !errors.As(err, &fl) {
			fl = f.setUpFailed(wt, err)
		}
		f.reportFailure(ctx, c, fl)
		return true
	}
	mc := f.withFlags(stage, res.ModelChoice)
	f.startTurn(ctx, c, turnInput{Dir: wt, Choice: mc, Prompt: firstPrompt(c, stage, f.baseBranch(), conflicts)})
	return true
}

// claimRetries is how many times a claim whose answer was lost is sent
// again, claimRetryPause apart.
const (
	claimRetries    = 3
	claimRetryPause = time.Second
)

// claimRecovering claims in.Ticket. When the answer is lost, Code Central
// may have put the Ticket in the Factory's hands, so it sends in again,
// which gives back the same Claim if the first got through (ADR 0010).
// The pause between tries is on the Factory's clock, as every other
// timer is, and the poll rests on it. It stops trying when ctx ends or
// the Factory drains.
func (f *factory) claimRecovering(ctx context.Context, in core.ClaimInput) (core.ClaimResult, error) {
	res, err := f.client.Claim(ctx, f.project, in)
	if !answerLost(err) || ctx.Err() != nil {
		return res, err
	}
	f.printf("%s: claim: the answer was lost (%v): claiming it again", in.Ticket, err)
	for range claimRetries {
		pause := f.polling.after(f.e.Clock, claimRetryPause, f.pollName(), ": claim retry pause of ", string(in.Ticket))
		select {
		case <-ctx.Done():
		case <-f.drainCh:
		case <-pause:
		}
		f.polling.busy(f.pollName())
		if ctx.Err() != nil || f.draining() {
			return res, err
		}
		res, err = f.client.Claim(ctx, f.project, in)
		if !answerLost(err) {
			if err == nil {
				f.printf("%s: claim: the answer came back on claiming it again", in.Ticket)
			}
			return res, err
		}
	}
	return res, err
}

// answerLost reports whether err leaves unknown what Code Central did: it
// couldn't be reached, or its answer was cut off. Any other *core.Error
// is Code Central's answer.
func answerLost(err error) bool {
	if err == nil {
		return false
	}
	var ce *core.Error
	return !errors.As(err, &ce) || ce.Code == core.ServiceUnavailable
}

// resumeAnswered picks up the answers to c's Round and resumes its
// session with them, unless another process holds the session, when it
// waits a poll.
func (f *factory) resumeAnswered(ctx context.Context, c claim) {
	if !f.live(c.ClaimID) {
		return
	}
	if pid, ok := f.sessionHolder(c.SessionID); ok {
		f.claimf(c, "%s: session %s is held by pid %d: waiting a poll before resuming", c.Ticket, c.SessionID, pid)
		return
	}
	// Before the answers are picked up, so a failure leaves it Answered.
	mc, ok := f.resumeChoice(ctx, c)
	if !ok {
		return
	}
	res, err := f.client.PickUpAnswers(ctx, f.project, core.PickUpAnswersInput{Ticket: core.Ref(c.Ticket), ClaimID: c.ClaimID})
	if err != nil {
		if ctx.Err() == nil {
			f.claimf(c, "%s: pick_up_answers: %v", c.Ticket, err)
		}
		return
	}
	f.claimf(c, "%s resumed %s with the answers to Round %d", c.Ticket, f.about(c), res.Round.Number)
	c, decided := f.takeDenialAnswers(c, res)
	prompt := answeredPrompt(c, res)
	if decided != "" {
		prompt += "\n" + decided + "\n"
	}
	f.startTurn(ctx, c, turnInput{Dir: f.worktree(c.Ticket), Resume: true, Choice: mc, Prompt: prompt})
}

// sessionHolder looks through the process list for a process other than
// this one whose command line names session, as a claude still running
// that session would.
func (f *factory) sessionHolder(session string) (int, bool) {
	out, err := f.e.listProcesses()
	if err != nil {
		f.printf("can't read the process list: %v", err)
		return 0, false
	}
	for _, line := range strings.Split(out, "\n") {
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
