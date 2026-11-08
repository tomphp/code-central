# Accepted work lands before it is Done

In v1, a merge is what completes an implementation Ticket. Accepting the Ticket merges its branch into the base, and a merge you make yourself counts as accepting it. But accept moved a Ticket straight to Done, and only Done clears a Blocker. So a merge that failed after accept would leave a Done Ticket unmerged, and a Ticket it blocked could be claimed before the merge landed. We decided to add a State, `Accepted`, between `In Review` and `Done`: the human has accepted the work and it is waiting to land. `finish` takes an optional branch and head commit. When a Ticket has a branch, accept moves it to Accepted. When it doesn't, as with Stage Tickets, accept still goes straight to Done. A new action, `land`, moves the Ticket from Accepted or In Review to Done and records the commit that put the work into the base. An agent or the human takes it, and the Factory takes it both after its own merge and when it sees a merge made by hand. Code Central never sees git, so whoever delivers the work reports the landing, and the model says nothing about how it lands. A required pull request, which comes later, would fit the same State and action.

## Considered Options

- **Accept moves to Done and the Factory merges afterwards.** Rejected because a failed merge would leave a Done Ticket unmerged, which no agent action can reopen, and because a Ticket it blocked could start before the merge landed.
- **The Factory merges first and then accepts.** Rejected because the human's accept would have to become a request kept beside the State, which is the kind of flag [ADR 0001](0001-one-state-value-on-tickets.md) rules out.
- **The Factory sends `accept` as the human.** Rejected because the Actor header isn't authenticated, so the Moves would record a human who never acted.
- **A new `factory` Actor.** Rejected because the Factory is already an agent client, and every front door would have to learn one more Actor.
- **Deciding by Label which Tickets land** (no `stage:*` Label). Rejected because a Label has no workflow meaning, and this would tie the workflow to one Factory's Labels.

## Consequences

Every Factory, the UI and each Integration has to learn the Accepted State. When a merge conflicts, the Factory sends the Ticket back with `send_back`, which gains an agent row from Accepted to Ready with a required note. A later Claim then resolves the conflict, and the human reviews it again. A merge that is clean but can't land yet, for example because the human's uncommitted changes are in the way, leaves the Ticket Accepted. The Factory retries it on each poll, and once it has been Accepted for more than 10 minutes it shows in Needs Me. A squash or rebase merge done by hand isn't recognised, because the commit changes, so the human accepts or lands it as usual.
