# How agent sessions are taught to use Code Central

Type: grilling
Status: resolved
Blocked by: 12

## Question

How does an agent session learn to use Code Central? When an agent sets up a repository, what does it write there so that sessions point at the workflow the MCP server publishes rather than copying it, as The State workflow decided? Does this replace `docs/agents/issue-tracker.md` for this repo and others, and how do the skills that read that file keep working? How do these instructions make the unchanged grilling skill post Rounds to the Ticket, end the turn and start `code-central wait`, and how is the hook that adds the session ID to each Round installed? How a Project is identified decided that `code-central init` is run by the human and writes `.mcp.json`. What, if anything, should it write besides that?

[How agents claim work](09-claiming-work.md) adds to this: the hook now also adds the session ID to the claim and sends the throttled `PostToolUse` ping, and the published workflow gains the branch rule and the limits of holding several Claims. Where does that branch rule go, given that Code Central knows nothing about git?

[Process shape and web stack](10-process-and-stack.md) adds the setup outside the repository: `code-central service install` to keep the service running, `sbx policy allow network localhost:7733` for Docker Sandboxes, and `--bind` for plain Linux Docker. Where are these taught, and does `init` mention them?

[The MCP and HTTP surface](12-api-surface.md) puts some teaching into the server itself: the MCP `instructions` point agents to `get_workflow`, and each write's response carries the next step, such as "end your turn and run `code-central wait <id>` in the background". The hook fills in `session_id` on `claim` and `post_round` through `updatedInput`, and the `PostToolUse` ping runs `code-central ping` with the hook's JSON on stdin. It is throttled on the server and always exits `0`. Given that, what still has to be written into the repository, and how much can the instructions there leave to the server?

## Answer

Amended by [How Code Central is upgraded](21-upgrades.md), which made the `.mcp.json` URL, the server name and the hook subcommands a permanent contract, and put the writing version in each generated file's header.

The server does the teaching and the repository only holds pointers. Nothing committed to a repository describes the workflow, so nothing there can go stale.

**What the server teaches.**

- **MCP `instructions`** reach every session. They say what Code Central is, to call `get_workflow` before working a Ticket, and the Round rule: while a session holds a Claim, every question for the human is a Round posted with `post_round`, never printed in the terminal. That includes the unchanged grilling skill: each numbered question becomes one Question, with its recommendation as `recommended`. `post_round`'s response then says to end the turn and run `code-central wait`.
- **`get_workflow`** holds everything else, including the branch rule (work happens on a branch named after the Ticket ID, committed and pushed often) and the limits of holding several Claims. Code Central states the branch rule as a convention and never checks it.
- **Write responses** carry the next step at the moment it matters.

**Rounds go to Code Central whenever a Claim is held,** even with the human at the terminal, because an agent can't tell who is present and only the UI records answers. To grill in the terminal, don't claim.

**Context budget.** These are limits, not targets, and a `core` test fails if the instructions or the `get_workflow` prose go over them:

| Text | Paid | Limit |
| --- | --- | --- |
| MCP `instructions` | Every session | About 150 words |
| Tool descriptions | Every session, unless deferred | One line each; detail in the schema |
| `get_workflow` prose | Once per session that works a Ticket | About 1,500 tokens |
| Next step on a write | When the write is made | One sentence |
| Generated `docs/agents/` files | When a skill reads them | About 400 tokens each |

**What `init` writes.** `init` still never commits. It has two modes:

| File | Committed (default) | `--local` |
| --- | --- | --- |
| `.mcp.json` | Written | Written, and added to `.git/info/exclude` |
| Hooks | `.claude/settings.json` | `.claude/settings.local.json` |
| `docs/agents/issue-tracker.md`, `triage-labels.md` | Written | Written, and added to `.git/info/exclude` |
| Pointer to the tracker doc | `CLAUDE.md`, only if nothing there points at it yet | `CLAUDE.local.md` |

The Project Key lookup through the nearest `.mcp.json` works the same in both modes. The store remembers each Project's mode, and running `init` again follows it. A flag that conflicts with the stored mode asks before switching. `AGENTS.md` isn't touched in v1.

**The hooks** use the server name `code-central` and call subcommands, so their logic changes with the binary:

- `PreToolUse` on `mcp__code-central__claim|mcp__code-central__post_round` runs `code-central hook session-id`, which returns `updatedInput` with `session_id` added.
- `PostToolUse` on every tool runs `code-central ping`.

Without the binary, both fail without blocking the session.

**The CLI** uses `CLAUDE_CODE_SESSION_ID` as the default `session_id` when it is set, with `--session` taking priority, so a Factory or skill working through Bash still records it.

**Running `init` again** on a known key leaves the key alone and rewrites the hook entries and the `docs/agents/` files to the current templates. Files `init` wrote carry a header comment. For any other file, it shows a diff and asks before replacing it. With the human's confirmation, `init` can also join a key that already exists, which sets up a new worktree or fresh clone of a `--local` Project. This amends How a Project is identified, where a taken key was always refused. Joining needs confirmation, so a mistyped key can't quietly merge two Projects.

**The generated tracker doc** replaces `docs/agents/issue-tracker.md` in each Project. It maps the skills' operations to Code Central operations by name only and points at `get_workflow` for the rules:

- **Publishing:** `create_spec` or `create_ticket`.
- **Fetching:** `get_ticket` or `get_spec`.
- **Comments:** `comment`.
- **Wayfinding:**
  - The map is a Spec with the `wayfinder:map` Label, and its child tickets are Tickets on it.
  - Blocking is `add_blocker`, and the frontier is `find_tickets(spec, claimable)`.
  - Claiming is `claim(ticket)`, and resolving is `finish`, followed by the human accepting it.

`triage-labels.md` maps roles to States where one fits: `needs-triage` to `Draft`, `ready-for-agent` to `Ready` (as Import does), and `wontfix` to `Cancelled`. `needs-info` and `ready-for-human` stay Labels.

**Setup outside the repository** is taught where it's needed. The `service_unavailable` error names `serve` and `service install`, and `code-central help sandboxes` covers `sbx policy allow network localhost:7733` and `--bind`. `init`'s last line points to that topic, and the connection errors that can come from a sandbox do too. `init` doesn't detect sandboxes.

**This repository** stays on local markdown until Code Central v1 can bring it over through the Scratch Import.

**Scope.** `--local` brings repositories that can't take committed files into v1, which narrows the map's out-of-scope line on shared repositories. Shared use by other people stays out of scope.

Considered and rejected: copying the workflow into repository docs, because it would go stale. Putting the Round rule in a repository file, because the MCP `instructions` already reach every session. Only committed hooks, because shared repositories can't take them. Claude Code's local MCP scope for `--local`, because it is keyed by path, which breaks the CLI's key lookup and every worktree. Sending Rounds to the terminal when a human seems present, because an agent can't tell and the answer wouldn't be recorded. Leaving Wayfinding out of the tracker doc, because the skills used most couldn't run against Code Central. Teaching sandbox setup in `init`, because most Projects don't need it.

Not verified: `CLAUDE_CODE_SESSION_ID` in Bash subprocesses, and `updatedInput` on MCP tools. Both carry over from How agents claim work. Whether Claude Code defers Code Central's tools through tool search, which affects the per-session cost.
