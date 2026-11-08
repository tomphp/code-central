# A Claim's own session can claim it again and gets the same Claim back

Factory sessions re-claim Tickets the Factory already holds for them: the Factory claims the Ticket, starts a session under that Claim, and the session calls `claim` anyway, which used to fail because the Ticket wasn't Ready. So we decided that `claim` with a named Ticket and a non-empty session ID equal to its live Claim's session ID returns that live Claim (its Claim ID, the Ticket view in its current State and the Stage Guide), writing nothing: no new Claim, no move, no change to `updated`, `working_dir` or `resume_command`, and no event. A bare `claim` or `claim(spec)` never returns the caller's own Claim, and an empty session ID never matches. A `claim` from any other session still fails with `action_not_allowed`, saying which Claim holds the Ticket and what to do.

## Considered Options

- **Keep failing, with a better message.** Rejected because the session would still have to work out that the Claim is its own, and it can't get the Claim ID back if it lost it.
- **Return the Claim on a bare or Spec-narrowed `claim` too.** Rejected because "give me the next one" would quietly hand back the Ticket already held.
- **Update `working_dir` and `resume_command` on a re-claim.** Rejected because a re-claim should leave the Ticket's history exactly as it was; `set_session` is how to change them.

## Consequences

The session ID, which the caller supplies, now gets the Claim ID back. That weakens the Claim ID as sole proof, which is acceptable for a local, single-user service whose session IDs aren't shown anywhere another agent would pick them up by chance. A session that loses its Claim ID, for example after context compaction, can get it back by claiming again with its session ID.
