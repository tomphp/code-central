## Agent skills

### Issue tracker

Specs and Tickets live in Code Central: see `docs/agents/issue-tracker.md`.

### Triage labels

Uses the five default labels: `needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` and `docs/adr/` at the repo root. See `docs/agents/domain.md`.

## Checks

While working, run `make typecheck` and `make vitest F=<file>` (they install web dependencies first in a fresh worktree); run `make test` once at the end.

## Docs

The guides in `docs/` and the README show real commands and output. When a change affects what they show (`init`, the CLI, the Factory, the UI or the Stages' skills), update them in the same change.

## Commits

From the tag `v1.0.0` on, every commit message follows [Conventional Commits](https://www.conventionalcommits.org/), as `cog.toml` defines them. The version of the next release comes from these messages (see `docs/adr/0014-versions-come-from-conventional-commits-a-release-is-a-tag-pushed-by-ci.md`).

The first line is `type(scope)!: subject`. The scope is optional; `!` marks a breaking change, as does a `BREAKING CHANGE:` footer. The types, and what each bumps:

- `feat`: a new feature. Bumps the minor version.
- `fix`: a bug fix. Bumps the patch version.
- `perf`: a performance improvement. Bumps the patch version.
- `docs`, `test`, `refactor`, `build`, `ci`, `chore`, `style`, `revert`: release nothing.
- `!` or `BREAKING CHANGE:` on any type: bumps the major version.

For example, `feat(factory): push the base after merging (code-central-177)` or `fix: keep the Inbox's order on refresh`.

Merge commits, whose first line starts `Merge ` (`Merge main into code-central-171`, the Factory's `Merge code-central-172: …`), are exempt.

The committed `commit-msg` hook in `.githooks/` refuses a message that doesn't follow the convention, with cog's explanation. `make test`, `make typecheck`, `make vitest` and `make build` turn it on (`make hooks`). CI runs `cog check` over the commits since the latest tag, to catch any that bypassed the hook.

Setup: install cocogitto, which provides `cog`, before your first commit. The hook refuses every commit while `cog` isn't on PATH.

- macOS or Linux: `brew install cocogitto`
- Any OS with Rust: `cargo install cocogitto`
- Windows: download a release binary from <https://github.com/cocogitto/cocogitto/releases> and put `cog` on PATH.
