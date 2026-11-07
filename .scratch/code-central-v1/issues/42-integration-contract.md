# 42: The Integration contract

**What to build:** v1 is ready for Integrations without shipping one. A separate program can create linked Specs and Tickets as the `integration` Actor, keep their definitions up to date and record the external status; the human sees linked items, can't edit their definitions, sees "closed externally" in Needs Me and can unlink. See the [spec](../spec.md), section "Integrations", and [ADR 0004](../../../docs/adr/0004-an-integration-is-a-client-of-the-http-api.md).

**Blocked by:** 27, 32, 33

**Status:** ready-for-agent

- [ ] Specs and Tickets gain an optional `link`: `{integration, id, url, external_state: open|closed|deleted|out_of_scope, detached}`, at most one per item
- [ ] HTTP-only operations for the `integration` Actor create linked items in `Draft` or `Ready`, update their title, description and parent, set `external_state`, and look up by link (including detached links); keep their signatures minimal, as the first real Integration will settle them
- [ ] Only the `integration` Actor can write definitions on a linked item that isn't detached; the human and agents get `action_not_allowed`; the `integration` Actor can't move State; its definition updates go through edit history with Actor `integration`
- [ ] A linked Ticket whose `external_state` isn't `open`, whose link isn't detached and that isn't finished gets the computed `needs_me_reason` "closed externally", last in the Needs Me order; reopening clears it
- [ ] Human `unlink` marks the link `detached`, making the definition editable and stopping it counting for Needs Me
- [ ] The UI shows a Link badge (Integration and external ID, linking to the URL) on rows and in the detail, a note that the title and description are read-only while linked, and Unlink in the detail
- [ ] A closed-externally panel links to the external item and offers Cancel, Mark Done (the human's `move` to `Done`) and Unlink
- [ ] The Actor rules, "closed externally" and `unlink` are tested in `core`; the badge and panel have component tests against the fake API
