package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/domain"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/factory/factorytest"
)

// chatCalls are the runs of the fake claude for Chat turns, oldest
// first: the ones whose prompts name no Ticket.
func chatCalls(h *factorytest.Harness) []factorytest.Call {
	var out []factorytest.Call
	for _, c := range h.Calls() {
		if c.Ticket == "" {
			out = append(out, c)
		}
	}
	return out
}

// chatWorktree is where the Factory keeps its Chat worktree.
func chatWorktree(h *factorytest.Harness) string { return h.Worktree("chat") }

// reply is the Conversation's last Chat Message, Claude's reply.
func reply(cv api.ConversationView) api.ChatMessageView { return cv.Messages[len(cv.Messages)-1] }

// itemsOf is the kinds and texts of the reply's items, as "kind: text".
func itemsOf(m api.ChatMessageView) []string {
	var out []string
	for _, it := range m.Items {
		out = append(out, string(it.Kind)+": "+it.Text)
	}
	return out
}

var uuid = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestAChatMessageStartsANewSessionInTheChatWorktreeDetachedAtTheBase(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start("--", "--chrome")

	h.SendChat("What does this Project do?")
	cv := h.WaitForReply()

	calls := chatCalls(h)
	if len(calls) != 1 {
		t.Fatalf("Chat calls %+v, want one", calls)
	}
	c := calls[0]
	session, ok := c.Flag("--session-id")
	if !ok || !uuid.MatchString(session) || c.Args[0] != "-p" || c.Args[1] != "--session-id" {
		t.Errorf("args %q, want -p --session-id <a new UUID>", c.Args)
	}
	if c.Has("--resume") {
		t.Errorf("args %q resume, want a new session", c.Args)
	}
	if c.Dir != chatWorktree(h) {
		t.Errorf("ran in %s, want the Chat worktree %s", c.Dir, chatWorktree(h))
	}
	if head, tip := h.GitIn(chatWorktree(h), "rev-parse", "HEAD"), h.Git("rev-parse", "main"); head != tip {
		t.Errorf("Chat worktree at %s, want the base's tip %s", head, tip)
	}
	if b := h.GitIn(chatWorktree(h), "branch", "--show-current"); b != "" {
		t.Errorf("Chat worktree on branch %q, want detached", b)
	}
	if m, _ := c.Flag("--model"); m != core.DefaultChatModel {
		t.Errorf("--model %q, want the Chat default %q", m, core.DefaultChatModel)
	}
	if c.Has("--effort") {
		t.Errorf("args %q set an effort, want Claude Code's default", c.Args)
	}
	for _, want := range [][2]string{{"--output-format", "stream-json"}, {"--permission-mode", "auto"}, {"--permission-prompts", "none"}} {
		if v, _ := c.Flag(want[0]); v != want[1] {
			t.Errorf("%s %q, want %q", want[0], v, want[1])
		}
	}
	if !c.Has("--verbose") || !c.Has("--chrome") {
		t.Errorf("args %q, want --verbose and the Claude Args", c.Args)
	}
	if c.Prompt != "What does this Project do?" || c.Args[len(c.Args)-2] != "--" {
		t.Errorf("args %q, want the Chat Message after --", c.Args)
	}
	for _, want := range []string{
		"a Chat from the Code Central UI about Project acme",
		"no Claim, so no Claim ID, claim, finish or Round applies",
		"write nothing, commit nothing and create no branch",
		"only when the human asks in the Chat",
		"directly and concisely",
	} {
		if !strings.Contains(c.SystemPrompt, want) {
			t.Errorf("system prompt %q doesn't say %q", c.SystemPrompt, want)
		}
	}

	if cv.SessionID != session {
		t.Errorf("Conversation's session %q, want the turn's %q", cv.SessionID, session)
	}
	if want := "cd " + chatWorktree(h) + " && claude --resume " + session; cv.ResumeCommand != want {
		t.Errorf("Resume Command %q, want %q", cv.ResumeCommand, want)
	}
	r := reply(cv)
	if r.Outcome != api.ChatDone || r.Error != "" {
		t.Errorf("reply ended %q (%q), want done", r.Outcome, r.Error)
	}
	if got, want := itemsOf(r), []string{"text: " + factorytest.ChatReply}; !slicesEqual(got, want) {
		t.Errorf("reply items %q, want %q", got, want)
	}
}

func slicesEqual(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func TestAChatTurnUsesTheConversationsModelAndEffort(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start("--model", "opus", "--effort", "high")
	if _, err := h.Human().StartConversation(context.Background(), h.Project, api.StartConversationInput{Model: "sonnet", Effort: "low"}); err != nil {
		t.Fatal(err)
	}

	h.SendChat("Hi")
	h.WaitForReply()

	c := chatCalls(h)[0]
	if m, _ := c.Flag("--model"); m != "sonnet" {
		t.Errorf("--model %q, want the Conversation's sonnet, not the Factory's flag", m)
	}
	if e, _ := c.Flag("--effort"); e != "low" {
		t.Errorf("--effort %q, want the Conversation's low, not the Factory's flag", e)
	}
}

func TestTheChatTurnsItemsReachGetChatAsTheyArrive(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat,
		factorytest.Say("Looking."),
		factorytest.ToolUse("Bash", `{"command":"ls"}`),
		factorytest.ToolResult("toolu_1", "README.md", false),
		factorytest.Wait(),
		factorytest.Say("Done looking."),
		factorytest.Result("success"))
	run := h.Start()

	h.SendChat("List the files")
	run.PollUntil("the reply's first items while it runs", func() bool {
		cv := h.Chat()
		return cv.Running && len(reply(cv).Items) == 3
	})
	h.Continue(factorytest.Chat)
	r := reply(h.WaitForReply())

	if r.Outcome != api.ChatDone {
		t.Errorf("reply ended %q (%q), want done", r.Outcome, r.Error)
	}
	kinds := []string{}
	for _, it := range r.Items {
		kinds = append(kinds, string(it.Kind))
	}
	if want := []string{"text", "tool_call", "tool_result", "text"}; !slicesEqual(kinds, want) {
		t.Errorf("reply items %q, want %q", itemsOf(r), want)
	}
}

func TestASecondChatMessageResumesTheSession(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start()
	h.SendChat("First")
	first := h.WaitForReply()

	h.SendChat("Second")
	cv := h.WaitForReply()

	calls := chatCalls(h)
	if len(calls) != 2 {
		t.Fatalf("Chat calls %+v, want two", calls)
	}
	c := calls[1]
	if s, _ := c.Flag("--resume"); s != first.SessionID || c.Args[1] != "--resume" {
		t.Errorf("args %q, want -p --resume %s", c.Args, first.SessionID)
	}
	if c.Has("--session-id") {
		t.Errorf("args %q start a new session, want a resume", c.Args)
	}
	if c.Dir != chatWorktree(h) || c.Prompt != "Second" {
		t.Errorf("ran %q in %s, want Second in the Chat worktree", c.Prompt, c.Dir)
	}
	if !strings.Contains(c.SystemPrompt, "a Chat from the Code Central UI") {
		t.Errorf("resume's system prompt %q, want the Chat instructions", c.SystemPrompt)
	}
	if cv.SessionID != first.SessionID || reply(cv).Outcome != api.ChatDone {
		t.Errorf("Conversation %+v, want the same session and a done reply", cv)
	}
}

func TestAChatPostThatFailsIsDroppedAndTheTurnGoesOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat, factorytest.Say("Lost."), factorytest.Wait(), factorytest.Say("Kept."), factorytest.Result("success"))
	run := h.Start()
	h.SetFailing("/chat/events")

	h.SendChat("Hi")
	run.PollUntil("a post of the reply to fail", func() bool { return h.Refused("/api/p/acme/chat/events") > 0 })
	h.SetFailing("")
	h.Continue(factorytest.Chat)
	cv := h.WaitForReply()

	r := reply(cv)
	if r.Outcome != api.ChatDone {
		t.Errorf("reply ended %q (%q), want done", r.Outcome, r.Error)
	}
	if got, want := itemsOf(r), []string{"text: Kept."}; !slicesEqual(got, want) {
		t.Errorf("reply items %q, want the failed post's dropped and the rest, %q", got, want)
	}
	if session, _ := chatCalls(h)[0].Flag("--session-id"); cv.SessionID != session || cv.ResumeCommand == "" {
		t.Errorf("Conversation's session %q (%q), want the turn's %q posted again after the failure", cv.SessionID, cv.ResumeCommand, session)
	}
}

func TestAChatTurnsPermissionDenialsArePostedAsAnItemBeforeTheOutcome(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat, factorytest.Say("I'll try."), factorytest.ResultDenied(factorytest.Denied("rm -rf build")))
	h.Start()

	h.SendChat("Clean up")
	r := reply(h.WaitForReply())

	if r.Outcome != api.ChatDone {
		t.Errorf("reply ended %q (%q), want done", r.Outcome, r.Error)
	}
	if n := len(r.Items); n != 2 || r.Items[1].Kind != "permission_denials" || !strings.Contains(r.Items[1].Text, "Bash(rm -rf build)") {
		t.Errorf("reply items %q, want the text then a permission_denials item naming Bash(rm -rf build)", itemsOf(r))
	}
}

func TestAFailingChatTurnEndsFailedWithNoFailureReport(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		steps []factorytest.Step
		want  string
	}{
		"non-zero exit": {[]factorytest.Step{factorytest.Say("Partway."), factorytest.Exit(3)}, "exit 3"},
		"error result":  {[]factorytest.Step{factorytest.Say("Partway."), factorytest.Result("error_during_execution")}, "error_during_execution"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := factorytest.New(t)
			h.Script(factorytest.Chat, tc.steps...)
			r := h.Start()

			h.SendChat("Hi")
			cv := h.WaitForReply()

			got := reply(cv)
			if got.Outcome != api.ChatFailed || !strings.Contains(got.Error, tc.want) {
				t.Errorf("reply ended %q (%q), want failed naming %q", got.Outcome, got.Error, tc.want)
			}
			if items := itemsOf(got); len(items) != 1 || items[0] != "text: Partway." {
				t.Errorf("reply items %q, want what it said before failing", items)
			}
			// It goes on taking Chat Messages.
			h.Script(factorytest.Chat, factorytest.DefaultChatScript...)
			h.SendChat("Again")
			if again := reply(h.WaitForReply()); again.Outcome != api.ChatDone {
				t.Errorf("next reply ended %q (%q), want done", again.Outcome, again.Error)
			}
			r.Settle()
			if out := r.Stdout(); strings.Contains(out, "report_failure") || strings.Contains(out, ": Failed: ") {
				t.Errorf("stdout %q, want no Failure Report", out)
			}
		})
	}
}

func TestAChatReplyCutOffByAnOutageEndsFailedAndTheFactoryGoesOn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat, factorytest.Say("Partway."), factorytest.APIError("rate_limit"), factorytest.Result("error_during_execution"), factorytest.Exit(1))
	r := h.Start()

	h.SendChat("Hi")
	got := reply(h.WaitForReply())
	r.Settle()

	if got.Outcome != api.ChatFailed || !strings.Contains(got.Error, "claude hit an Outage (`rate_limit`)") || !strings.Contains(got.Error, "Send the message again later.") {
		t.Errorf("reply ended %q (%q), want failed with the Outage text naming rate_limit", got.Outcome, got.Error)
	}
	if out := r.Stdout(); strings.Contains(out, "report_failure") || strings.Contains(out, ": Failed: ") {
		t.Errorf("stdout %q, want no Failure Report", out)
	}
	// Not paused: a Ready Ticket is still claimed.
	id := h.ReadyTicket("After the Outage")
	h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	r.Poll()
	h.WantState(id, domain.InProgress)
	h.Continue(id)
	r.Settle()
	h.WantState(id, domain.InReview)
}

func TestAChatReplyWithAnExpiredLoginEndsFailedAndTheFactoryKeepsRunning(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat, factorytest.APIError("authentication_failed"), factorytest.Result("error_during_execution"), factorytest.Exit(1))
	r := h.Start()

	h.SendChat("Hi")
	got := reply(h.WaitForReply())
	r.Settle()

	if got.Outcome != api.ChatFailed || !strings.Contains(got.Error, "(authentication_failed)") || !strings.Contains(got.Error, "`claude /login`") {
		t.Errorf("reply ended %q (%q), want failed with the login text naming claude /login", got.Outcome, got.Error)
	}
	if out := r.Stdout(); strings.Contains(out, "report_failure") || strings.Contains(out, ": Failed: ") {
		t.Errorf("stdout %q, want no Failure Report", out)
	}
	// The Factory keeps running: it answers the next message.
	h.Script(factorytest.Chat, factorytest.DefaultChatScript...)
	h.SendChat("Again")
	if again := reply(h.WaitForReply()); again.Outcome != api.ChatDone {
		t.Errorf("next reply ended %q (%q), want done", again.Outcome, again.Error)
	}
}

func TestARejectedChatModelEndsFailedNamingTheModel(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat, factorytest.ModelNotFound("claude-nope")...)
	h.Start()
	if _, err := h.Human().StartConversation(context.Background(), h.Project, api.StartConversationInput{Model: "claude-nope"}); err != nil {
		t.Fatal(err)
	}

	h.SendChat("Hi")
	cv := h.WaitForReply()

	r := reply(cv)
	if r.Outcome != api.ChatFailed || !strings.Contains(r.Error, "claude rejected the model `claude-nope`") {
		t.Errorf("reply ended %q (%q), want failed naming claude-nope", r.Outcome, r.Error)
	}
}

func TestAChatEffortClaudeDoesntAcceptEndsFailedWithNoTurn(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	// Set while the seeded levels allow it; this claude doesn't.
	if _, err := h.Human().StartConversation(context.Background(), h.Project, api.StartConversationInput{Effort: "xhigh"}); err != nil {
		t.Fatal(err)
	}
	h.ClaudeHelp(fewerLevels)
	h.Start()

	h.SendChat("Hi")
	cv := h.WaitForReply()

	r := reply(cv)
	if r.Outcome != api.ChatFailed || !strings.Contains(r.Error, "`xhigh`") || !strings.Contains(r.Error, "low, medium, high") {
		t.Errorf("reply ended %q (%q), want failed naming xhigh and the levels claude accepts", r.Outcome, r.Error)
	}
	if n := len(chatCalls(h)); n != 0 {
		t.Errorf("%d Chat calls, want no turn started", n)
	}
	if cv.SessionID != "" {
		t.Errorf("Conversation's session %q, want none after a failed first turn", cv.SessionID)
	}
}

func TestADirtyChatWorktreeFailsTheFirstTurnNamingTheFiles(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start()
	h.SendChat("First")
	h.WaitForReply()
	if err := os.WriteFile(filepath.Join(chatWorktree(h), "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Human().StartConversation(context.Background(), h.Project, api.StartConversationInput{}); err != nil {
		t.Fatal(err)
	}

	h.SendChat("Second conversation")
	cv := h.WaitForReply()

	r := reply(cv)
	if r.Outcome != api.ChatFailed {
		t.Fatalf("reply ended %q, want failed", r.Outcome)
	}
	for _, want := range []string{chatWorktree(h), "notes.txt", "git -C " + chatWorktree(h) + " clean"} {
		if !strings.Contains(r.Error, want) {
			t.Errorf("error %q doesn't name %q", r.Error, want)
		}
	}
	if n := len(chatCalls(h)); n != 1 {
		t.Errorf("%d Chat calls, want no turn started for the dirty worktree", n)
	}
	if cv.SessionID != "" {
		t.Errorf("Conversation's session %q, want none after a failed first turn", cv.SessionID)
	}
	if b, err := os.ReadFile(filepath.Join(chatWorktree(h), "notes.txt")); err != nil || string(b) != "mine" {
		t.Errorf("notes.txt %q (%v), want it left as it was", b, err)
	}
}

func TestANewConversationMovesACleanChatWorktreeToTheBasesTip(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Start()
	h.SendChat("First")
	h.WaitForReply()
	tip := h.CommitOn("main", "new.txt", "new")

	// A later turn of the same Conversation uses the worktree as it is.
	h.SendChat("Second")
	h.WaitForReply()
	if head := h.GitIn(chatWorktree(h), "rev-parse", "HEAD"); head == tip {
		t.Errorf("a later turn moved the Chat worktree to %s, want it left as it was", tip)
	}

	if _, err := h.Human().StartConversation(context.Background(), h.Project, api.StartConversationInput{}); err != nil {
		t.Fatal(err)
	}
	h.SendChat("New conversation")
	if r := reply(h.WaitForReply()); r.Outcome != api.ChatDone {
		t.Fatalf("reply ended %q (%q), want done", r.Outcome, r.Error)
	}
	if head := h.GitIn(chatWorktree(h), "rev-parse", "HEAD"); head != tip {
		t.Errorf("Chat worktree at %s, want the base's new tip %s", head, tip)
	}
}

func TestAChatTurnRunsWhileEveryClaimSlotIsFull(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	id := h.ReadyTicket("Long job")
	h.Script(id, factorytest.Wait(), factorytest.Finish("Done."), factorytest.Result("success"))
	run := h.Start("--parallel", "1")
	h.WantCalls(1)

	h.SendChat("Hi")
	if r := reply(h.WaitForReply()); r.Outcome != api.ChatDone {
		t.Errorf("reply ended %q (%q), want done while the slot is full", r.Outcome, r.Error)
	}
	run.Settle()
	if tk := h.Ticket(id); tk.State != domain.InProgress {
		t.Errorf("%s is %s, want its turn still running", id, tk.State)
	}
	h.Continue(id)
	run.Settle()
	h.WantState(id, domain.InReview)
}

func TestAChatTurnsUsageIsPostedWithKindChat(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	h.Script(factorytest.Chat,
		factorytest.Response("msg_1", "", "claude-haiku-5-5", 100, 0, 0, 10),
		factorytest.Say("Hi."),
		factorytest.Result("success"))
	r := h.Start()

	h.SendChat("Hi")
	cv := h.WaitForReply()

	var got []api.UsageSession
	r.PollUntil("the Chat turn's usage", func() bool {
		got = nil
		for _, s := range h.Usage().Sessions {
			if s.SessionID == cv.SessionID {
				got = append(got, s)
			}
		}
		return len(got) > 0
	})
	if s := got[0]; len(got) != 1 || s.Kind != "chat" || s.Ticket != "" || s.Claim != 0 || s.Responses != 1 {
		t.Errorf("usage sessions %+v, want one of kind chat with no Ticket", got)
	}
}

func TestTheSweepLeavesTheChatWorktreeAlone(t *testing.T) {
	t.Parallel()
	h := factorytest.New(t)
	r := h.Start()
	h.SendChat("Hi")
	h.WaitForReply()

	r.Poll()
	r.Poll()

	if _, err := os.Stat(filepath.Join(chatWorktree(h), ".mcp.json")); err != nil {
		t.Errorf("the Chat worktree is gone: %v", err)
	}
	if out := r.Stdout(); strings.Contains(out, "chat: get_ticket") || strings.Contains(out, "removed its worktree") {
		t.Errorf("stdout %q, want nothing said of the Chat worktree as a Ticket's", out)
	}
}
