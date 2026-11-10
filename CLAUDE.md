## Agent skills

### Issue tracker

Specs and Tickets live in Code Central: see `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Recipes

Adding or changing an operation, or a store file: see `docs/agents/recipes.md`.

## Checks

While working, run `make typecheck` and `make vitest F=<file>` (they install web dependencies first in a fresh worktree); run `make test` once at the end.

## Context report

To measure what agents load into context, compare the sessions since a change against the baseline: `make context-report ARGS="-baseline internal/contextreport/baseline.json -since <RFC 3339 time>"`. `docs/context-report.md` explains the figures.

## Docs

The guides in `docs/` and the README show real commands and output. When a change affects what they show (`init`, the CLI, the Factory, the UI or the Stages' skills), update them in the same change.

## Commits

Commit messages are [Conventional Commits](https://www.conventionalcommits.org/), as `cog.toml` defines them, and the type sets the next release (ADR 0014): `feat` bumps the minor version, `fix` and `perf` the patch, and `!` or a `BREAKING CHANGE:` footer the major; `docs`, `test`, `refactor`, `build`, `ci`, `chore`, `style` and `revert` release nothing. For example, `feat(factory): push the base after merging (code-central-177)`.
