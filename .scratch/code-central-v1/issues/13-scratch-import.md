# How the Scratch Import behaves

Status: resolved
Type: grilling
Blocked by: 02, 05, 08, 11

## Question

How does each part of a Scratch folder map to Specs and Tickets, including wayfinder maps? How is an Import triggered, what happens if it runs twice or hits something it cannot map, and what happens to the `.scratch/` folder afterwards? How a Project is identified decided that an Import takes its folder as an argument and that only `code-central init` adds a Project, so must `init` run first, and how does an Import name its target Project?

[Process shape and web stack](10-process-and-stack.md) made the service the only writer, so an Import runs through the HTTP API and needs the service running. Does the CLI parse the folder and send the result, or does the service read the folder itself?

[The store's file layout](11-store-format.md) makes every write touch exactly one file, and Import is the one exception: it has to stage its files and swap them into place in one step. How does it do that, and what does it do with the IDs it assigns?

[The MCP and HTTP surface](12-api-surface.md) didn't define Import. It has to fit that surface: a named `POST` action on the HTTP API, `invalid` errors that name a `field`, and no Actor other than `agent` from the CLI. Is Import an operation in `core`'s list, and does MCP expose it?

## Answer

An agent maps a `.scratch/` folder, and the service checks and writes the result. `core` gains one bulk operation, `import`, which takes a strictly validated payload. The agent builds that payload by reading the folder, following guidance the server publishes. This leaves the messy variants the Scratch research found to the agent's judgement, and the only code we keep is a strict validator.

**Exposure.** `import(payload, dry_run?)` sits in `core`'s operation list, exposed as:

- an MCP tool
- `code-central import`, with the payload on stdin or through `--file`
- `POST /api/p/<key>/import`

The guidance is a separate read, `get_import_guide` (also `code-central import-guide`), so sessions don't pay for it in `get_workflow`. The MCP `instructions` gain one line: "To import a `.scratch/` folder, call `get_import_guide`." The service never reads a repository, and Import writes as `agent`, like every CLI and MCP write.

**The plan.** A `dry_run` runs every check, including the run-twice check, and writes nothing. It returns the plan: each source path with its provisional new ID, title, State, Labels, Tags and Blockers, grouped by Spec, with a count per State. IDs are provisional, because something could be created in between. The guidance tells the agent to present the plan, list its doubts, and wait for the human's go-ahead before calling again without `dry_run`. No pending plan is stored.

**Target Project.** `code-central init` must have run first. Import finds the Project the same way as every other command, and an unknown key returns `unknown_project`. Import never creates a Project.

**The unit.** One Import takes a whole `.scratch/` folder, with each feature folder becoming one Spec, all or nothing, so Blockers across folders and Ticket numbers that run across the repository still resolve.

**Specs.**

- A folder's `spec.md` becomes the Spec body.
- A wayfinder `map.md` becomes the body verbatim, and each decision ticket becomes a Ticket on that Spec.
- A folder with both gives one Spec, `spec.md` followed by `map.md`.
- A folder with neither gets a Spec titled after the folder, with an empty body.

**States.**

| Scratch | State |
| --- | --- |
| `resolved`, `done`, `closed` | `Done` |
| `ready-for-agent` | `Ready` |
| `Superseded by N`, `Broken into N, M` | `Cancelled`, with Replacements |
| `wontfix` | `Cancelled`, no Replacements, the reason as a comment |
| `Deferred` | `Draft`, the reason as a comment |
| Anything else, or missing | `Draft` |

No Claims are imported, and nothing comes in as `In Progress`, `Needs Answer`, `Answered`, `Failed` or `In Review`. States are often stale, so the agent flags in the plan any Ticket whose State disagrees with its body: a `Ready` Ticket with every box ticked or an answer present, or a `resolved` one with every box unticked. The human picks the State for each one before the real Import.

**Labels and Tags.** This decision adds Tags, free-form key/value pairs, and gives Specs Labels and Tags as well as Tickets.

- `Label:` and `Labels:` values become Labels, split on commas. The map's `Label: wayfinder:map` becomes a Label on its Spec.
- Every other `Key: value` header line becomes a Tag, with its key lowercased, such as `type: grilling`.
- The original State line becomes the Tag `scratch.status`, because mapping to State loses detail.
- `source` holds the path relative to the repository root, such as `.scratch/code-central-v1/issues/13-scratch-import.md`.
- Custom bullet fields are left to the agent's judgement: a Tag if they're clearly a key and a value, otherwise kept in the description.
- Header lines that became State, Blockers, Labels or Tags are removed from the description. Everything else stays verbatim, including `## Answer`, `## Comments`, `## Triage Notes` and pointers to research branches. Scratch comments aren't turned into typed `comments`, because they have no author or time.

Tag keys are lowercase `[a-z0-9._-]`, and each key has one non-empty, single-line value. The surface gains:

- `add_labels` and `remove_labels`, which now work on Specs as well.
- `set_tags(target, {k: v})`, which merges and overwrites only the keys it names.
- `remove_tags(target, keys)`.
- A `tags` filter on `find_tickets` and `find_specs`, where every pair given must match exactly, and a `labels` filter on `find_specs`.

**IDs and references.** New IDs come from the Project's counter in source order: folder, then file number. References inside bodies aren't rewritten, and the `source` Tag traces each one back. Real Blockers become `blocked_by` links. When a Blocker points at something that doesn't exist, the agent decides: it drops the link (the text stays in the description) and lists it in the plan as a doubt.

**Validation.** Any fault returns `invalid` with a `field` path such as `tickets[12].blocked_by[0]`, and a cycle among the imported Tickets returns `cycle`. Nothing is written.

**Running twice.** The Import is refused if any Spec or Ticket in the Project already has a `source` Tag equal to one in the payload. The error lists the matches. There's no merge, update or `--force`. Someone who removes a `source` Tag by hand can import that file again, which is accepted as a deliberate act.

**History.** Each imported Ticket gets one `moves` entry with no `from`, `to` set to its State, Actor `agent` and `via: import`. Only `import` can create a Ticket straight into `Done` or `Cancelled`, and only as a new record.

**The bulk write.** Under the store-wide write lock, every new file is written and fsynced into `staging/<import-id>/`. Then a manifest listing each file and its final path is written and fsynced, each file is renamed into place, and the manifest is deleted. Memory is updated after the last rename. At startup, a manifest left behind is completed (rolled forward), and a staging folder with no manifest is deleted. Every file is new, so nothing existing is overwritten. [ADR 0003](../../../docs/adr/0003-the-store-is-yaml-files-loaded-into-memory.md) is amended to say so.

**Afterwards.** Code Central never touches the repository. A successful Import reports what it created and recommends deleting `.scratch/` and committing that. Repointing `docs/agents/issue-tracker.md` belongs to [How agent sessions are taught to use Code Central](17-teaching-agent-sessions.md).

Considered and rejected:

- **A parser in the CLI:** every variant needs code for a one-off migration, and unknown forms still fail.
- **The service reading the folder:** it breaks the rule that the service reads only the store, and it fails from a sandbox.
- **A stored plan approved in the UI:** new state and a new screen for a one-off migration.
- **Importing one feature folder at a time:** Blockers across folders would dangle.
- **Sending `ready-for-agent` to `Draft` for review:** it slows down ready work. Flagging finished-looking Tickets in the plan covers stale States instead.
- **Keeping header fields in the description:** they're worth querying, so they became Labels and Tags.
- **Rewriting references in bodies:** the `source` Tag already traces them.

