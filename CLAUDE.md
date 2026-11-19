## Agent skills

### Issue tracker

Specs and Tickets live in Code Central: see `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### The real service

Never run `serve`, `init`, `service install/uninstall`, `pkill`, a restart or a `curl` write against the service on 7733, or point `CODE_CENTRAL_URL` at it from a test or a demo: it is the human's, and the MCP server in `.mcp.json` is for tracker work only. For a running copy use `code-central serve --store <scratch> --port <spare>`. See `docs/agents/real-service.md`.

## Finding your way

Adding or changing an operation, or a store file, or renaming Go code across files: see `docs/agents/recipes.md`.

To run the UI and look at a change: see `docs/agents/running-the-ui.md`.

To learn a Go API, read `go doc ./<pkg> <Name>` before the source.

## Editing

Change a file with Read (`offset` and `limit`) then Edit; for a change in several places, make several Edit calls in one message. For a merge conflict, `git checkout --ours/--theirs <file>` then Edit.

## Checks

While working, run `make typecheck` (tsc only: it runs no Vitest; after changing a Go wire type, or a doc comment on one, `make wiretypes` regenerates the UI's types, then run all of `make vitest`) `make lint F=<file>` (ESLint on one file, or all of `web/`) and `make vitest F=<file>` (they install web dependencies first in a fresh worktree); `make e2e F=web/src/e2e/<file>` builds what `make e2e` builds and runs that one web end-to-end file; `make vitest-stress F=<file>` repeats one file under load to show a flake (`N=`, `LOAD=`); run `make test` once at the end, bare (a hook refuses a pipe): its output is already trimmed and its exit status is the result. It prints only what failed, then one `FAIL <suite> <file or test>` line per failure, and on a failure keeps the whole output in `$TMPDIR/code-central-test-<worktree>-<hash>.log` and prints its path last, so read that file rather than rerunning; failed Vitest files and Go tests killed under load are rerun once serially, and a pass there prints "passed on serial rerun: flaky under load" with exit 0; failures print the load average, and a budget overrun under a load above the core count is a warning; `make test-verbose` prints everything). `make ci-failures RUN=<id>` shows what failed in a CI run.

## Context report

To measure what agents load into context, compare the sessions since a change against the baseline: `make context-report ARGS="-baseline internal/contextreport/baseline.json -since <RFC 3339 time>"`. `docs/context-report.md` explains the figures.

## Docs

The guides in `docs/` and the README show real commands and output. When a change affects what they show (`init`, the CLI, the Factory, the UI or the Stages' skills), update them in the same change, unless the Ticket or its Spec gives the docs to another Ticket: then leave them to that Ticket.

## Commits

Commit messages are [Conventional Commits](https://www.conventionalcommits.org/), as `cog.toml` defines them, and the type sets the next release (ADR 0014): `feat` bumps the minor version, `fix` and `perf` the patch, and `!` or a `BREAKING CHANGE:` footer the major; `docs`, `test`, `refactor`, `build`, `ci`, `chore`, `style` and `revert` release nothing. For example, `feat(factory): push the base after merging (code-central-177)`.

## When Making UI Changes

Make sure e2e tests still pass.
