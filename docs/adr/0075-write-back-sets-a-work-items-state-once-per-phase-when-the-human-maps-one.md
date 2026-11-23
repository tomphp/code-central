# Write-back sets a work item's State once per phase when the human maps one

[ADR 0055](0055-write-back-to-azure-boards-comments-and-tags-but-never-changes-a-work-items-state.md) had Write-back to Azure Boards never change a work item's State, and rejected a per-Project State mapping for now as a known follow-up. This is that mapping, and it supersedes 0055's "never changes a work item's State" in part. Comments and tags stay as 0055 left them.

The human sets **Work Item States** in the Azure Boards section of Project Options: the State name to set when the linked work first reaches In Progress, Accepted or Done. Each is blank by default, and a blank one sets nothing, so a Project that sets none behaves as under 0055.

## Decisions

- **Set once, never kept in step.** Write-back sets a State once per phase and never again, even if the work moves back or a teammate moves the item. Code Central doesn't fight the team's process over the State.
- **Furthest only.** When several phases are due at once, only the furthest is set.
- **No backfill.** Only work that reaches a phase after its mapping was last turned on is moved. A mapping turned on (blank to non-blank) is dated; an edit of a non-blank name keeps its date, so a corrected typo is retried for the work that failed on it.
- **A refusal is visible and retried.** When Azure DevOps refuses a State, the Work Item Watcher's status shows an error naming the item, State and reason, and it retries each poll until the State is written, the item is already in it, or the mapping is blanked or changed. Comments and tags are still written, and other items' States are still set.
- **No Won't fix mapping.** Won't fix only comments and tags; the State is still left to the human.
- **Written past out of scope.** The State is written after the item drops out of the query.

Saving Work Item States never runs the Azure Boards check, and resets the Watcher's status so a refused-State error clears at once.

## Consequences

- Write-back can now fail on a workflow rule or an unknown State. That failure is shown, not hidden, and doesn't block the comments and tags.
- A team whose process differs from the three phases leaves the mapping blank and keeps moving States by hand.
