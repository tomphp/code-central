# A Ticket gets a worktree in every Repository at each Claim's start, beside each clone

A Ticket's work can change any of its Project's Repositories, and its session needs a checkout of each one it changes. Changed Repositories are discovered, not declared ([ADR 0046](0046-a-tickets-repositories-are-discovered-from-its-work-not-declared.md)), so the Factory can't know which Repositories a Ticket will change, and a running Claude Code session can't gain new directories.

So at each Claim's start the Factory makes or reuses the Ticket's worktree in every Repository, beside each clone. The Home worktree stays at `<home>.worktrees/<ticket-id>` and is the session's working directory; each other Repository's worktree is at `<clone parent>/<Repository Name>.worktrees/<ticket-id>`. The session reaches the others through `--add-dir`, and its first prompt lists each Repository Name with its path. Stage Tickets get worktrees detached at each Repository's Base tip, under the same layout and the same check that fails a dirty one.

A Repository added mid-session waits for the next Claim. Unchanged Repositories' branches and worktrees are swept with the rest once the Ticket is done.

This amends [ADR 0032](0032-the-base-is-a-project-setting-not-the-factorys.md)'s single base only as far as [ADR 0047](0047-a-projects-repositories-are-a-project-setting-in-the-store-not-a-file-in-the-home-repository.md) already does: each Repository has its own Base.

## Considered Options

- **Worktrees made on demand**, when the work first touches a Repository. Rejected: the Factory can't tell in advance which Repositories that will be, and a running session can't gain the new directory.
- **Worktrees grouped per Ticket**, at `<home>.worktrees/<ticket-id>/<Repository Name>`. Rejected: the session would start outside any git checkout, or every existing path would move. Beside each clone, the session's working directory, CLAUDE.md, `.mcp.json`, the Claim's working_dir, the Resume Command and the context report all keep working as today, and a single-repository Project is unchanged.
- **The other worktrees nested inside the Home worktree.** Rejected: they would leave untracked files in the Home branch.

## Consequences

- Branch names (`<ticket-id>`, `spec/<spec id>`, `prototype/<id>`, `research/<id>`) are the same in every Repository, each cut from that Repository's own Base or Spec Branch. Each Repository's refs are its own, so a prefix would buy nothing.
- `finish` gains `repositories: [{name, branch, commit}]`, and `branch`/`commit` still mean the Home Repository. The Factory checks the list against every worktree's branch; a Repository with commits that is missing from the list, or whose commit is wrong, gets a Failure Report.
- Every Claim makes a worktree in every Repository, including those the Ticket never changes.
