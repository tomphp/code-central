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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/tomoram/code-central/core"
)

const (
	// chatWorktreeName is the Chat worktree's folder, beside the Ticket
	// worktrees: a Ticket ID is never chat.
	chatWorktreeName = "chat"
	// kindChat is the kind a Chat turn's model responses are posted
	// under in Usage.
	kindChat = "chat"
	// eventDenials is the kind of the item listing the calls a Chat
	// turn had denied, from its result's permission_denials.
	eventDenials = "permission_denials"
	// chatRetry is how long the Chat loop waits after a next_chat_work
	// that failed before it calls again.
	chatRetry = 5 * time.Second
)

func (f *factory) chatWorktree() string { return filepath.Join(f.worktrees, chatWorktreeName) }

// chatLoop keeps one next_chat_work call open until ctx ends or the
// Factory drains, and starts each turn it hands out, one at a time. It
// returns once the last turn has ended. With --no-chat it never calls.
func (f *factory) chatLoop(ctx context.Context) {
	if f.o.noChat {
		return
	}
	var turns sync.WaitGroup
	defer turns.Wait()
	// Draining closes the long-poll; a running turn goes on until ctx ends.
	pollCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-f.drainCh:
			cancel()
		case <-pollCtx.Done():
		}
	}()
	var turnRunning sync.Mutex // held by the running turn
	failing := false
	for pollCtx.Err() == nil {
		w, err := f.client.NextChatWork(pollCtx, f.project, core.NextChatWorkInput{Factory: factoryName, Host: f.host})
		if err != nil {
			if pollCtx.Err() != nil {
				return
			}
			if !failing {
				f.printf("warning: can't take Chat work from Code Central: %v", err)
			}
			failing = true
			select {
			case <-pollCtx.Done():
			case <-time.After(chatRetry):
			}
			continue
		}
		failing = false
		if w.Turn == nil {
			continue
		}
		turns.Add(1)
		go func(t core.ChatTurn) {
			defer turns.Done()
			// Code Central hands out a turn only once the reply before
			// it has ended, which its turn posts just before it returns.
			turnRunning.Lock()
			defer turnRunning.Unlock()
			f.runChatTurn(ctx, t)
		}(*w.Turn)
	}
}

// runChatTurn answers one Chat Message and posts the reply as it comes:
// its items, then its outcome.
func (f *factory) runChatTurn(ctx context.Context, t core.ChatTurn) {
	wt := f.chatWorktree()
	session := t.SessionID
	if t.First {
		session = newSessionID()
	}
	p := f.newChatPoster(t, session, wt)
	go p.run()
	outcome, msg := f.answerChat(ctx, t, session, wt, p)
	if outcome == core.ChatFailed {
		f.printf("chat: the reply failed: %s", firstLine(msg))
	}
	p.end(outcome, msg)
}

// answerChat runs t's claude turn in the Chat worktree wt, as session,
// and says how the reply ended, with the error text of a failed one. A
// first turn moves the Chat worktree to the base's tip first.
func (f *factory) answerChat(ctx context.Context, t core.ChatTurn, session, wt string, p *chatPoster) (core.ChatOutcome, string) {
	if t.First {
		if err := f.chatWorktreeAtBase(wt); err != nil {
			return core.ChatFailed, err.Error()
		}
	}
	if !f.accepts.takesEffort(t.Effort) {
		return core.ChatFailed, fmt.Sprintf("The effort `%s` isn't one this Factory's claude accepts (%s), so no reply was started. Pick another in New conversation, or set the Chat row's in Project Options → Models.", t.Effort, strings.Join(f.accepts.levels, ", "))
	}
	// A turn that waited on the one before it starts nothing once the
	// Factory is stopping.
	if ctx.Err() != nil {
		return core.ChatStopped, ""
	}
	how := "--resume"
	if t.First {
		how = "--session-id"
		f.printf("chat: starting a Conversation in %s; resume: %s", wt, chatResumeCommand(wt, session))
	}
	args := append([]string{"-p", how, session}, f.flags(turnChoice{model: t.Model, effort: t.Effort}, nil, chatPrompt(f.project, wt))...)
	cmd := exec.Command(f.claude, append(args, t.Text)...)
	cmd.Dir = wt
	cmd.Env = f.e.Environ
	cmd.Stderr = f.e.Stderr
	// Its own process group, as a Claim's turn is, so the terminal's
	// Ctrl-C reaches only the Factory.
	startInOwnGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		return core.ChatFailed, fmt.Sprintf("The Factory couldn't start claude: %v", err)
	}
	ended := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			// As a Claim's turn is stopped.
			f.stopProcess(cmd.Process, ended, func(err error) { f.printf("chat: can't stop the reply: %v", err) })
		case <-ended:
		}
	}()
	end := readTurn(cmd, stdout, wt, newUsageReader(kindChat, f.e.Now), func(e core.SessionEvent) {
		p.add(e)
		if f.o.verbose && e.Kind == eventToolCall {
			f.printf("chat: %s", e.Text)
		}
	}, p.addUsage)
	close(ended)
	if end.Result != nil && len(end.Result.PermissionDenials) > 0 {
		p.add(denialsItem(end.Result.PermissionDenials, f.e.Now()))
	}
	if ctx.Err() != nil {
		return core.ChatStopped, ""
	}
	return chatOutcome(t, end)
}

// chatOutcome is how a Chat turn that ran ended: done, or failed with the
// Factory's error text, naming a model claude rejected.
func chatOutcome(t core.ChatTurn, end turnEnd) (core.ChatOutcome, string) {
	if end.apiError() == modelNotFound {
		model := "`" + t.Model + "`"
		if t.Model == "" {
			model = "its default"
		}
		return core.ChatFailed, fmt.Sprintf("claude rejected the model %s (model_not_found): it doesn't exist, or this account can't use it. Pick another in New conversation, or set the Chat row's in Project Options → Models.", model)
	}
	if end.ExitCode == 0 && end.Signal == "" && end.Result != nil && !end.Result.IsError {
		return core.ChatDone, ""
	}
	msg := "claude ended the reply with " + end.how()
	if api := end.apiError(); api != "" {
		msg += ", API error " + api
	}
	if end.Result != nil && strings.TrimSpace(end.Result.Result) != "" {
		msg += ": " + end.Result.Result
	}
	return core.ChatFailed, msg
}

// denialsItem is the item listing the distinct calls a Chat turn had
// denied.
func denialsItem(ds []denial, now time.Time) core.SessionEvent {
	return core.SessionEvent{Time: now, Kind: eventDenials, Text: "Auto mode denied these calls:\n" + denialList(ds)}
}

// chatWorktreeAtBase makes the Chat worktree wt detached at the base's
// tip for a Conversation's first turn. Anything it can't move is left as
// it was, with an error saying what is there and how to clear it.
func (f *factory) chatWorktreeAtBase(wt string) error {
	left, err := f.detachAtBase(wt)
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
	return fmt.Errorf("The Chat worktree at %s has uncommitted changes, so the Factory can't move it to %s's tip for a new Conversation. Changed files:\n\n%s\n\nA Chat writes nothing to the repository. Keep anything worth keeping elsewhere, then clean the worktree with `git -C %s reset --hard` and `git -C %s clean -fd`, and send the message again.", wt, f.baseBranch(), left.dirty, wt, wt)
}

// chatResumeCommand brings a Conversation's session back from anywhere,
// so it changes into the Chat worktree first.
func chatResumeCommand(wt, session string) string {
	return "cd " + wt + " && claude --resume " + session
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
	base    core.PostChatEventsInput // the Conversation, Factory and host
	session string                   // the turn's session, for its usage

	mu sync.Mutex
	// items wait for the next post.
	items []core.SessionEvent
	// sessionUnposted says the session ID and Resume Command are yet to
	// be posted; usage waits until they have been.
	sessionUnposted bool
	resumeCommand   string
	usage           []core.UsageResponse

	ended chan chatEnd // the outcome, from end
	done  chan struct{}
}

func (f *factory) newChatPoster(t core.ChatTurn, session, wt string) *chatPoster {
	return &chatPoster{
		f:       f,
		base:    core.PostChatEventsInput{Conversation: t.Conversation, Factory: factoryName, Host: f.host},
		session: session, sessionUnposted: t.First, resumeCommand: chatResumeCommand(wt, session),
		ended: make(chan chatEnd), done: make(chan struct{}),
	}
}

// add queues an item of the reply.
func (p *chatPoster) add(e core.SessionEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.items = append(p.items, e)
}

// addUsage posts a model response's usage under the turn's session, once
// Code Central has the session.
func (p *chatPoster) addUsage(r core.UsageResponse) {
	p.mu.Lock()
	if p.sessionUnposted {
		p.usage = append(p.usage, r)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	p.f.usage.add(p.session, r)
}

// run posts the waiting items every batch interval until end, then the
// rest with the outcome.
func (p *chatPoster) run() {
	defer close(p.done)
	tick := time.NewTicker(p.f.events.every)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			p.post(chatEnd{})
		case e := <-p.ended:
			p.post(e)
			return
		}
	}
}

// chatEnd is how a reply ended: its outcome, with error text msg on a
// failed one. The zero chatEnd is a reply still running.
type chatEnd struct {
	outcome core.ChatOutcome
	msg     string
}

// end posts the outcome, with error text msg on a failed one, after the
// items still waiting, and returns once it has been posted or dropped.
func (p *chatPoster) end(outcome core.ChatOutcome, msg string) {
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
	reporting := p.sessionUnposted && (len(in.Items) > 0 || in.Outcome != "" && in.Outcome != core.ChatFailed)
	if reporting {
		in.SessionID, in.ResumeCommand = p.session, p.resumeCommand
	}
	p.mu.Unlock()
	if len(in.Items) == 0 && in.Outcome == "" {
		return
	}
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
