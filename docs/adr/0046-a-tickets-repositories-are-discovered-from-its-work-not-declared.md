# A Ticket's Repositories are discovered from its work, not declared

A Project can span several Repositories, and a Ticket may change any of them. The Factory can't know in advance which Repositories to prepare a workspace in, and a list declared up front can drift from what the work actually did.

So a Ticket's Changed Repositories are discovered from its work: the Repositories in which its branch holds commits beyond the Base. Nobody declares them.

## Considered Options

- **Declared Repositories with a guard at finish**, checking that the work touched only what was declared. Rejected: the declaration is a guess made before the work, so the guard turns every wrong guess into a failed finish.
- **Both lists kept side by side**, a declared list and a discovered one. Rejected: two lists can disagree, and nobody could say which one is true.

## Consequences

- The Factory reports Changed Repositories live while the Ticket is In Progress, and each finish settles them, replacing the earlier list.
- Each finish carries a branch and commit per Repository.
- A Spec's Repositories are derived, as the union of its Tickets' Changed Repositories.
- There is no migration: a finish recorded before Repositories existed reads as the Home Repository's.
