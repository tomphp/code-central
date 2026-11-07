# The store's file layout

Type: grilling
Status: resolved
Blocked by: 05, 06, 07, 08

## Question

What is the on-disk layout of the central store: TOML or YAML, file structure and ID scheme? How are concurrent writes kept safe, how do queries stay fast over files, and what happens if a human edits a file by hand? How a Project is identified fixed a Project's stored data as its Project Key and display name. The key is permanent, so can it prefix Ticket IDs?

[How agents claim work](09-claiming-work.md) adds what must be stored: a Claim ID on each Claim, the session ID, last activity, and the history of past Claims with how each ended. A claim must be one atomic step across every process that can write, and picking the next Ticket needs the creation order.

[Process shape and web stack](10-process-and-stack.md) made the running service the only writer and has it lock the store at startup ([ADR 0002](../../../docs/adr/0002-the-service-is-the-only-writer.md)). Writes from different processes no longer compete, so the remaining questions are about writes inside one process, crash safety, and what the service does when it finds a file edited by hand. Claims, Rounds and other state must live only in the store, never in memory, so a restart loses nothing. Where is the store on disk, and is that location configurable?

## Answer

Amended by [How Code Central is upgraded](21-upgrades.md), which settled migrations: `serve` migrates automatically through a staging folder with a backup, and every schema change bumps `format`.

Amended by [How finished work is kept and hidden](20-finished-work.md), which added `archived` to `project.yaml` and an `edits` list with previous values to Tickets and to Spec frontmatter, and kept every file loaded for good.

Amended by [How the Scratch Import behaves](13-scratch-import.md), which gave Specs and Tickets `labels` and `tags` and made Import write through a staging folder and a roll-forward manifest.

The store is a readable record of plain files that the service loads whole into memory at startup. See [ADR 0003](../../../docs/adr/0003-the-store-is-yaml-files-loaded-into-memory.md).

**Location.** `$XDG_DATA_HOME/code-central`, falling back to `~/.local/share/code-central` on both macOS and Linux. It can be overridden with `CODE_CENTRAL_STORE` or `serve --store <dir>`. Only `serve` reads it, because every other command goes through HTTP.

**Layout.** The Project Key is the folder name, which is safe because the key is permanent. Files are named by their bare number. There is no global index file.

```
store.yaml                 # format: 1
projects/
  code-central/
    project.yaml           # key, display name, created
    specs/12.md
    tickets/13.yaml
```

The service refuses to start on a format version it doesn't know, and names it. How migrations work stays in the fog.

**IDs.** Each Project has one counter shared by Specs and Tickets, so a number is unique within the Project and is never reused, including for Cancelled Tickets. The display ID is `<key>-<n>`, such as `code-central-12`, which is also the branch name. MCP and the CLI also accept a bare `12`, because a connection is already scoped to one Project. The counter isn't stored: it is the highest existing number plus one, worked out at startup.

**Format.** YAML, with Markdown in literal block scalars (`|`), written only by the service in a fixed field order and decoded strictly, so an unknown field is an error.

- **A Ticket** is one YAML file holding everything:
  - its title, State, Spec, Labels, `blocked_by`, `replacements`, `created` and `updated`
  - its description
  - typed, numbered lists: `claims`, `rounds` (Questions and answers), `failure_reports`, `comments` and `moves`

  The UI and the claim response merge these lists by time when they need a timeline.
- **A Spec** is Markdown with strict YAML frontmatter (id, title, created, updated), and its body is the Spec itself.
- **Timestamps** are UTC RFC 3339. `created` gives the claim order, with the number as the tie-break.
- **A Claim entry** stores its Claim ID: 128 random bits, base32-encoded. It is left out of every API read. The file itself isn't secret, which is consistent with having no authentication.
- **`moves`** records each State move with from, to, Actor, time and the Claim number if there is one, so the audit trail can be built later. What the UI shows, and archiving, stay in the **Ticket history** fog.

**Reads.** Every read comes from memory, which is only safe because the service is the only writer and hand edits need a restart.

**Writes.** One store-wide read-write mutex is held across the check, the file write and the memory update, so a claim is atomic without further work. Each operation writes exactly one file: a temp file, then fsync, then rename. Memory is updated only after the file is on disk. Anything that would need two files is reshaped to need one:

- Being blocked by a Cancelled Ticket follows its Replacements when read, so other Tickets are never rewritten.
- The ID counter is derived rather than stored.
- History lives in the Ticket's own file.

Import is the one bulk write, and has to stage its files and swap them into place in one step.

**Hand edits.** These are supported only while the service is stopped: stop it, edit, start it. A file that fails validation stops startup and names the file and field. It is never skipped, because skipping would silently hide a Ticket. Edits made while the service is running are unsupported and are overwritten on the next write to that file. There is no file watcher in v1.

**Last activity.** Pings update memory only. Last activity is persisted only with real writes to a Ticket. After a restart, every claimed Ticket's last activity counts from the time the service started, which can delay a Silent Claim warning by at most 30 minutes but never causes a false one. This is the one exception to "state lives only in the store", because it is a guess about a session rather than a fact about the Ticket.

Considered and rejected:
- **SQLite:** the store would no longer be readable or fixable by hand.
- **Reading files for each query, or an index on disk:** memory is simpler and faster.
- **Reloading files through a watcher:** more moving parts for a rare need.
- **TOML:** its multi-line strings are awkward to hand-edit.
- **A folder per Ticket:** writes would span several files.
- **One mixed activity timeline:** a strict schema per type is simpler.
- **Persisting every ping:** too much disk churn for a value that is only a guess.
- **Locks per Project or per Ticket:** with one user, contention doesn't matter.
