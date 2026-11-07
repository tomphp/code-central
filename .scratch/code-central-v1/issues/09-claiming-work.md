# How agents claim work

Type: grilling
Status: resolved
Blocked by: 01, 05, 06

## Question

How does an agent claim work so two agents never take the same Ticket? With no Factory check-ins in v1, how is a claim left behind by a dead agent noticed and released, and into which State? How is the holder of a claim identified, given that a claim now lasts through `Needs Answer` and `Answered` and records a session ID, as How agents ask and the human answers decided?

A session that dies leaves no Failure Report and no note on where the work stands, as What a failure report contains noted, so the next agent gets neither. What, if anything, should make up for that?

## Answer

Only the human ends a Claim that an agent hasn't ended itself. A dead session is never detected for certain, so Code Central shows a warning and the human decides. Wrongly releasing a live session puts two agents on one Ticket, while a wrong warning costs the human one glance.

**Claiming.** A claim takes the next claimable Ticket in the Project: `Ready`, with no open Blockers, oldest created first, with Ticket ID as the tie-break. Checking the Ticket and moving it to `In Progress` is one atomic step, so two agents can never take the same Ticket. A claim can be narrowed:

- **No argument:** the next claimable Ticket in the Project.
- **A Spec:** the next claimable Ticket in that Spec.
- **A Ticket:** that Ticket, or a rejection if it can't be claimed.

If nothing can be claimed, the call returns an empty result rather than an error, so a loop knows to stop. There is no priority field in v1. A Ticket rejected from `In Review` or sent back from `Failed` keeps its place near the front. To hold work back, the human leaves it in `Draft` or adds a Blocker.

**What a Claim spans.** A Claim covers `In Progress`, `Needs Answer` and `Answered`. Any move out of those States ends it permanently, whether made by the holder or the human, including a human moving a Ticket out of `Needs Answer`. A Ticket rejected from `In Review` needs a fresh Claim even if the original session is still alive.

**Identifying the holder.** Code Central issues a Claim ID with each Claim. It is returned in the claim response, sent with every write that needs a Claim, and left out of Ticket reads. A write with an old Claim ID is rejected and the session stops. A session that loses its Claim ID, for example through context compaction, stops in the same way, and the human releases the Ticket.

The session ID is separate and optional. It is recorded at the claim and at each Round, can be updated, and is only used to show `claude --resume <id>`. It is never checked. This amends How agents ask and the human answers, which recorded it only with each Round. The session ID can't identify the holder because agents without the hook have none, and a Sandcastle resume may report a new one.

**Several Claims.** An agent can hold any number of Claims, one per Ticket, with no cap. A session can pass a Claim ID to a subagent, and Code Central doesn't care which process sends it. Three limits are recorded in the published workflow:

- Silence is per session, because a ping can't say which Ticket it's for, so a stuck subagent stays hidden while its main session is busy.
- Every Ticket from one session shows the same resume command, and a resume restores only the main session.
- Code Central doesn't stop two Tickets touching the same files. Blockers order such work.

**Silent Claims.** A Silent Claim is an `In Progress` Ticket with no activity for 30 minutes. It is computed, never stored, clears itself on the next activity and counts as Needs Me, which amends The State workflow. Activity is any write to the Ticket, or a throttled `PostToolUse` hook that sends the session ID. A failed ping never holds up the tool call. An agent with no hook can call a small `code-central` ping. `Needs Answer` and `Answered` can't be Silent because they already count as Needs Me.

Known false positives: an agent with no hook and no ping, a brief period after laptop sleep, and a session waiting on a terminal permission prompt, which is stuck on the human anyway.

**Recovering.** A Silent Claim shows how long it has been quiet, then Resume, then Release:

- **Resume** is the copyable `claude --resume <id>`, shown when a session ID exists. It keeps the context, the Claim ID and any uncommitted work, and needs no move.
- **Release** moves the Ticket to `Ready` with an optional note, stored as a comment. It warns that a session which is actually alive will have its next write rejected and that the Claim can't be restored.

**Making up for the missing handoff.** Nothing extra is required of healthy sessions. Claiming returns past Claims, each with when it started, its last activity and how it ended, so "released by the human, with no report" tells the next agent to look for existing work. The published workflow says work happens on a branch named after the Ticket ID, committed and pushed often.

Considered and rejected: a lease that releases a silent Ticket automatically, because a wrong guess puts two agents on one Ticket. A `Stalled` State, because a long build would flip the Ticket back and forth and silence is a guess about a session rather than a fact about the Ticket; ADR 0001 records the exception. Using the session ID as the holder, because it would leave out other agents and depend on an ID that may change. Required periodic progress notes, because healthy sessions would pay for the rare one that dies. Claiming only by ID, because a human's choice of Ticket is the exception and next is the normal case. `SessionEnd` as a signal in v1, because the docs don't say whether it fires on a crash.

Not verified: `updatedInput` on MCP tools isn't documented, though the session continuity research saw it working. `CLAUDE_CODE_SESSION_ID` was seen in a Bash subprocess by that research but wasn't found in a read of the first half of the env-vars docs. `claude --resume` keeping its session ID is inferred from the `--fork-session` wording. Whether subagents share the parent's `session_id` in hook input isn't documented. A live hook ping hasn't been run.
