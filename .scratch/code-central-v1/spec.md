# Code Central v1

Status: ready-for-agent
Labels: spec

Source: the wayfinder map in [map.md](map.md) and its resolved tickets in `issues/`. ADRs 0001–0004 in `docs/adr/` record the hard-to-reverse choices. Domain terms are as defined in `GLOSSARY.md`.

## Problem Statement

I run AI agents across many repositories, and the work they do lives in scattered `.scratch/` folders of Markdown. No one place tells me what needs me. An agent stuck on a question waits in some terminal I'm not looking at. A failed run leaves no record of what broke, why, or what to fix. A session that died quietly keeps holding work nobody else will touch. Nothing stops two agents taking the same piece of work, and each repository's notes on how to track work drift out of date. When I want to know what to do next, I have to go through every repository and every terminal by hand.

## Solution

Code Central is a single-user, local coordination point: one service on my machine that holds every Project's Specs and Tickets and shows me what needs me. It does not start or control agents.

- Agents connect over MCP (or a matching `code-central` CLI with JSON output). They claim Tickets atomically, ask me Questions in Rounds, report failures with a fixed structure, and hand finished work to me for review.
- I work from one web UI, a triage inbox. Needs Me sits at the top, grouped by reason. Each reason has an action panel: answer a Round, resume or release a stuck Claim, read a Failure Report and send the work back, or accept or reject finished work.
- The service sends OS Notifications when an agent is stopped and waiting on me, and the browser tab shows the Needs Me count.
- `code-central init` sets a repository up as a Project with only pointer files. The server itself teaches agents the workflow, so nothing in a repository goes stale.
- A one-way Scratch Import brings an existing `.scratch/` folder in.
- External ticketing systems can be connected later through Integrations, which are separate HTTP clients. v1 fixes the contract for them but ships no Integration.

## User Stories

### Setting up

1. As the human, I want to run `code-central serve` in the foreground, so that I can start the service by hand while trying it out.
2. As the human, I want `code-central service install` to run the service as a login item (launchd on macOS, systemd user unit on Linux) that restarts after a crash, so that agents can always reach it.
3. As the human, I want `service install` to be safe to run again, so that re-running it after an upgrade just rewrites the unit and restarts the service.
4. As the human, I want `code-central service restart`, so that an upgrade takes one command.
5. As the human, I want the service to refuse to start if port 7733 is taken, and to name the conflict, so that agents never end up on a surprise port.
6. As the human, I want a local setting that changes the port, used together with `CODE_CENTRAL_URL`, so that I can work around a real clash without changing committed files.
7. As the human, I want a second copy of the service on the same store to refuse to start, so that there is only ever one writer.
8. As the human, I want every CLI command to fail with a message naming `serve` and `service install` when the service isn't running, so that I know how to fix it.
9. As the human, I want `code-central init` to propose a Project Key from the repository's remote or folder name, and let me accept or override it, so that setup takes seconds.
10. As the human, I want `init` to refuse a key that is already taken unless I confirm I'm joining that Project, so that a mistyped key can't merge two Projects.
11. As the human, I want `init` to merge the server into an existing `.mcp.json` and never commit anything, so that I stay in control of my repository.
12. As the human, I want `init --local` to keep every generated file out of git (through `.git/info/exclude`, local settings and `CLAUDE.local.md`), so that I can use Code Central in a repository that can't take committed files.
13. As the human, I want the store to remember each Project's mode (committed or local), and `init` to ask before switching it, so that re-running `init` does what I expect.
14. As the human, I want re-running `init` to refresh the hooks and generated tracker docs to the current templates, and to show a diff before replacing a file it didn't write, so that I can update a repository safely.
15. As the human, I want `init` to tell me when a Project is archived instead of quietly bringing it back, so that archiving means something.
16. As the human, I want `init` to work outside git, so that I can track work for a folder that isn't a repository yet.
17. As the human, I want `code-central help sandboxes` to explain `sbx policy allow network localhost:7733` and `--bind`, so that I can reach the service from a sandbox when I need to.
18. As the human, I want `--bind` to add one specific address (such as a Docker bridge IP), with a warning against LAN addresses, so that plain Linux Docker can reach the service without exposing it to the network.
19. As the human, I want the store kept under `$XDG_DATA_HOME/code-central` (falling back to `~/.local/share/code-central`), overridable with `CODE_CENTRAL_STORE` or `serve --store`, so that I know where my data lives and can point tests elsewhere.

### Agents connecting

20. As an agent, I want to connect by a URL that names my Project, so that no tool needs a Project argument and I can't file work in the wrong Project.
21. As an agent in a sandbox, I want the URL to honour `CODE_CENTRAL_URL`, so that I can reach the host while the committed file stays the same.
22. As an agent on an unknown Project Key, I want every tool call to return `unknown_project` and tell me to ask the human to run `code-central init`, so that I can explain the problem instead of seeing "server failed".
23. As an agent using the CLI, I want the Project found from `--project`, then `CODE_CENTRAL_PROJECT`, then the nearest `.mcp.json`, so that shell commands and MCP agree on the Project.
24. As an agent, I want short MCP `instructions` that say what Code Central is, to call `get_workflow` before working a Ticket, the Round rule, and to call `get_import_guide` to import a `.scratch/` folder, so that every session learns the essentials cheaply.
25. As an agent, I want `get_workflow` to return every State, its meaning, every action with who can take it and the content it requires, what counts as Needs Me, the three exits on trouble, the branch rule and the limits of holding several Claims, so that I follow the real rules rather than a stale copy.
26. As an agent, I want every write to tell me the next step in one sentence, so that the instruction arrives when it matters.
27. As an agent, I want to accept both `<key>-<n>` and a bare `<n>` as an ID, so that I can use whatever the human gives me.
28. As an agent, I want an ID with another Project's key rejected with an error naming my connection's Project, so that I don't act on the wrong work.

### Finding and defining work

29. As an agent, I want to create a Spec with a title and Markdown body, so that I can record a piece of work.
30. As an agent, I want to create a Ticket on a Spec in `Draft` or `Ready`, with Labels and Blockers, so that I can break a Spec into work.
31. As an agent, I want to edit a Spec's title or body and a Ticket's title or description, so that definitions can improve.
32. As the human, I want every edit that changes something to record its Actor, time and the previous text, so that an agent's rewrite of my Spec can be undone by hand.
33. As an agent, I want to add and remove Labels on Specs and Tickets one at a time rather than replace the whole list, so that two writers don't overwrite each other.
34. As an agent, I want to set and remove Tags (lowercase keys, single-line values) on Specs and Tickets, so that I can record structured facts with no workflow meaning.
35. As an agent, I want to add and remove Blockers between Tickets in the same Project, across Specs, with cycles rejected, so that work happens in the right order.
36. As an agent, I want to comment on any Ticket, even one another agent holds, so that I can triage without a Claim.
37. As an agent, I want `find_tickets` filtered by State, title substring, Labels, Tags, Spec and claimable, in claim order, so that I can find the work I need.
38. As an agent, I want `find_tickets` to leave out `Done` and `Cancelled` unless I name them, so that finished work doesn't crowd results.
39. As an agent, I want `get_ticket` to return one full view (stored fields, Spec, Blockers with their States, Replacements, Claims without Claim IDs, Rounds, Failure Reports, comments, moves, edits, and the computed `blocked`, `claimable`, `silent_since` and `needs_me_reason`), so that I have everything in one read.
40. As an agent, I want `get_spec` to return its body, a derived State summary and its Tickets, so that I can see a Spec's progress.
41. As an agent, I want `find_specs` to hide finished Specs unless I ask for them, and filter by title, Labels and Tags, so that I see live work first.
42. As an agent, I want to approve a `Draft` Ticket to `Ready`, so that I can release work I've finished defining.
43. As an agent, I want to defer a `Ready` Ticket to `Draft` with a required reason, so that work that isn't ready stops being claimable.
44. As an agent, I want to cancel a `Draft` or `Ready` Ticket only by naming its Replacements, so that work is never dropped without a successor.
45. As an agent, I want anything blocked by a Cancelled Ticket to become blocked by its Replacements, so that the order of work survives a split or supersede.
46. As the human, I want nothing to be deleted in v1, with cancelling in its place, so that history is never lost.

### Claiming work

47. As an agent, I want to claim the next claimable Ticket in my Project (`Ready`, no open Blockers, oldest first, ID as tie-break) in one atomic step, so that two agents never take the same Ticket.
48. As an agent, I want to narrow a claim to one Spec or one Ticket, so that I can work where the human pointed me.
49. As an agent, I want an empty result when nothing can be claimed, so that my loop knows to stop.
50. As an agent, I want a named Ticket that can't be claimed to be rejected with its State or its Blockers as the reason, so that I can report why.
51. As an agent, I want the claim to return a Claim ID with the full Ticket view, including past Claims and how each ended, so that I can pick up where a previous session left off.
52. As an agent, I want to send the Claim ID only on actions on my Claim, so that reads and triage writes stay simple.
53. As an agent, I want a write with an old Claim ID rejected with `stale_claim`, so that a released session stops instead of clobbering the next agent.
54. As an agent, I want to hold several Claims and pass a Claim ID to a subagent, so that I can work in parallel.
55. As an agent, I want my session ID recorded at the claim and with each Round (filled in by a hook, or from `CLAUDE_CODE_SESSION_ID` in the CLI), and updatable with `set_session`, so that the human can resume me.
56. As an agent, I want a Ticket rejected from review or sent back from `Failed` to keep its place near the front of the claim order, so that it isn't starved.

### Asking Questions

57. As an agent holding a Claim, I want to post a Round of Questions (each with a title, a body and an optional recommended answer) in one write, so that the human sees everything I need at once.
58. As an agent, I want posting a Round to move the Ticket to `Needs Answer` and tell me to end my turn and run `code-central wait` in the background, so that I stop cleanly.
59. As an agent, I want every question to go to a Round whenever I hold a Claim, even with the human at the terminal, so that answers are always recorded.
60. As an agent using the grilling skill, I want each numbered question to become one Question with its recommendation, so that the skill works unchanged.
61. As an agent, I want `code-central wait <ticket>` to exit `0` when the Ticket is `Answered`, `3` when it left for another State, and `1` on a usage error, retrying through connection errors, so that a live session wakes at the right time and survives a service restart.
62. As an agent, I want `pick_up_answers` to move `Answered` to `In Progress` in one step and return the Round's answers and dismissals plus comments added since, so that I can carry on with full context.
63. As the human, I want each answer box pre-filled with the recommended answer, labelled as such, and nothing stored until I submit it, so that answering is quick but deliberate.
64. As the human, I want to accept every recommended answer in a Round at once, so that easy Rounds take one click.
65. As the human, I want answers saved one by one and editable until the Round is complete, so that I can answer in pieces without waking the agent early.
66. As the human, I want to dismiss one Question inside a Round, so that I can decline a question without moving the Ticket.
67. As the human, I want the Ticket to move to `Answered` automatically when the last open Question is answered or dismissed, so that the agent is woken exactly once.
68. As the human, I want open Questions to become dismissed if I move a Ticket out of `Needs Answer` by hand, so that the next agent sees they were deliberately left unanswered.
69. As the human, I want a copyable `claude --resume <id>` on an `Answered` Ticket, so that I can resume a session that has exited.

### Failing, giving up and finishing

70. As an agent with no way forward, I want to post a Failure Report with four required fields (what broke, why, what to fix, where the work stands), so that the human sees exactly what happened.
71. As an agent, I want a report missing any field rejected, with the Ticket left `In Progress`, so that every `Failed` Ticket has a complete report.
72. As an agent, I want reporting a failure to end my Claim and tell me to stop, so that I don't keep working a Ticket I've given up on.
73. As the human, I want Failure Reports to be permanent and numbered, and the failure count shown, so that the next agent doesn't repeat failed attempts.
74. As the human, I want only the claim holder to be able to put a Ticket in `Failed`, so that a `Failed` Ticket always has a report to show.
75. As an agent that can't finish but found nothing wrong, I want to give up cleanly back to `Ready` with a required note on where the work stands, so that the next agent doesn't start cold.
76. As an agent, I want to finish a Ticket into `In Review` with a required summary (what was done, where, what to check), so that the human can review quickly.
77. As an agent picking up a Ticket that failed before, I want the claim to return every report and any send-back note, and the workflow to tell me a send-back without a note means nothing has changed, so that I never assume a fix was made.

### Needs Me and the UI

78. As the human, I want one triage inbox with Projects and their Needs Me counts on the left, the Needs Me list in the middle and the selected Ticket on the right, so that I see what needs me across every Project at once.
79. As the human, I want Needs Me grouped by reason, cheapest to unblock first (Questions, `Answered`, Silent Claim, `Failed`, `In Review`, blocked by a Cancelled Ticket, closed externally), longest-waiting first in each group, so that I unblock agents fastest.
80. As the human, I want each row to show the Ticket ID, title, how long it has waited and a one-line detail for its reason, so that I can triage without opening it.
81. As the human, I want Drafts in a collapsed, quiet group below Needs Me, so that they don't compete for attention.
82. As the human, I want to filter by Project and by any single State, combined, with a dot on rows that need me, so that I can look beyond Needs Me.
83. As the human, I want the selected Ticket's action panel first, then its Spec, Labels, Link, description, Claim activity and a collapsed history of moves, comments and edits, so that the action I need comes first.
84. As the human, I want an `In Review` panel with the `finish` summary, Accept, and Reject that needs a comment, so that reviewing is one decision.
85. As the human, I want a `Failed` panel with the failure count, the latest report's four fields, Send back (with an optional note), To Draft and Cancel, then the report's resume command and earlier reports collapsed, so that I can resolve a failure in place.
86. As the human, I want a Silent Claim shown with how long it has been quiet, then Resume, then Release, so that I try the cheap, safe option first.
87. As the human, I want Release to warn me that a live session's next write will be rejected and the Claim can't be restored, with an optional note, so that I release knowingly.
88. As the human, I want a Silent Claim to be an `In Progress` Ticket with no activity for 30 minutes, cleared by the next activity, so that a dead session is noticed without guessing a State.
89. As the human, I want a hook ping after each tool call, throttled by the server to once a minute per session and never holding up the tool call, so that busy sessions don't look silent.
90. As the human, I want a panel for a Ticket blocked by a Cancelled Ticket with no Replacement, offering remove the Blocker, To Draft or Cancel, so that stranded work is visible and fixable.
91. As the human, I want to move any Ticket to any State except `Failed`, so that I'm never stuck.
92. As the human, I want to rename a Project's display name, so that it reads well in the UI.
93. As the human, I want the UI to update live over SSE, refetching after each event and refetching everything after a reconnect, so that what I see is current.
94. As the human, I want the view and the selected Ticket in the URL, so that a Notification or a link can open the right place.
95. As the human, I want finished Tickets visible by filtering to `Done` or `Cancelled`, newest first, and any Ticket to open by its URL, so that I can look things up.

### Attention outside the UI

96. As the human, I want an OS Notification for a new Round, a new Failure Report or a Claim turning Silent, so that I hear about agents stopped on me with no browser open.
97. As the human, I want Notifications gathered over a 15-second window into one per event or one summary, so that a burst doesn't flood me.
98. As the human, I want clicking a Notification to open the Ticket or Needs Me where the platform makes it cheap, so that I can act right away.
99. As the human, I want `In Review`, `Answered`, blocked and closed-externally to count without notifying, so that I'm only interrupted when an agent is waiting.
100. As the human, I want one summary on startup if Needs Me isn't empty, so that I catch up after a restart.
101. As the human, I want a `notifications: on|off` setting and `serve --no-notify`, so that I can turn Notifications off.
102. As the human, I want the tab's title and favicon to show the Needs Me count, so that I can see it at a glance.

### Finished work and archiving

103. As the human, I want finished to be computed from State (a Ticket `Done` or `Cancelled`; a Spec with Tickets, all finished), so that nothing needs archiving by hand.
104. As the human, I want to archive a Project only once all its work is finished, so that archiving can never hide something that needs me.
105. As the human, I want an Archived Project to refuse every write while reads keep working, with the refusal listing `unarchive_project`, so that it stays put away but readable.
106. As the human, I want Archived Projects in a collapsed group at the bottom of the left pane, with Archive disabled and explained while work remains, so that the list stays clean.

### Importing a Scratch folder

107. As the human, I want an agent to map my whole `.scratch/` folder into one Import following `get_import_guide`, so that messy, version-specific formats are handled by judgement rather than parser code.
108. As the human, I want a dry run that returns the plan (each source path with its provisional ID, title, State, Labels, Tags and Blockers, grouped by Spec, with a count per State) and writes nothing, so that I can review before committing.
109. As the human, I want the agent to flag any Ticket whose State disagrees with its body, and any dropped Blocker, as doubts, and wait for my go-ahead, so that stale States don't come in unseen.
110. As the human, I want each feature folder to become one Spec (`spec.md`, `map.md`, both in that order, or an empty body titled after the folder), so that the structure survives.
111. As the human, I want Scratch States mapped (`resolved`/`done`/`closed` to `Done`, `ready-for-agent` to `Ready`, superseded or broken-into to `Cancelled` with Replacements, `wontfix` to `Cancelled` with a comment, `Deferred` to `Draft` with a comment, anything else to `Draft`), so that the work lands in the right place.
112. As the human, I want header lines turned into Labels and Tags, the original State line kept as `scratch.status`, and a `source` Tag with the path, so that nothing is lost and every item traces back.
113. As the human, I want an Import refused if any `source` already exists in the Project, listing the matches, so that running it twice does nothing.
114. As the human, I want an invalid payload rejected with a `field` path and nothing written, so that the agent can fix it precisely.
115. As the human, I want the Import written all or nothing, even across a crash, so that I never get half an Import.
116. As the human, I want a successful Import to recommend deleting `.scratch/` and committing that, so that there's one source of truth.

### Integrations (contract only)

117. As an Integration author, I want an Integration to be a separate program using the HTTP API with its own polling, configuration and credentials, so that Code Central never needs a rebuild or holds an external credential.
118. As an Integration author, I want a typed Link on Specs and Tickets (integration name, external ID, URL, external state, detached), so that I can record what I mirror.
119. As an Integration author, I want an `integration` Actor that alone can create linked items, update their definition fields and set the external state, but can't move State, so that ownership is never split on a field.
120. As the human, I want the title and description of a linked item read-only while the Link isn't detached, with a Link badge on rows and in the detail, so that I know where to edit them.
121. As the human, I want a linked Ticket whose external item closed, was deleted or left the query to show in Needs Me as "closed externally", with Cancel, Mark Done or Unlink, so that I decide what happens.
122. As the human, I want Unlink to detach the Link and make the definition editable, while lookups still find it, so that an Integration won't pull the item in again.
123. As an Integration author, I want to subscribe to the same SSE events as the UI, so that I can push a delivery summary outward.

### Upgrading

124. As the human, I want `serve` to migrate an older store format automatically through a staging folder, keeping a backup per old format, so that a login item never crash-loops after an upgrade.
125. As the human, I want a failed migration to leave the original store untouched and name the step and file, so that an upgrade can't corrupt my data.
126. As the human, I want a newer, unknown format refused by name, so that an older binary never misreads my store.
127. As the human, I want a CLI whose version differs from the service's to warn on stderr without failing, naming the older side when it can, so that a stale copy in a sandbox still works but I know to update.
128. As the human, I want the `.mcp.json` URL shape, port 7733, the server name `code-central`, the hook subcommands and the Import manifest to stay the same across versions, so that repositories set up by an older `init` keep working.
129. As the human, I want startup, including a migration, to finish within about 10 seconds, so that Claude Code's reconnect window isn't missed.

### Hand repair

130. As the human, I want the store to be plain YAML and Markdown that I can read, grep and fix with an editor while the service is stopped, so that I'm never locked out of my data.
131. As the human, I want a file that fails strict validation to stop startup and name the file and field, so that a bad edit is never silently skipped.

## Implementation Decisions

### Shape of the system

- **One service, the only writer** ([ADR 0002](../../docs/adr/0002-the-service-is-the-only-writer.md)). One long-running process serves MCP, the HTTP API and the embedded UI on one port, fixed at 7733. It takes an exclusive lock on the store at startup. Every `code-central` command, including `init` and Import, is an HTTP client of the service and never opens the store. The CLI never starts the service.
- **Binding.** By default it binds `127.0.0.1` and `::1`. `--bind` adds one address. It never binds `0.0.0.0`.
- **One middleware in front of MCP and the HTTP API**, with the MCP library's own checks turned off:
  - `Host` must be `localhost`, `127.0.0.1`, `[::1]`, `host.docker.internal` or a `--bind` address.
  - A missing `Origin` is allowed. One that is present must be from those hosts.
  - The service sends no CORS headers.
  - `GET` never writes.
- **MCP is served statelessly** over Streamable HTTP at `/p/<key>/mcp`, using one of the maintained Go MCP libraries. All state lives in the store, so a restart loses nothing except the in-memory last-activity times.
- **Go structure: one module.**
  - `core` is the one deep module. It holds every domain rule behind one list of named operations, and takes a clock and an ID source as inputs.
  - `store` is the persistence interface, with a file-backed implementation and an in-memory fake.
  - `events` carries change notices to SSE, the long-poll and `notify`.
  - `mcp` and `httpapi` are thin translators that share the middleware and one `ServeMux`.
  - `client` is the Go HTTP client used by every command. It tolerates unknown response fields.
  - `notify` sends OS Notifications.
  - `codecentraltest` starts a real test server.
  - `web` holds the embedded UI.
  - The entry point wires these together.
  - There is no `integration` package.
- **Web stack.** React and TypeScript, built by Vite as a static single-page app, embedded with `go:embed`. Built files aren't committed. A small committed placeholder page lets `go build` and Go tests run without Node. `make build` runs Vite, then `go build`, and stamps `git describe --tags --always --dirty` as the version.
- **Live updates** use SSE at `/api/events`, with three event types: `ticket.changed {project, id}`, `spec.changed {project, id}` and `project.changed {project}`. A background ticker also emits `ticket.changed` when a Claim turns Silent. There is no event replay: clients refetch.

### The workflow ([ADR 0001](../../docs/adr/0001-one-state-value-on-tickets.md))

There is one fixed workflow of nine States, the same for every Project, which agents can read but not change:

| State | Meaning |
| --- | --- |
| `Draft` | Still being defined, so no agent should pick it up |
| `Ready` | An agent can claim it |
| `In Progress` | An agent has claimed it and is working |
| `Needs Answer` | The agent is stopped on an unanswered Question |
| `Answered` | Every Question is answered and the asking session has not yet picked the work up |
| `Failed` | The agent stopped because something broke |
| `In Review` | The work is finished and waiting for the human to accept it |
| `Done` | Finished |
| `Cancelled` | Abandoned |

| From | To | Who | Action |
| --- | --- | --- | --- |
| `Draft` | `Ready` | Human or agent | `approve` |
| `Ready` | `Draft` | Human or agent | `defer(reason)` |
| `Ready` | `In Progress` | Agent | `claim` |
| `In Progress` | `Ready` | Agent | `give_up(note)` |
| `In Progress`, `Answered` | `Ready` | Human | `release(note?)` |
| `In Progress` | `Needs Answer` | Agent (holder) | `post_round` |
| `Needs Answer` | `Answered` | Automatic | Last open Question answered or dismissed |
| `Answered` | `In Progress` | Agent (holder) | `pick_up_answers` |
| `In Progress` | `Failed` | Agent (holder) | `report_failure` |
| `Failed` | `Ready` | Human | `send_back(note?)` |
| `Failed` | `Draft` | Human | `to_draft` |
| `In Progress` | `In Review` | Agent (holder) | `finish(summary)` |
| `In Review` | `Done` | Human | `accept` |
| `In Review` | `Ready` | Human | `reject(comment)` |
| `Draft`, `Ready` | `Cancelled` | Agent | `cancel(replacements, reason?)`, where Replacements are required |
| Any unfinished | `Cancelled` | Human | `cancel(replacements?)` |
| Any | Any except `Failed` | Human | `move(to_state)` |

- `Needs Answer` and `Failed` can only be entered by posting the Round or the Failure Report.
- Every Ticket passes through `In Review` in v1.
- A rejected action returns `action_not_allowed` with the allowed actions by name.
- The published workflow (`get_workflow`, `GET /api/workflow`, `code-central workflow`) is generated from the same table `core` enforces, plus prose covering:
  - the three exits on trouble: a Question if the human can unblock this session, `Failed` if nothing can, a clean give-up if nothing is wrong but this session can't finish
  - that a send-back with no note means nothing has changed
  - the branch rule: work on a branch named after the Ticket ID, committed and pushed often (a convention that is never checked)
  - the limits of holding several Claims
- **Spec State** is never stored. It is derived as a count per State, or `Draft` when the Spec has no Tickets. Any work on a Spec, including grilling, is a Ticket on it.
- **Blocking** is computed. A Blocker is a many-to-many link between Tickets within one Project. Only `Done` clears a Blocker. Links that would form a cycle are rejected with `cycle`. A Cancelled Blocker is followed through its Replacements when read. A Cancelled Blocker with no Replacements keeps its dependants blocked and puts them in Needs Me.
- **Actor** is `human`, `agent` or `integration`, read from a request header on the HTTP API and defaulting to `agent`. Only the UI sends `human`. MCP and the CLI always write as `agent`. The Actor guards against mistakes, not misuse.

### Claims

- `claim(spec?, ticket?, session_id?)` checks and moves a Ticket under the store's write lock, so the claim is atomic.
  - The claim order is `created`, then number. There is no priority field.
  - Passing both `spec` and `ticket` is an error.
  - When nothing can be claimed it returns `{ticket: null}`. Otherwise it returns the Claim ID with the full Ticket view.
- **A Claim spans** `In Progress`, `Needs Answer` and `Answered`. Any move out of those States ends it permanently.
- **Claim IDs** are 128 random bits, base32-encoded, stored in the Ticket file and left out of every read. Only actions on the Claim need one. A wrong Claim ID gets `stale_claim`.
- **The session ID** is optional and never checked. It is recorded at the claim and with each Round, can be updated with `set_session`, and is used only to show `claude --resume <id>`.
- **Silent Claims** are computed: an `In Progress` Ticket with no activity for 30 minutes.
  - Activity is any write to the Ticket, or a ping.
  - `POST /api/ping {session_id}` marks every live Claim with that session ID, always returns `204`, and is throttled by the server to once per 60 seconds per session.
  - Pings update memory only. After a restart, last activity counts from service start.

### Rounds, Questions and Failure Reports

- A **Round** is posted in one write, numbered and stored on the Ticket. Each **Question** has a title, a body and an optional recommended answer, and is open, answered or dismissed. Answers are free text, editable until the Round is complete.
- Human actions on Rounds are `answer(question, text)`, `accept_recommended(round)` and `dismiss_question(question)`.
- Moving a Ticket out of `Needs Answer` by hand dismisses its open Questions.
- Questions sit only on Tickets. The roll-up of a Spec's Questions is HTTP-only, for the UI.
- **Waiting.** `GET /api/p/<key>/tickets/<n>/wait?from=<State>` holds the request for up to 60 seconds and returns at once if the Ticket has already left that State. `code-central wait` loops on it, and exits `0` (Answered), `3` (left for another State, printing the State and the last move) or `1` (usage error). It needs no Claim ID.
- A **Failure Report** has four required Markdown fields: `what_broke` (headline plus symptom), `why`, `what_to_fix` and `where_work_stands`. Reports are numbered and permanent. Each keeps the session ID. Posting one ends the Claim. There are no categories or attachments.
- **Required notes.** `give_up`, `finish`, `defer` and `reject` require a note, stored as a comment. `send_back`, `release` and `cancel` take an optional one.
- **Next steps.** `post_round` returns "End your turn now. If you can run background commands, run `code-central wait <id>` in the background first." `report_failure`, `give_up` and `finish` return "stop working this Ticket".

### Operations and front doors

- `core` defines one list of named operations. MCP exposes the agent operations as snake_case tools. The CLI exposes the same set as kebab-case commands with JSON output, and adds `init`, `serve`, `service`, `wait`, `ping`, `workflow`, `hook`, `import-guide` and `help`. The HTTP API exposes everything, including the human's actions.
- **Agent operations:**
  - `claim`, `post_round`, `pick_up_answers`, `report_failure`, `give_up`, `finish`, `set_session`, `approve`, `defer`, `cancel`
  - `create_spec`, `edit_spec`, `create_ticket`, `edit_ticket`
  - `add_labels`, `remove_labels`, `set_tags`, `remove_tags`, `add_blocker`, `remove_blocker`, `comment`
  - `find_tickets`, `get_ticket`, `find_specs`, `get_spec`, `get_workflow`, `get_import_guide`, `import`
- **Human-only operations (HTTP only):**
  - `answer`, `accept_recommended`, `dismiss_question`
  - `release`, `accept`, `reject`, `send_back`, `to_draft`, `move`
  - `rename_project`, `archive_project`, `unarchive_project`, `create_project`, `unlink`
  - reads across every Project
- **Integration-only operations:** create linked items, update linked definitions, set `external_state`, and look up by Link. Exact signatures wait for the first Integration.
- **HTTP conventions.**
  - `GET` reads resources under `/api/p/<key>/…`.
  - Every write is a `POST` to a named action with a JSON body. There is no `PUT`, `PATCH` or `DELETE`.
  - The unscoped paths are `/api/needs-me` (across Projects, already grouped and ordered by `core`), `/api/projects`, `/api/events`, `/api/ping` and `/api/workflow`.
  - Paths carry no version in v1. The service version goes in a response header.
- **Errors** have one shape everywhere: `{code, message, allowed_actions?, field?}`.
  - The codes are `unknown_project`, `not_found`, `action_not_allowed`, `stale_claim`, `invalid`, `cycle`, and `service_unavailable` (CLI only).
  - Over HTTP they use status 404, 409 or 422. Over MCP they are a tool result with `isError` set and the JSON as text. On the CLI the JSON goes to stderr with exit code `2`.
- **CLI conventions.**
  - The Claim ID is passed only with `--claim`.
  - Long input goes on stdin or through `--file`.
  - The base URL is `CODE_CENTRAL_URL`, defaulting to `http://localhost:7733`.
  - `--session` takes priority over `CLAUDE_CODE_SESSION_ID`.
  - `ping` gives up after 300 ms and always exits `0`.
- **Reads.**
  - `find_tickets` rows hold the ID, title, State, Spec ID, Labels, `blocked` and `updated`, in claim order, with no pagination.
  - `get_ticket` returns the full view. `claim` returns the same view plus the Claim ID.
  - `find_specs` leaves out finished Specs unless `finished: true` is passed. Its rows include the State summary.
- **Compatibility.** Within v1 the API changes additively only. Requests are decoded strictly.

### The store ([ADR 0003](../../docs/adr/0003-the-store-is-yaml-files-loaded-into-memory.md))

- **Layout.** A `store.yaml` holds the `format`. Under `projects/<key>/` sit `project.yaml` (key, display name, created, mode, `archived?`), `specs/<n>.md` (strict YAML frontmatter plus a Markdown body) and `tickets/<n>.yaml`.
- **IDs.** Each Project has one counter shared by Specs and Tickets. It is derived at startup as the highest existing number plus one, and a number is never reused. The display ID is `<key>-<n>`, which is also the branch name.
- **A Ticket file** holds:
  - title, State, Spec, Labels, Tags, `blocked_by`, `replacements`, `link?`, `created`, `updated` and the description
  - typed, numbered lists: `claims`, `rounds`, `failure_reports`, `comments`, `moves` (from, to, Actor, time, Claim number, `via?`) and `edits` (number, Actor, time, `previous` values)
- **A Spec's frontmatter** holds id, title, Labels, Tags, `link?`, created, updated and `edits`.
- **Format.** Timestamps are UTC RFC 3339. Markdown is stored in literal block scalars. Fields are written in a fixed order and decoded strictly.
- **Reads** all come from memory. Every file stays loaded for good.
- **Writes.** One store-wide read-write mutex is held across the check, the write and the memory update. Each operation writes exactly one file (temp file, fsync, rename), and memory is updated only after the file is on disk.
- **Import is the one bulk write.** Files are staged and fsynced, then a manifest is written, then each file is renamed into place, then the manifest is deleted. At startup a leftover manifest is rolled forward, and a staging folder with no manifest is deleted.
- **Startup order:** roll forward any Import manifest, then migrate the format, then load and strictly validate every file (a failure stops startup and names the file and field), then bind the port.
- **Migration.** Chained steps run into a staging folder. The old store is renamed to `code-central.format-<n>.bak` and the staging folder is renamed into place. A failure leaves the original store untouched. Every schema change bumps `format`, even an additive one. A checked-in schema snapshot enforces the bump.
- **Hand edits** are supported only while the service is stopped. There is no file watcher.

### Projects

- A Project stores its Project Key (permanent, unique, lowercase), a display name, its mode and `archived?`. It stores no path and no remote.
- `init` is the only way to add a Project. It needs `GET /api/projects/<key>` and `create_project(key, display_name)`. Agents and the UI can't create Projects.
- The key is proposed from the only remote, else `origin`, else the folder name, as the repository name alone, lowercased.
- `init` writes:
  - `.mcp.json`, with the server `code-central` at `${CODE_CENTRAL_URL:-http://localhost:7733}/p/<key>/mcp`
  - the hooks: a `PreToolUse` hook on `claim` and `post_round` runs `code-central hook session-id` to inject `session_id` through `updatedInput`, and a `PostToolUse` hook on every tool runs `code-central ping`
  - `docs/agents/issue-tracker.md` and `triage-labels.md`, which map skill operations to Code Central operations by name and point at `get_workflow`
  - a pointer to the tracker doc in `CLAUDE.md`, or in `CLAUDE.local.md` with `--local`
- Generated files carry a header comment with the writing version. `AGENTS.md` isn't touched.
- Archiving is human-only and refused while any Ticket is unfinished. An Archived Project refuses every write and allows reads.

### Context budget for teaching agents

These are limits, not targets, and a `core` test fails when the text goes over them:

| Text | Limit |
| --- | --- |
| MCP `instructions` | About 150 words |
| Each tool description | One line |
| `get_workflow` prose | About 1,500 tokens |
| Next step on a write | One sentence |
| Each generated `docs/agents/` file | About 400 tokens |

### Import

- `import(payload, dry_run?)` is in `core`'s operation list. It is exposed over MCP, as `code-central import` and at `POST /api/p/<key>/import`. `get_import_guide` holds the mapping guidance. The service never reads a repository.
- The payload is a whole `.scratch/` folder, with one Spec per feature folder. It is validated strictly: a fault returns `invalid` with a `field` path, and a cycle returns `cycle`.
- The mapping rules are those in user stories 110–112.
- Tag keys are lowercase `[a-z0-9._-]`, and each has one non-empty, single-line value.
- Rejected when any `source` Tag already exists in the Project. There is no merge or `--force`.
- New IDs come from the counter in source order: folder, then file number. References inside bodies aren't rewritten.
- Each imported Ticket gets one `moves` entry with no `from` and `via: import`. Only Import can create a Ticket directly in `Done` or `Cancelled`.
- No Claims are imported. Nothing comes in as `In Progress`, `Needs Answer`, `Answered`, `Failed` or `In Review`.
- A dry run stores no pending plan.

### Integrations ([ADR 0004](../../docs/adr/0004-an-integration-is-a-client-of-the-http-api.md))

- The optional `link` on Specs and Tickets is `{integration, id, url, external_state: open|closed|deleted|out_of_scope, detached}`. An item has at most one link.
- The external system owns the definition (title, description, parent). Code Central owns delivery (State, Claims, Rounds, Failure Reports, Labels, Tags, Blockers).
- The `integration` Actor alone writes definitions on linked items and can't move State.
- "Closed externally" is a computed `needs_me_reason`: the `external_state` isn't `open`, the Link isn't detached, and the Ticket isn't finished.
- `unlink` marks the link `detached`. There is no re-link in v1. External trees flatten to Spec and Ticket.

### Needs Me and Notifications

- `core` computes `needs_me_reason`. The reasons are:
  - `Needs Answer`
  - `Answered`
  - Silent Claim
  - `Failed`
  - `In Review`
  - blocked by a Cancelled Ticket with no Replacements
  - closed externally
- `core` also computes the group order (as listed in user story 79) and longest-waiting first within a group. `/api/needs-me` returns them already ordered.
- `notify` subscribes to `events` and runs a timer for the Silent mark. It notifies only on a new Round, a new Failure Report, or a Claim turning Silent, once per crossing.
  - Notifications are gathered over a 15-second window into one per event or one summary.
  - One summary is sent at startup if Needs Me is non-empty.
  - The sender uses `terminal-notifier` or `osascript` on macOS, and D-Bus or `notify-send` on Linux, with a click-through where available.
  - The setting is `notifications: on|off`, and `serve --no-notify` turns them off for one run.

### UI

- One view ships in v1: the three-pane triage inbox.
- The action panels (Round, Resume-then-Release for both `Answered` and Silent Claim, `Failed`, `In Review`, Blocked, closed externally) and the Ticket detail are components shared by any view. A later focus queue or Project board is then only a layout.
- The view is in the URL (`?view=inbox&ticket=<id>`).
- The UI reaches the HTTP API and SSE only through one TypeScript API module.
- The tab title and favicon show the Needs Me count.
- The throwaway prototype on branch `prototype/attention-ui` is a reference for layout and is not merged.

## Testing Decisions

A good test checks external behaviour at the seam that owns it, and each behaviour is tested once. A fake may be used many times, but a contract suite runs it beside the real implementation it stands in for, so the two can't drift. There is no fake of `core` and no fake of Code Central. A test that sleeps on real time is a bug: the clock is injected everywhere time matters. "100% confidence" means every behaviour is covered at a seam, not a coverage percentage.

- **`core` (the primary seam).** Every domain rule is tested here and nowhere else, through `core`'s operation interface, against the in-memory store with a fake clock and ID source:
  - States and actions, Claims and Claim IDs, Rounds and automatic `Answered`, Failure Reports, Blockers, Replacements and cycles, and Silent Claims (30 minutes, the 60-second ping throttle)
  - the Actor rules, including the `integration` Actor, "closed externally" and `unlink`
  - Archived Project refusals
  - edit history
  - the Import validator and its `field` paths
  - Needs Me reasons and order
  - the context-budget limits on taught text
  - the schema snapshot that forces a `format` bump
- **Store contract suite.** One suite runs against the file-backed store and the in-memory fake: reads after writes, the derived ID counter, Replacements followed on read, and Import's all-or-nothing result.
  - File-only tests: strict decoding that stops startup and names the file and field, refusing an unknown `format`, the exclusive lock, one file per write, fixed field order, and migration through staging with a backup.
  - Import crash recovery uses a test-only "stop after step k" hook. For every k, a test runs a real Import that stops after step k, runs startup recovery, and checks the result is all or nothing. Two named cases sit beside it: an orphan staging folder is deleted, and a partly renamed manifest is rolled forward.
  - File-backed tests use temporary directories.
- **Front doors.** One table-driven suite, built from `core`'s operation list, runs each operation through every door that exposes it, against the real `core` and the in-memory store, in-process:
  - MCP through the Go MCP client over Streamable HTTP
  - the HTTP API directly
  - CLI commands in-process, with `client` pointed at a test server
  - Each operation gets one successful and one rejected call, checking that door's error shape.
  - The suite fails if a door is missing an operation, or exposes a human-only one over MCP or the CLI.
  - It tests translation only, never rules.
  - The shared middleware is unit-tested once: the `Host` and `Origin` allowlist, the Actor header, no CORS headers, and `GET` never writing.
- **CLI-only behaviour**, tested against `codecentraltest` with a fake clock:
  - `wait`'s exit codes and its retries through connection errors
  - `ping` giving up after 300 ms and always exiting `0`
  - the version-mismatch warning
  - how `init` derives the key and writes its files in both modes
  - the `service_unavailable` message
- **`codecentraltest`** runs the real `core`, the in-memory store and a fake clock behind the real `httpapi` on a test port. The `client` package and any future Go Factory or Integration test against it. Clients in other languages run `code-central serve --store <tmpdir>`.
- **`notify`.** The window, what notifies and the wording are tested with a fake clock and a fake sender. The real OS sender has one contract test that skips when no notification daemon is present.
- **UI.**
  - The TypeScript API module has a contract suite (each read, each human action, the error shape, SSE delivery). It runs against the in-memory fake in the fast suite, and against the real module talking to a real `serve` in the end-to-end tier.
  - Each action panel gets component tests once, with Vitest and Testing Library against the fake API.
  - Inbox layout tests use real panels and the fake API, and check only layout: group order as received, Drafts collapsed, State filter with dots, combined filters, view in the URL, and refetch on an SSE notice.
- **End-to-end** (behind an `e2e` build tag, through the real binary). There are exactly three:
  1. The agent loop: `init`, an MCP `claim` and `post_round`, the human answers over HTTP, `wait` exits `0`, then `pick_up_answers`, `finish` and `accept`.
  2. Restart during a `wait`: state survives, and a second `serve` refuses to start.
  3. Playwright against the embedded UI: an agent posts a Round, the inbox updates over SSE, and the human answers.
- **Performance tests.** A startup test loads a generated store of about 5,000 Tickets. An end-to-end test migrates and loads 10,000 Tickets with long histories and fails over 10 seconds.
- **Budgets.** `make test` runs `go test ./...` and `vitest run` in parallel against the placeholder page, and fails if it takes more than 10 seconds. `make e2e` builds everything and runs the end-to-end tier.
- **Manual checklist**, run when v1 is built: Docker Desktop for Mac, Linux Docker with `--bind`, `sbx`, `service install`, and real Claude Code (hooks, `updatedInput`, `CLAUDE_CODE_SESSION_ID`).
- **Prior art.** None yet: the repository has no code. The suites above are the first.

## Out of Scope

- Factory registration, check-ins and feedback in the UI. v1 must not block them.
- Starting, watching or stopping agents.
- Any active Integration, including GitHub Issues and Azure DevOps. Jira and Trello come later still. Only the contract above ships.
- A required Handoff note with each Round.
- MCP tasks as a wake mechanism.
- Shared use of a repository by other people (`init --local` covers repositories that can't take committed files).
- Multi-user use, authentication and access from other machines. Any local process can act as the human in v1, which is an accepted risk.
- Packaging and releasing the binary. v1 is built from source with `make build`.
- The focus queue and the Project × State board views. v1 must not block them.
- Deleting anything, a Spec browser, search across finished work, and a restore action for edits.
- Human actions from the CLI, per-reason Notification switches, quiet hours, and holding back Notifications while a tab is visible.
- Detecting stale generated files in repositories.

## Further Notes

- **Hard-to-reverse choices are in the ADRs:** one State value ([0001](../../docs/adr/0001-one-state-value-on-tickets.md)), the service as the only writer ([0002](../../docs/adr/0002-the-service-is-the-only-writer.md)), YAML loaded into memory ([0003](../../docs/adr/0003-the-store-is-yaml-files-loaded-into-memory.md)) and Integrations as HTTP clients ([0004](../../docs/adr/0004-an-integration-is-a-client-of-the-http-api.md)).
- **Unverified assumptions** to check while building:
  - `updatedInput` on MCP tools, and `CLAUDE_CODE_SESSION_ID` in Bash subprocesses
  - whether a container on Docker Desktop for Mac reaches a loopback-only service, and whether the `sbx` proxy rewrites `Host`
  - waits of hours or days, laptop sleep and reboots during a `wait`
  - whether Claude Code defers Code Central's tools through tool search
  - how long a real migration takes
- **Long-term direction** this must not block: the human defines work in the UI, background Factories pick it up and grill it, Questions come back to the UI, Factories deliver, and failures show what broke, why and what to fix.
- **This repository** stays on local Markdown until Code Central v1 can bring it in through the Scratch Import.
- The decision detail behind each section is in the resolved tickets linked from [map.md](map.md).
