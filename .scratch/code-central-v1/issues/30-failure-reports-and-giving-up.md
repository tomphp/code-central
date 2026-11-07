# 30: Failure Reports and giving up

**What to build:** An agent with no way forward posts a Failure Report that says what broke, why, what to fix and where the work stands, which ends its Claim. The human sends the Ticket back (optionally with a note), returns it to `Draft`, or cancels it. An agent that can't finish but found nothing wrong gives up cleanly with a note. The next agent to claim the Ticket gets the whole history. See the [spec](../spec.md), sections "Rounds, Questions and Failure Reports" and "Claims".

**Blocked by:** 25

**Status:** ready-for-agent

- [ ] `report_failure(ticket, claim_id, what_broke, why, what_to_fix, where_work_stands)` is allowed only for the claim holder from `In Progress`; a report missing any field returns `invalid` with that `field` and leaves the Ticket `In Progress`
- [ ] A report is numbered, permanent and uneditable, keeps the Claim's session ID, moves the Ticket to `Failed`, ends the Claim, and returns "stop working this Ticket"
- [ ] Nobody can move a Ticket to `Failed` any other way, including the human
- [ ] Human `send_back(note?)` moves `Failed` to `Ready`, storing any note as a comment, and the Ticket keeps its place near the front of the claim order; human `to_draft` moves `Failed` to `Draft`; human `cancel` works from `Failed`
- [ ] `give_up(ticket, claim_id, note)` moves `In Progress` to `Ready` with the note required and stored as a comment, and returns "stop working this Ticket"
- [ ] `claim` and `get_ticket` return every past Claim with when it started, its last activity and how it ended, every Failure Report in order, and every comment including send-back notes
