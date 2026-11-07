# Code Central v1

Label: wayfinder:map

## Destination

A v1 spec, plus ADRs for the hard-to-reverse choices, for a single-user local coordination point: Projects, Specs and Tickets; a UI that surfaces what needs the human; an MCP server; a one-way Scratch Import; and an Integration plugin interface with no active implementations. The map is done when that spec can be sliced into implementation tickets.

## Notes

Domain: use the vocabulary in `GLOSSARY.md`. Grilling sessions also call domain-modeling.

When the map clears, hand off to `/to-spec`.

Long-term vision, as context v1 must not block: the human defines work in the UI; background Factories pick it up and grill it; questions come back to the UI; the human answers; Factories deliver the work; failures appear in the UI with what broke, why, and what to fix.

Settled while charting:

- In v1, Code Central is a coordination point: a ticket management system that does not start or control agents.
- A Project is one repository. Every Ticket belongs to exactly one Spec; a small job is a one-Ticket Spec.
- State is a single workflow value on a Ticket; Labels are free-form and separate.
- Code Central's local store is authoritative for imported work. All data lives in one central store on the machine, not in each repository: TOML or YAML with Markdown alongside.
- Import (one-way Scratch migration) and Integration (ongoing plugin link) are separate concepts.
- One core with two front doors: MCP for agents, an HTTP API for the web UI and other processes.
- One user, one machine, no authentication.
- Go backend, TypeScript UI, all web artifacts in a single binary, live updates over SSE or WebSockets.

Testing standing preference: confidence comes from testing at multiple layers. A mock may be used many times, but the real behaviour it stands in for is tested once elsewhere. Contract tests at each seam run against both the real implementation and its fake, so fakes cannot drift; a very small number of end-to-end tests run through the real binary. Everything except end-to-end tests finishes in under 10 seconds. "100% confidence" means every behaviour is covered at a seam, not a coverage percentage.

## Decisions so far

<!-- the index: one line per closed ticket, enough to judge relevance, then zoom the link for the detail the ticket holds -->

- [What Sandcastle needs from a ticket source](issues/01-sandcastle-needs.md): a library that runs one agent, not a scheduler; a Factory built on it needs shell-callable operations with JSON output, a configurable base URL, and an atomic claim.
- [Serving MCP from a Go binary](issues/03-go-mcp.md): both Go libraries are viable and current, MCP and the HTTP API can share one port, and Docker access raises unresolved `Host`, `Origin` and bind-address issues.
- [How external trackers model work](issues/04-external-tracker-models.md): Spec-owns-Tickets fits all three in shape, but breaks on optional parents, deep trees and non-matching workflows; polling is the only path from a laptop that works on all three.
- [The Scratch format in the wild](issues/02-scratch-format.md): no schema or version marker, and real folders depart from the template in almost every field; State is often stale, so an Import can't take it at face value.
- [The State workflow](issues/05-state-workflow.md): one fixed workflow, now nine States, that agents read over MCP, with `Needs Answer`, `Answered`, `Failed` and `In Review` as what needs the human; Spec State is derived, and blocking is computed.
- [Keeping an agent session going across Questions](issues/18-session-continuity.md): an MCP task wakes a live interactive session and `claude --resume` covers everything else, including a Factory; both need the claim to stay with the asking session, so `Needs Answer` can no longer return to `Ready`.
- [How agents ask and the human answers](issues/06-question-model.md): Questions are blocking records on Tickets, posted in Rounds; the asking session posts, ends its turn and continues when woken, holding its claim through a new `Answered` State; a lost session is released to a new agent working from the Ticket.
- [What a failure report contains](issues/07-failure-reports.md): four required fields, numbered and permanent, posted only by the claim holder when it has no way forward; `Failed` ends the claim, the human sends it back, to `Draft` or cancels, and claiming returns the full history.
- [How a Project is identified](issues/08-project-identity.md): a permanent Project Key, derived once by `code-central init` from the remote or folder and overridable, committed in the `.mcp.json` URL; connections belong to one Project, and `init` is the only way to add one.
- [How agents claim work](issues/09-claiming-work.md): an atomic claim of the next claimable Ticket, proved by a Claim ID on each write; only the human releases a Claim, helped by a computed Silent Claim warning after 30 minutes, with Resume offered before Release.
- [Process shape and web stack](issues/10-process-and-stack.md): one service, run by hand or as a login item, that is the only writer to the store and that every command calls over HTTP; it binds loopback on fixed port 7733 with a `Host` and `Origin` allowlist, serves MCP statelessly and pushes SSE to a React, TypeScript and Vite UI; one Go module with a deep `core`.
- [The store's file layout](issues/11-store-format.md): strict YAML files per Ticket (Markdown per Spec) under `projects/<key>/` in `~/.local/share/code-central`, loaded whole into memory; one file per write, edits only while stopped, and Ticket IDs `<key>-<n>` from one counter per Project.
- [The MCP and HTTP surface](issues/12-api-surface.md): one list of named operations, exposed as MCP tools and matching CLI commands for agents and as an HTTP API that adds the human's actions; only actions on a Claim need its Claim ID, writes return the next step, and one error shape lists the allowed actions.
- [How the Scratch Import behaves](issues/13-scratch-import.md): an agent maps the whole `.scratch/` folder into one strictly validated `import` payload, reviewed as a dry-run plan first; `ready-for-agent` becomes `Ready`, header fields become Labels and the new key/value Tags, a `source` Tag blocks re-import, and a roll-forward manifest makes the write atomic.
- [The Integration plugin contract](issues/14-integration-contract.md): an Integration is a separate HTTP client with its own polling, configuration and credentials; it owns the definition of a linked item and Code Central owns its delivery, through a typed `link` and an `integration` Actor; external closes show as a computed "closed externally" in Needs Me rather than moving State.
- [The attention-first UI](issues/15-attention-ui.md): v1 is one three-pane triage inbox, with Needs Me grouped by reason (Questions first) and one action panel per reason shared across views, so a focus queue and a Project board can be added later as layouts only.
- [The testing strategy by seam](issues/16-testing-strategy.md): each rule is tested once in `core`; one suite built from the operation list checks translation through MCP, HTTP and the CLI; contract suites keep the in-memory store and the UI's fake API honest; `codecentraltest` gives HTTP clients the real stack; `make test` fails over 10 seconds, and three end-to-end tests run separately.
- [How agent sessions are taught to use Code Central](issues/17-teaching-agent-sessions.md): the server teaches through compact MCP `instructions` (including the rule that a held Claim sends every question to a Round) and `get_workflow`, within a tested context budget; `init` writes only pointers (`.mcp.json`, hooks, the tracker docs), either committed or with `--local` kept out of git, and refreshes them when run again.
- [How attention reaches the human outside the open UI](issues/19-attention-outside-ui.md): the service sends OS Notifications for new Rounds, Failure Reports and newly Silent Claims, gathered over a 15-second window, with a click-through where the platform makes it cheap and a setting to turn them off; the tab shows the Needs Me count in its title and favicon, and the rest of Needs Me only counts.
- [How finished work is kept and hidden](issues/20-finished-work.md): nothing is removed and finished is computed from State; only a Project can be archived, by the human and only once all its work is finished, after which it refuses writes; `find_specs` hides finished Specs, and every edit records its Actor and the previous text.
- [How Code Central is upgraded](issues/21-upgrades.md): `serve` migrates the store automatically through a staging folder, keeping a backup per old format, within a tested 10-second startup budget; every schema change bumps `format`, the API changes additively only within v1, and the `.mcp.json` URL, hook subcommands and Import manifest are a permanent contract.

## Not yet specified

Nothing. The map is clear: hand off to `/to-spec`.

## Out of scope

- Factory registration, check-ins and feedback in the UI. Wanted later, so v1 must not block it; how a Factory registers is undecided, and it may be built in this project, as a separate process, or from existing technology.
- Starting, watching or stopping agents in v1.
- Any active Integration implementation.
- A required Handoff note with each Round in v1. It costs tokens that aren't needed yet ([How agents ask and the human answers](issues/06-question-model.md)).
- MCP tasks as a wake mechanism in v1. They are a flag-controlled beta that would be hand-written in Go ([How agents ask and the human answers](issues/06-question-model.md)).
- Shared use of a repository by other people. Wanted later, so v1 must not block it. A repository that can't take committed files is covered in v1 by `init --local` ([How agent sessions are taught to use Code Central](issues/17-teaching-agent-sessions.md)).
- Multi-user use, authentication, and access from other machines. This includes proving the human's actions: in v1 any local process, including a sandboxed agent that can reach localhost, can act as the human. That risk is accepted for v1, and authentication is wanted later, so v1 must not block it.
- Packaging and releasing the binary. v1 is built from source with `make build`.
- A focus queue view and a Project × State board view. Wanted later, so v1 must not block them: views share the action panels and the view is in the URL ([The attention-first UI](issues/15-attention-ui.md)).
- Jira and Trello Integrations, including whether Atlassian's API token policy allows one. They're months away, after GitHub Issues and Azure DevOps ([The Integration plugin contract](issues/14-integration-contract.md)).
