# 41: `init` teaching files and `--local`

**What to build:** `init` finishes setting a repository up for agents with pointer files only, so nothing in the repository describes the workflow or goes stale. It works in repositories that can't take committed files, and running it again refreshes everything safely. See the [spec](../spec.md), section "Projects".

**Blocked by:** 31

**Status:** resolved

- [x] `init` writes `docs/agents/issue-tracker.md` (mapping the skills' operations to Code Central operations by name, including Wayfinding: the map is a Spec with the `wayfinder:map` Label, blocking is `add_blocker`, the frontier is `find_tickets(spec, claimable)`, claiming is `claim(ticket)`, resolving is `finish` then the human accepting) and `docs/agents/triage-labels.md` (`needs-triage` to `Draft`, `ready-for-agent` to `Ready`, `wontfix` to `Cancelled`, the rest as Labels), each pointing at `get_workflow`
- [x] It adds a pointer to the tracker doc in `CLAUDE.md`, only if nothing there points at it yet; `AGENTS.md` isn't touched
- [x] `init --local` writes the same files but adds `.mcp.json` and the `docs/agents/` files to `.git/info/exclude`, puts the hooks in `.claude/settings.local.json` and the pointer in `CLAUDE.local.md`
- [x] The store remembers each Project's mode, re-running `init` follows it, and a flag that conflicts with it asks before switching
- [x] Files `init` writes carry a header comment with the writing version; re-running `init` on a known key rewrites the hook entries and those files to the current templates, and shows a diff and asks before replacing any other file
- [x] `code-central help sandboxes` covers `sbx policy allow network localhost:7733` and `--bind`; `init`'s last line points to it, and so do connection errors that can come from a sandbox
- [x] A `core` test fails if a generated `docs/agents/` file exceeds about 400 tokens
- [x] Both modes are tested in the CLI package against `codecentraltest`
