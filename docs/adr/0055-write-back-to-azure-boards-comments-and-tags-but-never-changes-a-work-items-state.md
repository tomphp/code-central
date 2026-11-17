# Write-back to Azure Boards comments and tags but never changes a work item's State

On a GitHub issue, Write-back posts comments, keeps one Status label, and closes the issue after Won't fix or once its Spec is Finished. A GitHub issue has two states, open and closed. A work item's States, and which transitions between them are allowed, vary by process and by organisation, and a transition can fail on the organisation's own workflow rules.

So Write-back to Azure Boards posts the same comments to the work item's Discussion, each exactly once, and keeps one `code-central:` tag of the Status label's names (`draft`, `requirement`, `building`, `done`, `wont-fix`), replacing only `code-central:` tags. It never changes the work item's State: after Won't fix, or once the work is done, the human moves the State in Azure DevOps, and Won't fix says so where it is offered.

## Considered Options

- **A per-Project State mapping** (which State means done, which means won't fix). Rejected for now: process-specific States and transition rules would make Write-back fail in ways the human can't see from Code Central. It is a known follow-up.

## Consequences

- Write-back never fails on a workflow rule, and the human's team keeps its own process in charge of the work item's State.
- A close is never due on an Azure Boards Link.
- The `code-central:` tag shows where the work stands, so boards and queries can filter on it.
