# 40: Scratch Import

**What to build:** An agent reads a repository's whole `.scratch/` folder, following guidance the server publishes, and maps it into one Import. A dry run shows the human the plan and the agent's doubts; after the go-ahead, the real Import writes every new Spec and Ticket all at once, even across a crash, and can't be run twice. See the [spec](../spec.md), section "Import", and user stories 107–116.

**Blocked by:** 28

**Status:** ready-for-agent

- [ ] `import(payload, dry_run?)` is in `core`'s operation list and exposed as an MCP tool, `code-central import` (payload on stdin or through `--file`) and `POST /api/p/<key>/import`; it writes as `agent` and never creates a Project
- [ ] `get_import_guide` (also `code-central import-guide`) publishes the mapping rules from the spec: one Spec per feature folder, the State table, Labels and Tags from header lines, `scratch.status` and `source` Tags, what stays in descriptions, and to present the plan, list doubts and wait for the go-ahead
- [ ] The MCP `instructions` gain one line: "To import a `.scratch/` folder, call `get_import_guide`."
- [ ] The payload is validated strictly: any fault returns `invalid` with a `field` path such as `tickets[12].blocked_by[0]`, a cycle among imported Tickets returns `cycle`, and nothing is written
- [ ] The Import is refused if any Spec or Ticket in the Project already has a `source` Tag in the payload, listing the matches
- [ ] A `dry_run` runs every check and writes nothing, returning each source path with its provisional ID, title, State, Labels, Tags and Blockers, grouped by Spec, with a count per State
- [ ] New IDs come from the counter in source order; each imported Ticket gets one `moves` entry with no `from`, Actor `agent` and `via: import`; only Import can create a Ticket directly in `Done` or `Cancelled`; no Claims are imported
- [ ] The bulk write, under the write lock, stages and fsyncs every new file, writes and fsyncs a manifest, renames each file into place, then deletes the manifest; memory is updated after the last rename
- [ ] At startup a leftover manifest is rolled forward and a staging folder with no manifest is deleted
- [ ] The file-backed Import has a test-only "stop after step k" hook; for every k, a test runs a real Import that stops after step k, runs startup recovery and checks the result is all or nothing; two named cases cover an orphan staging folder and a partly renamed manifest
- [ ] The store contract suite covers Import's all-or-nothing result; the validator and its `field` paths are tested in `core`
- [ ] A successful Import reports what it created and recommends deleting `.scratch/` and committing that
