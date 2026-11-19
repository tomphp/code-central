// Chat (Spec code-central-633): the Factory answers the Project's
// Conversation from the UI. A goroutine apart from the poll loop keeps one
// next_chat_work call open while the Factory runs with Chat on, and runs
// each turn it hands out as a claude session in the Chat worktree, one at
// a time and in no Claim slot, posting the reply to Code Central as it
// arrives.

package factory

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/api"
)

const (
	// kindChat is the kind a Chat turn's model responses are posted
	// under in Usage.
	kindChat = "chat"
	// chatRetry is how long the Chat loop waits after a next_chat_work
	// that failed before it calls again.
	chatRetry = 5 * time.Second
	// chatTurnKey names a Chat turn in the Factory's work (Work), as its
	// Ticket ID names a Claim's turn.
	chatTurnKey = "chat"
)

// chatStop stops a running Chat turn: cancel ends its context, and work
// hands its Turn runner the stop's busy work.
type chatStop struct {
	cancel context.CancelFunc
	work   handoff
}

// chatLoop keeps one next_chat_work call open until ctx ends or the
// Factory drains, and starts each turn it hands out, one at a time. It
// returns once the last turn has ended. With --no-chat it never calls. s
// is its line of work, busy starting it.
func (f *factory) chatLoop(ctx context.Context, s *stint) {
	defer s.stop()
	if f.o.noChat {
		return
	}
	var turns sync.WaitGroup
	defer func() {
		s.rest("waiting for the Chat turn to end")
		turns.Wait()
	}()
	// Draining closes the long-poll; a running turn goes on until ctx ends.
	pollCtx, cancel := f.untilDraining(ctx)
	defer cancel()
	var turnRunning sync.Mutex // held by the running turn
	// stops ends a Conversation's turn, by its ID, when a stop asks.
	var stopsMu sync.Mutex
	stops := map[string]chatStop{}
	failing := false
	for pollCtx.Err() == nil {
		s.rest(ChatLongPoll)
		w, err := f.client.NextChatWork(pollCtx, f.project, api.NextChatWorkInput{Factory: factoryName, Host: f.host})
		if err != nil {
			if pollCtx.Err() != nil {
				return
			}
			if !failing {
				f.printf("warning: can't take Chat work from Code Central: %v", err)
			}
			failing = true
			retry := s.after(f.e.Clock, chatRetry, "Chat retry pause")
			select {
			case <-pollCtx.Done():
			case <-retry:
			}
			continue
		}
		failing = false
		if w.Stop != nil {
			stopsMu.Lock()
			if stop, ok := stops[w.Stop.Conversation]; ok {
				stop.work.give(f.work.busy("stopping turn ", chatTurnKey))
				stop.cancel()
			}
			stopsMu.Unlock()
			continue
		}
		if w.Turn == nil {
			continue
		}
		turnCtx, cancelTurn := context.WithCancel(ctx)
		stop := chatStop{cancel: cancelTurn, work: newHandoff()}
		stopsMu.Lock()
		stops[w.Turn.Conversation] = stop
		stopsMu.Unlock()
		turns.Add(1)
		work := f.starting("turn ", chatTurnKey)
		go func(t api.ChatTurn) {
			defer turns.Done()
			defer work.stop()
			defer func() {
				stopsMu.Lock()
				delete(stops, t.Conversation)
				stopsMu.Unlock()
				// A stop that came as the turn ended was never taken up.
				stop.work.take()
				cancelTurn()
			}()
			// Code Central hands out a turn only once the reply before
			// it has ended, which its turn posts just before it returns.
			turnRunning.Lock()
			defer turnRunning.Unlock()
			f.runChatTurn(turnCtx, t, work, stop.work)
		}(*w.Turn)
	}
}

// runChatTurn answers one Chat Message and posts the reply as it comes:
// its items, then its outcome. work is the turn's line of work, busy
// starting it; stopWork hands its Turn runner a stop's busy work.
func (f *factory) runChatTurn(ctx context.Context, t api.ChatTurn, work *stint, stopWork handoff) {
	ws := f.chatWorktrees(t.First)
	session := t.SessionID
	if t.First {
		session = newSessionID()
	}
	p := f.newChatPoster(t, session, chatResumeCommand(ws.home().path, session, ws.addDirs()))
	p.start()
	outcome, msg := f.answerChat(ctx, t, session, ws, p, work, stopWork)
	if outcome == api.ChatFailed {
		f.printf("chat: the reply failed: %s", firstLine(msg))
	}
	p.end(outcome, msg)
}

// chatWorktrees is the Chat worktree of every Repository with a clone, for
// a Conversation's first turn, or for a later one those already there: a
// Repository added mid-Conversation joins the next.
func (f *factory) chatWorktrees(first bool) worktreeSet {
	ws := f.repos.worktreesNamed(chatWorktreeName, "")
	if first {
		return ws
	}
	var there worktreeSet
	for _, w := range ws {
		if _, err := os.Stat(w.path); w.r.home || err == nil {
			there = append(there, w)
		}
	}
	return there
}

// answerChat runs t's claude turn in the Chat worktrees ws, as session,
// from the Home Repository's, and says how the reply ended, with the
// error text of a failed one. A first turn moves each Chat worktree to
// its base's tip first.
func (f *factory) answerChat(ctx context.Context, t api.ChatTurn, session string, ws worktreeSet, p *chatPoster, work *stint, stopWork handoff) (api.ChatOutcome, string) {
	if t.First {
		for _, w := range ws {
			if err := f.chatWorktreeAtBase(w.r, w.path); err != nil {
				return api.ChatFailed, ws.about(w, err.Error())
			}
		}
	}
	// A turn that waited on the one before it starts nothing once the
	// Factory is stopping.
	if ctx.Err() != nil {
		return api.ChatStopped, ""
	}
	wt := ws.home().path
	if t.First {
		f.printf("chat: starting a Conversation in %s; resume: %s", wt, p.resumeCommand)
	}
	out := f.runner.run(ctx, turnInput{
		Dir:          wt,
		AddDirs:      ws.addDirs(),
		Session:      session,
		Resume:       !t.First,
		Choice:       turnChoice{model: t.Model, effort: t.Effort},
		SystemPrompt: chatPrompt(f.project, wt) + chatRepositories(ws),
		Prompt:       t.Text,
		UsageKind:    kindChat,
		OnEvent:      p.add,
		OnUsage:      p.addUsage,
		OnPlanLimits: f.planLimits.set,
		Linef:        func(format string, args ...any) { f.printf("chat: "+format, args...) },
		Key:          chatTurnKey,
		Work:         work,
		StopWork:     stopWork,
	})
	if out.End.Result != nil && len(out.End.Result.PermissionDenials) > 0 {
		p.add(denialsItem(out.End.Result.PermissionDenials, f.e.Clock.Now()))
	}
	return chatOutcome(t, out)
}

// chatOutcome is how a Chat turn ended: done, stopped, or failed with the
// Factory's error text, naming the cause when it was a rejected model or
// effort, an Outage or an expired login. Chat never retries, pauses or
// stops the Factory for any of them.
func chatOutcome(t api.ChatTurn, out turnOutcome) (api.ChatOutcome, string) {
	const pick = "Pick another in New conversation, or set the Chat row's in Project Options → Models."
	switch out.Kind {
	case turnStopped:
		return api.ChatStopped, ""
	case turnDone:
		return api.ChatDone, ""
	case turnEffortRejected:
		return api.ChatFailed, fmt.Sprintf("The effort `%s` isn't one this Factory's claude accepts (%s), so no reply was started. %s", t.Effort, strings.Join(out.Levels, ", "), pick)
	case turnModelRejected:
		model := "`" + t.Model + "`"
		if t.Model == "" {
			model = "its default"
		}
		return api.ChatFailed, fmt.Sprintf("claude rejected the model %s (model_not_found): it doesn't exist, or this account can't use it. %s", model, pick)
	case turnOutage:
		return api.ChatFailed, fmt.Sprintf("claude hit an Outage (`%s`): a usage or rate limit, overload, or a server or network error cut off the reply. Send the message again later.", out.End.apiError())
	case turnLoginFailed:
		return api.ChatFailed, "claude's login has expired or is missing (authentication_failed): log in with `claude /login` on the Factory's host, then send the message again."
	}
	if out.StartErr != nil {
		return api.ChatFailed, fmt.Sprintf("The Factory couldn't start claude: %v", out.StartErr)
	}
	end := out.End
	msg := "claude ended the reply with " + end.how()
	if apiRepo := end.apiError(); apiRepo != "" {
		msg += ", API error " + apiRepo
	}
	if end.Result != nil && strings.TrimSpace(end.Result.Result) != "" {
		msg += ": " + end.Result.Result
	}
	return api.ChatFailed, msg
}

// denialsItem is the item listing the distinct calls a Chat turn had
// denied.
func denialsItem(ds []denial, now time.Time) api.SessionEvent {
	return api.SessionEvent{Time: now, Kind: api.EventPermissionDenials, Text: "Auto mode denied these calls:\n" + denialList(ds)}
}

// chatWorktreeAtBase makes r's Chat worktree wt detached at its base's
// tip for a Conversation's first turn. Anything it can't move is left as
// it was, with an error saying what is there and how to clear it.
func (f *factory) chatWorktreeAtBase(r *repository, wt string) error {
	left, err := f.repos.detachAtBase(r, wt)
	switch {
	case err != nil:
		return fmt.Errorf("The Factory couldn't set up the Chat worktree at %s: %v", wt, err)
	case left == nil:
		return nil
	case left.stray:
		return fmt.Errorf("Something that isn't a git worktree is at the Chat worktree's path, %s, so the Factory can't start the Conversation there. Keep anything worth keeping elsewhere, remove or move it, then send the message again.", wt)
	case left.branch != "":
		return fmt.Errorf("The Chat worktree at %s is on branch %s, not detached, so the Factory can't use it. Detach it with `git -C %s switch --detach`, or remove it with `git worktree remove %s`, then send the message again.", wt, left.branch, wt, wt)
	}
	return fmt.Errorf("The Chat worktree at %s has uncommitted changes, so the Factory can't move it to %s's tip for a new Conversation. Changed files:\n\n%s\n\nA Chat writes nothing to the repository. Keep anything worth keeping elsewhere, then clean the worktree with `git -C %s reset --hard` and `git -C %s clean -fd`, and send the message again.", wt, f.repos.baseOf(r), left.dirty, wt, wt)
}

// chatResumeCommand brings a Conversation's session back from anywhere,
// so it changes into the Chat worktree wt first, and adds the other
// Repositories' Chat worktrees, dirs, as its turns do.
func chatResumeCommand(wt, session string, dirs []string) string {
	return "cd " + wt + " && claude --resume " + session + addDirFlags(dirs)
}

// chatRepositories ends a Chat turn's system prompt in several
// Repositories, ws, saying where each one's Chat worktree is; it is
// empty with one.
func chatRepositories(ws worktreeSet) string {
	if !ws.several() {
		return ""
	}
	var each []string
	for _, w := range ws {
		each = append(each, w.label()+": "+w.path)
	}
	return " This Project has several Repositories, each read-only too, with a Chat worktree at its Base's tip: " + strings.Join(each, "; ") + "."
}

// chatPrompt is what every Chat turn adds to claude's system prompt, for
// the Project with key project, in the Chat worktree wt. The Project's
// MCP server comes from the worktree's .mcp.json, as it does for Stages.
func chatPrompt(project, wt string) string {
	return fmt.Sprintf("This is a Chat from the Code Central UI about Project %s, run by code-central-factory in its Chat worktree, %s. There is no Ticket and no Claim, so no Claim ID, claim, finish or Round applies. "+
		"The repository is read-only: write nothing, commit nothing and create no branch. "+
		"Read Code Central freely, and write to it (a Spec, a Small Change, a comment) only when the human asks in the Chat. "+
		"Answer the human directly and concisely.", project, wt)
}

// chatPoster posts one Chat turn's reply to Code Central as it arrives:
// its items every batch interval, then its outcome. A first turn's
// session ID and Resume Command go with the first post that holds items,
// or with the outcome when the reply didn't fail, so a first turn that
// failed before claude said anything leaves the Conversation with no
// session. The turn's usage waits until Code Central has the session,
// since it files usage by session; if it never gets it, the usage is
// dropped, as Code Central would drop it. A post that fails is dropped,
// but for the session, which goes with the next: it never fails the turn.
type chatPoster struct {
	f       *factory
	base    api.PostChatEventsInput // the Conversation, Factory and host
	session string                  // the turn's session, for its usage

	mu sync.Mutex
	// items wait for the next post.
	items []api.SessionEvent
	// sessionUnposted says the session ID and Resume Command are yet to
	// be posted; usage waits until they have been.
	sessionUnposted bool
	resumeCommand   string
	usage           []api.UsageResponse

	ended chan chatEnd // the outcome, from end
	done  chan struct{}
}

func (f *factory) newChatPoster(t api.ChatTurn, session, resume string) *chatPoster {
	return &chatPoster{
		f:       f,
		base:    api.PostChatEventsInput{Conversation: t.Conversation, Factory: factoryName, Host: f.host},
		session: session, sessionUnposted: t.First, resumeCommand: resume,
		ended: make(chan chatEnd), done: make(chan struct{}),
	}
}

// add queues an item of the reply.
func (p *chatPoster) add(e api.SessionEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = append(p.items, e)
}

// addUsage posts a model response's usage under the turn's session, once
// Code Central has the session.
func (p *chatPoster) addUsage(r api.UsageResponse) {
	p.mu.Lock()
	if p.sessionUnposted {
		p.usage = append(p.usage, r)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.f.usage.add(p.session, r)
}

// start times the first round and runs the poster until end.
func (p *chatPoster) start() {
	tick, waited := p.f.work.after(p.f.e.Clock, p.f.events.every, "waiting to post the Chat reply")
	go p.run(tick, waited)
}

// run posts the waiting items every batch interval until end, then the
// rest with the outcome. The interval is timed afresh after each round;
// waited ends the wait for tick.
func (p *chatPoster) run(tick <-chan time.Time, waited func()) {
	defer close(p.done)
	for {
		select {
		case <-tick:
			posting := p.f.work.busy("posting the Chat reply")
			waited()
			p.post(chatEnd{})
			tick, waited = p.f.work.after(p.f.e.Clock, p.f.events.every, "waiting to post the Chat reply")
			posting()
		case e := <-p.ended:
			waited()
			p.post(e)
			return
		}
	}
}

// chatEnd is how a reply ended: its outcome, with error text msg on a
// failed one. The zero chatEnd is a reply still running.
type chatEnd struct {
	outcome api.ChatOutcome
	msg     string
}

// end posts the outcome, with error text msg on a failed one, after the
// items still waiting, and returns once it has been posted or dropped.
func (p *chatPoster) end(outcome api.ChatOutcome, msg string) {
	p.ended <- chatEnd{outcome, msg}
	<-p.done
}

// post posts the waiting items, with e's outcome if the reply has ended.
// Nothing is posted when there is neither.
func (p *chatPoster) post(e chatEnd) {
	in := p.base
	in.Outcome, in.Error = e.outcome, e.msg
	p.mu.Lock()
	in.Items, p.items = p.items, nil
	reporting := p.sessionUnposted && (len(in.Items) > 0 || in.Outcome != "" && in.Outcome != api.ChatFailed)
	if reporting {
		in.SessionID, in.ResumeCommand = p.session, p.resumeCommand
	}
	p.mu.Unlock()
	if len(in.Items) == 0 && in.Outcome == "" {
		return
	}
	// Real time, not the clock, like every deadline on a call to Code
	// Central: no test moves the clock while a call hangs.
	ctx, cancel := context.WithTimeout(context.Background(), p.f.events.timeout)
	defer cancel()
	// The session goes again with the next post until one is taken.
	if err := p.f.client.PostChatEvents(ctx, p.f.project, in); err != nil || !reporting {
		return
	}
	p.mu.Lock()
	usage := p.usage
	p.sessionUnposted, p.usage = false, nil
	p.mu.Unlock()
	for _, r := range usage {
		p.f.usage.add(p.session, r)
	}
}
