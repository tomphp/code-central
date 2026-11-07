# The MCP and HTTP surface

Type: grilling
Status: resolved
Blocked by: 03, 05, 06, 07, 08, 09

## Question

What are the exact MCP tools and HTTP endpoints, covering finding Tickets by State, title, Label and Spec, fetching details, updating Labels, and posting Questions and comments? Can agents create Specs and Tickets as well as update them? Does a shell-callable command line sit alongside them, given what Sandcastle needs from a ticket source? How do these carry what The State workflow decided: serving the workflow itself, declaring the actor on every write, and returning the allowed moves when a move is rejected? How do they carry what How agents ask and the human answers decided: posting a Round in one write with a session ID, the atomic pick-up from `Answered`, and the long-poll behind `code-central wait`? How do they carry what How a Project is identified decided: MCP served at `/p/<key>/mcp` with no Project argument on any tool, an error for an unknown key, the order shell commands look for the key, and `code-central init`? How are lookups that cross Projects handled, given that a connection belongs to one Project?

How do they carry what [How agents claim work](09-claiming-work.md) decided: claiming the next Ticket, narrowed by a Spec or a Ticket, with an empty result when nothing can be claimed; returning the Claim ID and past Claims from the claim; the Claim ID on every write that needs one and never on reads; updating the session ID; and the ping, by session ID from a hook or by a `code-central` command?

[Process shape and web stack](10-process-and-stack.md) decided that every `code-central` command, including `init`, is a client of the HTTP API through a shared Go `client` package and never touches the store, so the HTTP API must cover everything the CLI does. MCP is served statelessly. `GET` requests never write. `code-central wait` retries through connection errors. SSE events only name what changed, such as a Ticket ID and a type, so which event types exist?

## Answer

Amended by [How Code Central is upgraded](21-upgrades.md), which made the API additive-only within v1, made the `client` tolerate unknown response fields, and made the version warning name the older side when it can.

Amended by [How finished work is kept and hidden](20-finished-work.md), which added `archive_project`, `unarchive_project` and `find_specs(finished?)`, and made every write to an Archived Project refused.

Amended by [How the Scratch Import behaves](13-scratch-import.md), which added `import` and `get_import_guide`, Labels on Specs, `set_tags`, `remove_tags`, and `tags` filters on `find_tickets` and `find_specs`.

Amended by [The Integration plugin contract](14-integration-contract.md), which added the `integration` Actor, a `link` field on Specs and Tickets, writes only an Integration can make (create linked items, update their definition, set `external_state`), lookup by link, the human-only `unlink` action, and the "closed externally" `needs_me_reason`. Exact signatures wait for the first Integration.

`core` defines one list of named operations, and each front door exposes part of it under the same names: snake_case MCP tools, the same names in kebab-case as `code-central` commands, and snake_case action paths on the HTTP API.

- **MCP** exposes only what agents can do. Every write is made as `agent`.
- **The CLI** matches the MCP tools one for one with JSON output, which covers what Sandcastle needs. It adds `init`, `serve`, `service`, `wait`, `ping` and `workflow`. Every write is made as `agent`.
- **The HTTP API** exposes everything: the agent operations, the human's actions, reads that span Projects, and what `init` needs. `GET` is used for resources (`GET /api/p/<key>/tickets?…`, `GET /api/p/<key>/tickets/<n>`) and never writes. Every write is a `POST` to a named action (`POST /api/p/<key>/tickets/<n>/finish`, `POST /api/p/<key>/claim`) with a JSON body. There is no `PUT`, `PATCH` or `DELETE`.

**Actor.** The HTTP API reads the Actor from a header (such as `X-Code-Central-Actor: human`), defaulting to `agent`. Only the UI sends `human`. The human's actions exist only in the UI in v1. Human CLI commands can be added later without changing stored data.

**Agent actions.** State changes are named actions rather than a generic move, so every move that needs content has a typed shape:

| Action | Claim ID | Does |
| --- | --- | --- |
| `claim(spec?, ticket?, session_id?)` | Returns one | `Ready` to `In Progress`. Returns `{claim_id, ticket: <full view>}`, or `{ticket: null}` when nothing can be claimed. Passing both `spec` and `ticket` is an error. A named Ticket that can't be claimed is rejected with the reason (its State or its Blockers) |
| `post_round(ticket, claim_id, session_id?, questions: [{title, body, recommended?}])` | Yes | Moves to `Needs Answer` and returns the Round number |
| `pick_up_answers(ticket, claim_id)` | Yes | `Answered` to `In Progress` in one step. Returns the latest Round with every answer and dismissal, plus any comments added since it was posted |
| `report_failure(ticket, claim_id, what_broke, why, what_to_fix, where_work_stands)` | Yes | Moves to `Failed` and ends the Claim |
| `give_up(ticket, claim_id, note)` | Yes | `In Progress` to `Ready`. The note on where the work stands is required and stored as a comment |
| `finish(ticket, claim_id, summary)` | Yes | Moves to `In Review`. A summary is required, stored as a comment: what was done, where (branch or commit), and what the reviewer should check |
| `set_session(ticket, claim_id, session_id)` | Yes | Updates the session ID |
| `approve(ticket)` | No | `Draft` to `Ready` |
| `defer(ticket, reason)` | No | `Ready` to `Draft`. The reason is required and stored as a comment |
| `cancel(ticket, replacements, reason?)` | No | Cancels a `Draft` or `Ready` Ticket by naming its Replacements. The optional reason is stored as a comment |

A Claim ID is needed only for actions on the Claim, and never on reads. Anyone can comment on, label and link a Ticket, even one someone else holds. A rejected action returns the actions allowed from the Ticket's current State, by name.

**Writes say what to do next.** `post_round` returns: "End your turn now. If you can run background commands, run `code-central wait <id>` in the background first." `report_failure`, `give_up` and `finish` return "stop working this Ticket". That way the instruction arrives at the moment it matters, not only in setup docs.

**Editing.** Agents can create and edit, but nobody can delete anything in v1. Cancelling takes the place of deleting.

- `create_spec(title, body)` and `edit_spec(spec, title?, body?)`, where the body is replaced whole.
- `create_ticket(spec, title, description, state = Draft | Ready, labels?, blocked_by?)` and `edit_ticket(ticket, title?, description?)`.
- `add_labels` and `remove_labels`, rather than setting the whole list, so two writers don't overwrite each other.
- `add_blocker` and `remove_blocker`. A link that would form a cycle is rejected.
- `comment(ticket, body)`. Comments sit only on Tickets.

**Reads.**

- **`find_tickets(state?, title?, labels?, spec?, claimable?)`:** every filter given must match. The title match is a case-insensitive substring, and Labels must all be present. `Done` and `Cancelled` are left out unless the State filter names them. Each row holds ID, title, State, Spec ID, Labels, `blocked` and `updated`, in claim order (`created`, then number). There is no pagination in v1.
- **`get_ticket(ticket)`:** one full view, the same everywhere.
  - The stored fields, the Spec's ID and title, each Blocker with its State (after following Replacements), and Replacements.
  - The typed lists as stored: `claims` (with no Claim IDs), `rounds`, `failure_reports`, `comments` and `moves`. Clients sort them by time if they want a timeline.
  - Computed fields: `blocked`, `claimable`, `silent_since` and `needs_me_reason`.

  `claim` returns this same view plus the Claim ID.
- **`get_spec(spec)`:** the frontmatter, the Markdown body, a derived State summary (a count per State, or `Draft` when the Spec has no Tickets) and its Tickets as `find_tickets` rows. **`find_specs(title?)`** finds Specs. The roll-up of a Spec's Questions is HTTP only, for the UI.
- **`get_workflow()`:** the published workflow as structured data plus prose.
  - States, meanings, actions, who can take each, required content and what counts as Needs Me.
  - The three exits on trouble, the branch rule, and the limits of holding several Claims.
  - That a send-back with no note means nothing has changed.

  The MCP server's `instructions` say briefly what Code Central is and to call `get_workflow` before working a Ticket. It is a tool rather than a resource, because Claude Code only brings resources into a session when they are @-mentioned. It is also served as `GET /api/workflow` and `code-central workflow`.

**The human's actions (HTTP only).**

- Everything agents can do.
- `answer(question, text)`, which can be edited until the Round is complete, and `accept_recommended(round)`.
- `dismiss_question(question)`, which dismisses one Question inside a Round. This adds to How agents ask and the human answers, where dismissal happened only by moving the Ticket. When the last open Question is answered or dismissed, the Ticket moves to `Answered`.
- `release(note?)`, `accept`, `reject(comment)` with the comment required, `send_back(note?)`, `to_draft` and `cancel(replacements?)`.
- `move(to_state)`, to any State except `Failed`.
- `rename_project`.
- For `init`: `GET /api/projects/<key>` and `create_project(key, display_name)`, with the display name defaulting to the key.

**Projects.** Everything an agent touches is scoped to its connection's Project, at `/p/<key>/mcp` or `/api/p/<key>/…`. MCP and the CLI accept `<key>-<n>` or a bare `<n>`. An ID with another Project's key is rejected with an error naming the connection's Project. Only these HTTP paths are unscoped:

- `/api/needs-me`, across every Project
- `/api/projects`
- `/api/events`
- `/api/ping`

For an unknown Project Key, the MCP endpoint still accepts the connection, and every tool call then returns `unknown_project`, naming the key and telling the agent to ask the human to run `code-central init`. Rejecting the connection outright would only show as "server failed" in `/mcp`, which no agent reads.

**Waiting.**

- `GET /api/p/<key>/tickets/<n>/wait?from=Needs%20Answer` holds the request for up to 60 seconds and returns the Ticket's State. It returns at once if the Ticket has already left that State.
- `code-central wait <id>` loops on it and retries through connection errors.
- **Exit `0`:** the Ticket is `Answered`. It prints JSON saying to call `pick_up_answers`.
- **Exit `3`:** the Ticket left for any other State, so the Claim has probably ended. It prints the State and the last move.
- **Exit `1`:** a usage error.

It takes no Claim ID, because it is a read. A session finds out whether it still holds the Claim when it calls `pick_up_answers`.

**Session ID and ping.**

- **Hook injection:** `claim` and `post_round` take an optional `session_id`, which the hook fills in through `updatedInput`.
- **The ping:** `POST /api/ping {session_id}` marks activity on every live Claim with that session ID, in any Project. It always returns `204`, even for an unknown ID. The server ignores pings for a session within 60 seconds of its last one, so the hook needs no state file.
- **The ping command:** `code-central ping` reads the hook's JSON from stdin, or takes `--session <id>`. It gives up after 300 ms and always exits `0`.

**SSE.** `GET /api/events` sends three event types:

- `ticket.changed {project, id}`
- `spec.changed {project, id}`
- `project.changed {project}`

A background ticker also sends `ticket.changed` when a Claim turns Silent, because that changes a computed value with no write behind it.

**Errors.** One shape everywhere: `{code, message, allowed_actions?, field?}`.

- **Codes:** `unknown_project`, `not_found`, `action_not_allowed` (with `allowed_actions`), `stale_claim`, `invalid` (with `field`), `cycle`, and `service_unavailable` (CLI only).
- **HTTP:** status 404, 409 or 422.
- **MCP:** a tool result with `isError` set and this JSON as text.
- **CLI:** the JSON on stderr, with exit code `2`.

**CLI conventions.**

- JSON on stdout for every operation command. `init`, `serve` and `service` print text.
- The Claim ID is passed only with `--claim <id>`, never an environment variable, because one session can hold several Claims.
- Long input (the Questions of a Round, a Failure Report's fields) is taken as JSON on stdin or through `--file`.
- The base URL is `CODE_CENTRAL_URL`, defaulting to `http://localhost:7733`.

**Versioning.** Paths carry no version in v1. The service sends its version in a response header, and the CLI prints a warning on stderr, without failing, when its own version differs, because a CLI inside a sandbox can be an older copy. How upgrades are handled stays with the distribution fog.

Considered and rejected:

- **Designing each front door separately:** the three would drift apart.
- **The human's actions on MCP:** an agent could act as the human by mistake.
- **A generic `move(to_state)` for agents:** it hides the content each move requires.
- **Requiring a Claim ID on every write to a claimed Ticket:** a triage agent couldn't comment on or label a Ticket someone else holds.
- **Setting Labels as a whole list:** two writers would overwrite each other.
- **Strict REST with `PATCH` on State:** it hides the actions and their required content.
- **Publishing the workflow as an MCP resource:** agents don't reliably read resources.
- **Rejecting the MCP connection for an unknown key:** agents never see why it failed.
- **Throttling the ping in the hook:** it would need a state file per session.
- **A Claim ID in an environment variable:** one session can hold several Claims.
- **Version prefixes on paths:** they're premature while upgrades are still in the fog.
