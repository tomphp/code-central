# Versions come from conventional commits; a release is a tag pushed by CI

Code Central had no releases and no tags, and its commit messages followed no convention, so nothing could tell from the history whether a change was a feature, a fix or a breaking change. We decided that from the baseline tag `v1.0.0` on, every commit message follows Conventional Commits, and cocogitto (`cog`) works out the next version from the commits since the latest tag: `feat` bumps the minor, `fix` and `perf` the patch, and `!` or a `BREAKING CHANGE:` footer the major; every other type releases nothing. A release happens in CI, only after the CI workflow has passed on `main`, and builds from the exact commit CI tested. It pushes a tag only: there is no bump commit and no `CHANGELOG.md`, and the release notes are generated from the commits instead. The Factory lands work into the human's local `main` and the human pushes it, so a bot commit on `origin/main` would make every later push diverge.

The convention lives in one file, `cog.toml`, which both the committed `commit-msg` hook in `.githooks/` and CI's `cog check` read, so they never disagree about what is valid. Merge commits are exempt: `cog.toml` sets `ignore_merge_commits`, and the hook passes any message whose first line starts `Merge ` without calling cog. So agents' `Merge main into …` commits and the Factory's `Merge code-central-N: …` landings, which `git commit-tree` writes without running hooks, need nothing new. `v1.0.0` is the baseline: the human tagged it by hand on `main`'s tip, it has no binaries, and cog reads only the commits after it, so the history from before the convention is ignored.

## Considered Options

- **A bump commit on `main` with a `CHANGELOG.md`**, as cog does by default. Rejected: CI would push a commit the human's local `main` doesn't have, and the next push of landed work would be refused until the human pulled it.
- **Releasing from a manually pushed tag.** Rejected: the human would have to work out each version and remember to release.
- **A hook that skips the check when `cog` is missing.** Rejected: a missing tool would go unnoticed until CI failed after the push. The hook refuses the commit and says how to install cog.

## Consequences

Every commit, by an agent or the human, needs `cog` on PATH once the hook is on; `make test`, `make typecheck`, `make vitest` and `make build` turn it on through `make hooks`, and `CLAUDE.md` lists installing cog as a setup step. A worktree on a branch from before the hook has no `.githooks/`, and a commit made with `--no-verify` skips the hook, so CI's `cog check` from the latest tag catches what slips through, in a job of its own beside the tests. The convention is this repository's rule only: `init`'s generated files and the Stage Guides don't teach it to other Projects. If `v1.0.0` were missing when this work reached `origin`, the check and the bump would read the whole history, so the human tags it before pushing.
