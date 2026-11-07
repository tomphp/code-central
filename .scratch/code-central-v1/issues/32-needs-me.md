# 32: Needs Me across Projects

**What to build:** One read tells the human everything that is waiting on them across every Project, already grouped by reason and ordered cheapest-to-unblock first, so every view renders the same order. The human can also move any Ticket anywhere except `Failed`, and rename a Project. See the [spec](../spec.md), section "Needs Me and Notifications".

**Blocked by:** 28, 29, 30, 31

**Status:** ready-for-agent

- [ ] `core` computes `needs_me_reason` on each Ticket: `Needs Answer`, `Answered`, Silent Claim, `Failed`, `In Review`, or blocked by a Cancelled Ticket with no Replacements (closed externally arrives with ticket 42)
- [ ] `core` orders groups as Questions, `Answered`, Silent Claim, `Failed`, `In Review`, blocked by a Cancelled Ticket, and within a group puts the longest-waiting Ticket first
- [ ] `GET /api/needs-me` returns these groups across every Project, already ordered, with what each row needs for its one-line detail (open Questions out of how many and the Round number; when answered; how long silent; the latest report's headline and failure count; the first line of the `finish` summary; the Cancelled Blocker)
- [ ] `get_ticket` includes `needs_me_reason`
- [ ] `GET /api/projects` lists every Project
- [ ] Human `move(to_state)` moves a Ticket to any State except `Failed`; moving out of a Claim State ends the Claim
- [ ] Human `rename_project` changes the display name and never the key
- [ ] Grouping and order are tested in `core` with a fake clock
