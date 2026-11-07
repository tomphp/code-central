# 27: Defining work

**What to build:** Agents and the human can shape work after creating it: edit a Spec or Ticket's text, add and remove Labels and Tags on Specs and Tickets, comment on any Ticket, approve a `Draft` Ticket to `Ready` and defer a `Ready` one back to `Draft`. Every edit that changes something keeps the previous text, so a rewrite can be undone by hand. See the [spec](../spec.md), sections "Operations and front doors" and "The store".

**Blocked by:** 23

**Status:** ready-for-agent

- [ ] `edit_spec(spec, title?, body?)` replaces the body whole; `edit_ticket(ticket, title?, description?)` edits a Ticket
- [ ] Each edit that changes something adds one numbered entry to `edits` with the Actor, the time and `previous` holding each changed field's old value; an edit that changes nothing records nothing
- [ ] A Spec's `edits` live in its frontmatter, with old bodies as block scalars, so an edit still writes exactly one file
- [ ] `add_labels` and `remove_labels` work on Specs and Tickets one name at a time
- [ ] `set_tags(target, {k: v})` merges and overwrites only the keys it names; `remove_tags(target, keys)` removes them; keys are lowercase `[a-z0-9._-]` and values non-empty and single-line, else `invalid` with a `field`
- [ ] `comment(ticket, body)` works on any Ticket, including one another agent holds, with no Claim ID
- [ ] `approve` moves `Draft` to `Ready`; `defer(reason)` moves `Ready` to `Draft` with the reason required and stored as a comment
- [ ] Labels and Tags changes aren't recorded in `edits`
- [ ] All these operations run through the cross-door suite
