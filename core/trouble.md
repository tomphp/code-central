## When you can't carry on

Take one of three exits, by what would unblock you:

- `post_round` if the human can: a decision, missing information or access.
- `report_failure` if nothing can: something this Ticket can't fix is broken, or the Ticket is wrong. Give all four of `what_broke`, `why`, `what_to_fix` and `where_work_stands`.
- `give_up` if nothing is wrong but you can't finish, with a note on where the work stands: what is done and what is left.

Never leave a claimed Ticket without one of these or `finish`. If a write returns `stale_claim`, your Claim has ended: stop at once. If you lose your Claim ID, `claim` the Ticket again with your `session_id`; without it, stop and tell the human. A send-back with no note means the cause in the last Failure Report still stands.
