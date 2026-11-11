# The Context Report

The Context Report measures what agents load into context while they work this repository's Tickets. It reads Claude Code's own session logs on your machine (`~/.claude/projects/<project-dir>/<session>.jsonl`, with subagents under `<session>/subagents/`), not Code Central. A change that cuts what agents load compares the report against a saved baseline, to show the size it saved and any sign that it cut too far. It is a developer tool for this repository, built from `internal/contextreport`. It is not [Usage](running-as-a-service.md#usage), the UI's view of the Code Central calls agents make: Usage records what Code Central sees, for every Project and kept 90 days, while the Context Report reads Claude Code's logs on one machine, covers every tool, and keeps no history of its own.

## Running it

From the repository, or any of its worktrees:

```console
$ make context-report ARGS="-baseline internal/contextreport/baseline.json -since 2026-11-10T00:00:00Z"
```

Flags go in `ARGS`:

| Flag | What it does |
| --- | --- |
| `-since <time>`, `-until <time>` | Only sessions started at or after `-since` and before `-until`, as RFC 3339 times. A cut's sessions are those since it landed. |
| `-baseline <file>` | Prints each figure's change against the baseline in `<file>`. |
| `-save <file>` | Writes the figures to `<file>`, as a baseline. |
| `-top <n>` | How many of the largest results, re-read IDs and wider reads to list (default 20). |
| `-projects <dir>`, `-repo <dir>` | Claude Code's projects folder (default `~/.claude/projects`) and the main checkout whose sessions to read (default this repository's). |

The report reads the sessions of the main checkout and its worktrees (`<repo>.worktrees/<ticket>`), because Claude Code names a session's folder after the path it ran in. A session's Ticket comes from what the Factory adds to its system prompt (`Ticket: <id>`), which Claude Code records in the log as a `prompt_snapshot`, and its kind is what the Stage Guide there names (`The Stage Guide for <stage> follows`): a Stage (`grilling`, `spec` or `tickets`) or `implementation`, or `conflict` for a Claim the Factory started with the instructions for resolving a merge conflict in its first prompt, whose system prompt has `Conflict Claim:` in place of the Stage Guide, so its cost compares with the `implementation` baseline. Sessions from before the Factory used the system prompt hold both in their first prompt instead. Sessions you start by hand have neither, so they are left out. A subagent has its parent's Ticket and start time, and its kind is `review` when it is one of the code-review subagents (its description starts `Spec review` or `Standards review`) and `subagent` otherwise.

## The figures

Sizes are bytes of text in each tool result, which is what the agent reads. A `ToolSearch` result only names the tools it loads, so it counts as 0B even though their schemas take up context. Against a baseline, every figure is compared per session, so windows with different numbers of sessions compare fairly. A kind with no sessions in the window says so instead.

- **Tool results per session, by kind of session.** Each kind's sessions, the total size of their tool results, and that total per session.
- **First turn per session, by kind of session.** The size of the text a session starts with: its prompts before the agent's first response (the skill's command and the expanded skill) and what the Factory adds to its system prompt.
- **Tools, and Code Central operations, per session, by kind of session.** The calls and result size per session of each tool, with every Code Central operation counted as the one tool `Code Central`, and then each operation's.
- **Largest tool results.** The largest results, with their kind, Ticket, tool and a short form of the input: a Bash command, a Read's file, or else the input as JSON. Each is cut to 80 characters (a file keeps its end), with the worktree's folder cut from paths and the home folder shortened to `~`. Against a baseline, each one is compared with the baseline's result for the same tool and input, or is `new` when the baseline doesn't list one.
- **Specs and Tickets read more than once.** Each ID read more than once with `get_spec` or `get_ticket`, and which Tickets' kinds read it. The IDs differ from one window to the next, so what compares against the baseline is two figures per session: reads again within a session (an ID's reads after its first in the same session) and reads again across sessions (the sessions that read an ID after the first one did).
- **Signs a cut went too far, per session, by kind of session.**
  - **Wider reads:** a `get_spec` or `get_ticket` of an ID the session already read, asking for parts, sections of the body, or `full` that the earlier calls didn't fetch. A read that names no parts fetches its default parts: for `get_spec`, the body, tickets and edits, or only the body when it names sections; for `get_ticket`, its claims, Failure Reports and comments (ADR 0015), so asking for its rounds, moves or edits afterwards counts as wider. When an operation's default parts change, update `defaultParts` and `readScope` in `internal/contextreport/report.go`.
  - **Rounds and Questions:** `post_round` calls and their Questions.
  - Each wider read is then listed, with what it asked for first and then.

## The baseline

`internal/contextreport/baseline.json` holds the figures from every Factory session that started before 2026-11-09T10:43:00Z, when commit `9541c06` made the first cut. It holds sizes, counts, IDs and short inputs, never a tool result's content. It was saved with:

```console
$ make context-report ARGS="-until 2026-11-09T10:43:00Z -save internal/contextreport/baseline.json"
Context report: 204 sessions started from 2026-11-08T15:21:46Z to 2026-11-09T10:06:53Z

Tool results per session, by kind of session
KIND            SESSIONS  TOTAL    PER SESSION
grilling        11        878.9KB  79.9KB
spec            11        949.8KB  86.3KB
tickets         11        651.8KB  59.3KB
implementation  69        6.9MB    102.5KB
review          98        3.5MB    36.9KB
subagent        4         346.1KB  86.5KB
…
Code Central operations, per session, by kind of session
…
implementation        CALLS     SIZE
  get_spec            0.797     21.5KB
  finish              1         5.6KB
  get_ticket          1.1       5.3KB
  get_workflow        0.406     4.3KB
…
Largest tool results
46.2KB  review          code-central-110  Read       …ode-central-110/eeefdafe-aa2e-40b9-9c58-e087dbdf962b/tool-results/b4oon60v5.txt
44.1KB  implementation  code-central-141  get_spec   {"spec":"code-central-120"}
…
Specs and Tickets read more than once
code-central-124  13 reads in 13 sessions  code-central-115 tickets, code-central-125 grilling, code-central-126 spec, …
…
Reads again within a session  0.0049
Reads again across sessions   0.627

Signs a cut went too far, per session, by kind of session
KIND            WIDER READS    ROUNDS    QUESTIONS
grilling        0              1.27      8.27
spec            0              0.0909    0.273
tickets         0              0         0
implementation  0              0.029     0.029
review          0.0102         0         0
subagent        0              0         0
…
```

Compared against it, a later window prints each figure's change: `unchanged`, a percentage with the baseline's figure, or `new` when the baseline had none.

```console
$ make context-report ARGS="-since 2026-11-09T10:43:00Z -baseline internal/contextreport/baseline.json"
Context report: 5 sessions started from 2026-11-09T11:04:38Z to 2026-11-09T11:13:09Z
Baseline: 204 sessions started from 2026-11-08T15:21:46Z to 2026-11-09T10:06:53Z

Tool results per session, by kind of session
KIND            SESSIONS  TOTAL    PER SESSION  CHANGE
grilling        1         43.4KB   43.4KB       -46% (was 79.9KB)
implementation  2         293.8KB  146.9KB      +43% (was 102.5KB)
review          2         114.9KB  57.4KB       +56% (was 36.9KB)
spec            0                               no sessions (was 86.3KB per session)
…
Code Central operations, per session, by kind of session
…
implementation        CALLS  CHANGE              SIZE    CHANGE
  get_ticket          1      -9% (was 1.1)       4.4KB   -18% (was 5.3KB)
  pick_up_answers     0.5    +1625% (was 0.029)  3.2KB   +6545% (was 50B)
  get_spec            2      +151% (was 0.797)   2.9KB   -87% (was 21.5KB)
…
```

Save a new baseline only when the Spec it measures asks for one, and commit it in the same change.
