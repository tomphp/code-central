# A Project's Repositories are a Project setting in the store, not a file in the Home Repository

A Project spans several Repositories, and something must list them, with each one's Repository Name, Clone URL and Base. The human's first sketch, and the home repo pattern article (https://www.aihero.dev/the-home-repo-pattern), keep that list in a file committed in the Home Repository.

So the list is a Project setting in Code Central's store. The Home Repository is always on it without being configured. Only the human adds or removes Repositories, in Project Options or the CLI; there is no MCP tool. A Repository Name is fixed once added; the Clone URL, Base and local path can be edited. Removing a Repository leaves its clone on disk.

## Considered Options

- **A committed file in the Home Repository.** Rejected: it differs per branch, needs a commit to change, and the service never reads git, so the UI, Needs Me and the watchers couldn't see it. It would also be the only Project setting kept outside the store; ADR 0032 moved the Base into the store for the same reason.
- **A pointer from the store to the file.** Rejected for the same reasons: the list itself would still live in git.

## Consequences

- The Factory reads the list on every poll, so a new Repository is cloned without a restart.
- Landing stays Project-wide: no per-Repository Landing Method or Push remote. (Reversed by [ADR 0054](0054-landing-has-a-project-default-and-each-repository-may-have-its-own-landing-and-push-remote.md): Landing has a Project default, and a Repository may have its own.)
- Clones go at `<parent>/<Repository Name>` beside the Home Repository by default, with an optional local path. The Factory clones a missing Repository itself, adopts a clone whose URL matches (after normalising ssh/https and a trailing `.git`), and never touches anything else, raising Repository missing instead.
- Moving the list into a file later would need a migration.
