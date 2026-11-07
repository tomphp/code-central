# 28: Blockers, Replacements and finding work

**What to build:** Agents order work with Blockers and cancel work by naming what replaces it, and claiming respects both. Agents can find work by every filter the spec lists and see a Spec's progress at a glance. See the [spec](../spec.md), sections "The workflow" and "Operations and front doors".

**Blocked by:** 25, 27

**Status:** resolved

- [x] `add_blocker` and `remove_blocker` link Tickets in the same Project, across Specs; a link that would form a cycle returns `cycle`; `create_ticket` accepts `blocked_by`
- [x] Only `Done` clears a Blocker; a blocked `Ready` Ticket stays `Ready` but isn't claimable, and `claim` of a named blocked Ticket is rejected with its Blockers as the reason
- [x] Agent `cancel(ticket, replacements, reason?)` cancels a `Draft` or `Ready` Ticket only by naming at least one Replacement; human `cancel(replacements?)` cancels any unfinished Ticket
- [x] Anything blocked by a Cancelled Ticket is blocked by its Replacements instead, resolved when read so no other Ticket's file is rewritten; a Cancelled Blocker with no Replacements keeps its dependants blocked
- [x] `get_ticket` shows each Blocker with its State after following Replacements, plus the computed `blocked` and `claimable`
- [x] `find_tickets(state?, title?, labels?, tags?, spec?, claimable?)` matches every filter given (case-insensitive title substring, all Labels present, every Tag pair exact), leaves out `Done` and `Cancelled` unless the State filter names them, and returns rows of ID, title, State, Spec ID, Labels, `blocked` and `updated` in claim order
- [x] `get_spec` returns the frontmatter, the body, a State summary (a count per State, or `Draft` with no Tickets) and its Tickets as `find_tickets` rows
- [x] `find_specs(title?, labels?, tags?, finished?)` leaves out finished Specs unless `finished: true` is passed, and its rows include the State summary
- [x] The store contract suite covers Replacements followed on read
