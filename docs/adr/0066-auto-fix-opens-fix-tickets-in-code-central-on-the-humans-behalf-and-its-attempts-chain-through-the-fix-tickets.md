# Auto-fix opens fix Tickets in Code Central on the human's behalf, and its attempts chain through the fix Tickets

A CI failed item used to wait in Needs Me for the human to open a New fix Ticket. Auto-fix is a Project setting, off unless the human turns it on (and only the human can, over HTTP, like [Auto-accept](0037-auto-accept-happens-within-finish-on-the-humans-behalf-and-only-the-human-can-turn-it-on.md)), that opens the fix Ticket on the CI Fixes Spec for them, up to an Auto-fix limit of attempts per failure.

**It lives in Code Central, not the Factory or the CI Watcher.** The CI Watcher only reports runs and the Factory only works Tickets; neither knows the Project's settings, and both can run twice or crash between two writes. Code Central already holds the CI state, the responses and the fix Tickets, so one sweep per Project, run after each write that can make an item ready, opens the Tickets in the same store as the rest.

**The mark is stored and only Code Central sets it.** An auto fix Ticket carries a mark with its attempt number, the Repository and commit it was opened for, the Tickets it covers and the previous attempt's fix Ticket, as [ADR 0038](0038-code-central-opens-spec-merge-tickets-itself-at-the-factorys-request-and-marks-them.md) does for Spec merge Tickets. No Label stands in for it, so an agent can't make a Ticket look automatic, and a manual New fix Ticket gets none.

**Attempts are counted along the chain of fix Tickets, not per commit or per day.** The next attempt is one more than the highest attempt among the auto fix Tickets the failed item lists. A per-commit count would restart at 1 on every new commit, and a per-day count would stop a Project for reasons unrelated to the failure. The chain follows the failure through its fix Tickets' own pushes.

**It is level-triggered.** An item that is already waiting when Auto-fix is turned on, or when the limit is raised, is acted on; Auto-fix doesn't act on new items only. The setting then means what it says whenever it is read.

**Cancel hands back.** Recording any fix Ticket on a commit hands the commit to that Ticket for good, so cancelling an auto fix Ticket returns the item to the human, and the Tickets it covered with it, instead of reopening at attempt 1. The human cancelled it for a reason.

**No automatic re-run first.** A re-run of the failed jobs would hide a flaky test, which the Fix instructions ask the fix Ticket to find. Auto-fix opens the fix Ticket at once.

**No new Actor.** The comment on the failed Ticket is written as `integration`, Code Central acting from the CI Watcher's report; the Ticket opens Ready on the CI Fixes Spec and [Auto-accept](0037-auto-accept-happens-within-finish-on-the-humans-behalf-and-only-the-human-can-turn-it-on.md) treats it as any implementation Ticket.

## Considered Options

- **The Factory or the CI Watcher opens the Ticket.** Rejected: neither holds the setting or the CI state, and neither is idempotent across a crash.
- **A Label marks the auto fix Ticket.** Rejected for the reason [ADR 0038](0038-code-central-opens-spec-merge-tickets-itself-at-the-factorys-request-and-marks-them.md) gives: a mark an agent can write can't carry meaning.
- **Count per commit or per day.** Rejected above.
- **Act on new items only.** Rejected: turning the setting on would then leave the waiting failures where they are.
- **A cancel reopens at attempt 1.** Rejected: it would undo the human's decision on the next commit.

## Consequences

`set_auto_fix` has no MCP tool and no CLI command, so agents' tool lists don't grow. The setting is stored in the Project's CI settings and kept while CI Tracking is off, when Auto-fix doesn't act.
