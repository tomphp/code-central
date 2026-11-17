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

Adding or changing an operation, or a store file: see `docs/agents/recipes.md`.

To run the UI and look at a change: see `docs/agents/running-the-ui.md`.

To learn a Go API, read `go doc ./<pkg> <Name>` before the source.

## Editing

Change a file with Read (`offset` and `limit`) then Edit; a hook refuses `sed -i` and a python or perl replace on one file.

## Checks

While working, run `make typecheck` (after changing a Go wire type, `make wiretypes` regenerates the UI's types) `make lint F=<file>` (ESLint on one file, or all of `web/`) and `make vitest F=<file>` (they install web dependencies first in a fresh worktree); `make e2e F=web/src/e2e/<file>` builds what `make e2e` builds and runs that one web end-to-end file; `make vitest-stress F=<file>` repeats one file under load to show a flake (`N=`, `LOAD=`); run `make test` once at the end (it prints only what failed, and on a failure keeps the whole output in `$TMPDIR/code-central-test-<worktree>-<hash>.log` and prints its path last, so read that file rather than rerunning; `make test-verbose` prints everything). `make ci-failures RUN=<id>` shows what failed in a CI run.

## Context report

To measure what agents load into context, compare the sessions since a change against the baseline: `make context-report ARGS="-baseline internal/contextreport/baseline.json -since <RFC 3339 time>"`. `docs/context-report.md` explains the figures.

## Docs

The guides in `docs/` and the README show real commands and output. When a change affects what they show (`init`, the CLI, the Factory, the UI or the Stages' skills), update them in the same change.

## Commits

Commit messages are [Conventional Commits](https://www.conventionalcommits.org/), as `cog.toml` defines them, and the type sets the next release (ADR 0014): `feat` bumps the minor version, `fix` and `perf` the patch, and `!` or a `BREAKING CHANGE:` footer the major; `docs`, `test`, `refactor`, `build`, `ci`, `chore`, `style` and `revert` release nothing. For example, `feat(factory): push the base after merging (code-central-177)`.
